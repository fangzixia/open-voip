package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"

	"open-call/internal/config"
)

// Session OpenAI Realtime 双向音频会话。
type Session struct {
	conn *websocket.Conn
	mu   sync.Mutex
	onAudio func(pcm []byte)

	log       *slog.Logger
	callID    string
	provider  string
	inRateHz  int
	outRateHz int

	outBytes   atomic.Int64
	outChunks  atomic.Int64
	outSamples atomic.Int64
}

// Connect 建立 Realtime WebSocket 并完成 session.update。
func Connect(ctx context.Context, cfg config.AibotOpenAIConfig, systemPrompt string) (*Session, error) {
	u, err := realtimeWSURL(cfg)
	if err != nil {
		return nil, err
	}
	opts := &websocket.DialOptions{HTTPHeader: dialHeaders(cfg)}
	conn, _, err := websocket.Dial(ctx, u, opts)
	if err != nil {
		return nil, err
	}
	s := &Session{
		conn:      conn,
		provider:  strings.TrimSpace(cfg.Provider),
		inRateHz:  RealtimeInputRate(cfg),
		outRateHz: RealtimeOutputRate(cfg),
	}
	inFmt, outFmt := sessionAudioFormats(cfg)
	update := map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"modalities":          []string{"text", "audio"},
			"instructions":        systemPrompt,
			"voice":               cfg.Voice,
			"input_audio_format":  inFmt,
			"output_audio_format": outFmt,
			"turn_detection":      map[string]any{"type": "server_vad"},
		},
	}
	if err := s.writeJSON(ctx, update); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "session.update failed")
		return nil, err
	}
	if s.log != nil {
		s.log.Info("aibot.realtime.session",
			"provider", s.provider,
			"model", cfg.Model,
			"input_audio_format", inFmt,
			"output_audio_format", outFmt,
			"input_rate_hz", s.inRateHz,
			"output_rate_hz", s.outRateHz,
		)
	}
	go s.readLoop()
	return s, nil
}

// BindCallLog 绑定通话级日志（可选）。
func (s *Session) BindCallLog(log *slog.Logger, callID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.log = log
	s.callID = callID
	s.mu.Unlock()
}

func (s *Session) readLoop() {
	for {
		_, data, err := s.conn.Read(context.Background())
		if err != nil {
			return
		}
		var env struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &env) != nil {
			continue
		}
		switch env.Type {
		case "response.audio.delta", "response.output_audio.delta":
			if env.Delta == "" {
				continue
			}
			pcm, err := base64.StdEncoding.DecodeString(env.Delta)
			if err != nil {
				continue
			}
			s.outBytes.Add(int64(len(pcm)))
			s.outChunks.Add(1)
			s.outSamples.Add(int64(len(pcm) / 2))
			if s.outChunks.Load() == 1 && s.log != nil {
				s.mu.Lock()
				callID := s.callID
				outHz := s.outRateHz
				s.mu.Unlock()
				ms := int64(len(pcm)/2) * 1000 / int64(outHz)
				if outHz <= 0 {
					ms = 0
				}
				s.log.Info("aibot.realtime.audio_first_delta",
					"call_id", callID,
					"bytes", len(pcm),
					"pcm_samples", len(pcm)/2,
					"implied_ms_at_output_rate", ms,
					"output_rate_hz", outHz,
				)
			}
			s.mu.Lock()
			fn := s.onAudio
			s.mu.Unlock()
			if fn != nil {
				fn(pcm)
			}
		case "response.audio.done", "response.output_audio.done", "response.done", "response.completed":
			s.logAudioSummary(env.Type)
		case "error":
			return
		}
	}
}

func (s *Session) logAudioSummary(reason string) {
	if s == nil || s.log == nil {
		return
	}
	bytes := s.outBytes.Load()
	chunks := s.outChunks.Load()
	samples := s.outSamples.Load()
	s.mu.Lock()
	callID := s.callID
	outHz := s.outRateHz
	prov := s.provider
	s.mu.Unlock()
	var durMs int64
	if outHz > 0 {
		durMs = samples * 1000 / int64(outHz)
	}
	s.log.Info("aibot.realtime.audio_summary",
		"call_id", callID,
		"provider", prov,
		"reason", reason,
		"output_rate_hz", outHz,
		"chunks", chunks,
		"pcm_bytes", bytes,
		"pcm_samples", samples,
		"implied_audio_ms", durMs,
	)
}

func (s *Session) OnAudio(fn func([]byte)) {
	s.mu.Lock()
	s.onAudio = fn
	s.mu.Unlock()
}

// AppendInputPCM 发送 24kHz/16k mono PCM16（调用方负责重采样）。
func (s *Session) AppendInputPCM(ctx context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	msg := map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": base64.StdEncoding.EncodeToString(pcm),
	}
	return s.writeJSON(ctx, msg)
}

func (s *Session) writeJSON(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.conn.Write(ctx, websocket.MessageText, b)
}

func (s *Session) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close(websocket.StatusNormalClosure, "done")
}

// PCM16BytesToSamples 小端 PCM16 转 int16 样本。
func PCM16BytesToSamples(b []byte) []int16 {
	if len(b) < 2 {
		return nil
	}
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(int(b[2*i]) | int(b[2*i+1])<<8)
	}
	return out
}

// RealtimeInputRate 返回 provider 约定的上行 PCM 采样率（通义 16kHz，OpenAI 24kHz）。
func RealtimeInputRate(cfg config.AibotOpenAIConfig) int {
	if isQwenRealtime(cfg) {
		return 16000
	}
	return 24000
}

// RealtimeOutputRate 返回 provider 约定的下行 PCM 采样率。
func RealtimeOutputRate(cfg config.AibotOpenAIConfig) int {
	if isQwenRealtime(cfg) {
		return 24000
	}
	return 24000
}

func sessionAudioFormats(cfg config.AibotOpenAIConfig) (input, output string) {
	if isQwenRealtime(cfg) {
		return "pcm", "pcm"
	}
	return "pcm16", "pcm16"
}

func ValidateConfig(cfg config.AibotOpenAIConfig) error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("openai_realtime.base_url 必填")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("openai_realtime.api_key 必填")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("openai_realtime.model 必填")
	}
	return nil
}

// realtimeWSURL 由配置的 base_url 构造 WebSocket 地址（https→wss），不预设官方域名。
func dialHeaders(cfg config.AibotOpenAIConfig) http.Header {
	h := http.Header{
		"Authorization": []string{"Bearer " + cfg.APIKey},
	}
	if isQwenRealtime(cfg) {
		h.Set("x-dashscope-dataInspection", "disable")
		return h
	}
	h.Set("OpenAI-Beta", "realtime=v1")
	return h
}

func isQwenRealtime(cfg config.AibotOpenAIConfig) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "qwen", "dashscope", "aliyun":
		return true
	case "openai":
		return false
	}
	base := strings.ToLower(cfg.BaseURL)
	return strings.Contains(base, "qianwenaiapi.com") ||
		strings.Contains(base, "dashscope.aliyuncs.com") ||
		strings.Contains(base, "maas.aliyuncs.com")
}

func realtimeWSURL(cfg config.AibotOpenAIConfig) (string, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		return "", fmt.Errorf("openai_realtime.base_url 必填")
	}
	switch {
	case strings.HasPrefix(base, "https://"):
		base = "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}
	base = strings.TrimRight(base, "/")
	model := strings.TrimSpace(cfg.Model)
	if strings.Contains(base, "/v1/realtime") {
		if strings.Contains(base, "model=") || model == "" {
			return base, nil
		}
		sep := "?"
		if strings.Contains(base, "?") {
			sep = "&"
		}
		return base + sep + "model=" + url.QueryEscape(model), nil
	}
	if model == "" {
		return "", fmt.Errorf("openai_realtime.model 必填")
	}
	return base + "/v1/realtime?model=" + url.QueryEscape(model), nil
}
