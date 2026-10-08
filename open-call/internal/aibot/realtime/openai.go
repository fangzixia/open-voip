package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"open-call/internal/config"
)

// Session 与 OpenAI/通义 Realtime WebSocket 的双向音频会话。
type Session struct {
	conn  *websocket.Conn
	mu    sync.Mutex
	done  chan error
	start sync.Once

	log       *slog.Logger // 可选，由 BindCallLog 注入
	callID    string
	provider  string // qwen / openai 等
	inRateHz  int    // 本端假定上行采样率
	outRateHz int    // 本端假定下行采样率

	outBytes   atomic.Int64 // 累计下行 PCM 字节
	outChunks  atomic.Int64 // 下行 delta 包数
	outSamples atomic.Int64 // 累计 PCM 样本数（16-bit）
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
		done:      make(chan error, 1),
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

// OutputCallbacks express conversation decisions, without phone media processing.
type OutputCallbacks struct {
	Audio  func(uint64, []byte) error
	Clear  func(uint64) error
	Finish func(uint64) error
}

func (s *Session) Done() <-chan error { return s.done }

type outputChunk struct {
	generation uint64
	pcm        []byte
	finish     bool
}

func (s *Session) Start(ctx context.Context, cb OutputCallbacks) {
	s.start.Do(func() {
		ctx, cancel := context.WithCancel(ctx)
		// Bounded provider handoff lets VAD clear playback while a send waits for credit.
		chunks := make(chan outputChunk, 100) // at most 10 seconds of 100 ms model blocks
		result := make(chan error, 2)
		go func() {
			defer cancel()
			result <- s.readLoop(ctx, OutputCallbacks{
				Clear: cb.Clear,
				Audio: func(g uint64, b []byte) error {
					for len(b) > 0 {
						n := min(len(b), s.outRateHz/10*2)
						part := append([]byte(nil), b[:n]...)
						select {
						case chunks <- outputChunk{generation: g, pcm: part}:
						default:
							return errors.New("model audio handoff exceeds 10 seconds")
						}
						b = b[n:]
					}
					return nil
				},
				Finish: func(g uint64) error {
					select {
					case chunks <- outputChunk{generation: g, finish: true}:
						return nil
					default:
						return errors.New("model audio handoff full")
					}
				},
			})
		}()
		go func() {
			defer cancel()
			for {
				select {
				case <-ctx.Done():
					result <- ctx.Err()
					return
				case c := <-chunks:
					var err error
					if c.finish {
						err = cb.Finish(c.generation)
					} else {
						err = cb.Audio(c.generation, c.pcm)
					}
					if err != nil {
						result <- err
						return
					}
				}
			}
		}()
		go func() { err := <-result; cancel(); s.done <- err }()
	})
}

// Provider response IDs prevent late deltas from a canceled response from replaying.
func (s *Session) readLoop(ctx context.Context, cb OutputCallbacks) error {
	generation := uint64(1)
	current := ""
	blocked := map[string]bool{}
	interrupted := false
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			return err
		}
		var env struct {
			Type       string `json:"type"`
			Delta      string `json:"delta"`
			ResponseID string `json:"response_id"`
			Response   struct {
				ID string `json:"id"`
			} `json:"response"`
		}
		if json.Unmarshal(data, &env) != nil {
			continue
		}
		id := env.ResponseID
		if id == "" {
			id = env.Response.ID
		}
		switch env.Type {
		case "input_audio_buffer.speech_started":
			if current != "" {
				blocked[current] = true
			}
			interrupted = true
			generation++
			if err := cb.Clear(generation); err != nil {
				return err
			}
		case "response.created":
			if id == "" || blocked[id] {
				continue
			}
			current = id
			interrupted = false
			generation++
			if err := cb.Clear(generation); err != nil {
				return err
			}
		case "response.audio.delta", "response.output_audio.delta":
			if interrupted || (id != "" && blocked[id]) {
				continue
			}
			if current != "" && id != "" && id != current {
				continue
			}
			if current == "" {
				current = id
			}
			pcm, err := base64.StdEncoding.DecodeString(env.Delta)
			if err != nil {
				return err
			}
			if len(pcm)%2 != 0 {
				return fmt.Errorf("model returned incomplete PCM16 sample")
			}
			s.outBytes.Add(int64(len(pcm)))
			s.outChunks.Add(1)
			s.outSamples.Add(int64(len(pcm) / 2))
			// Providers can return large deltas. Only split transport blocks; Switch paces them.
			for len(pcm) > 0 {
				n := min(len(pcm), s.outRateHz*2)
				if err := cb.Audio(generation, pcm[:n]); err != nil {
					return err
				}
				pcm = pcm[n:]
			}
		case "response.audio.done", "response.output_audio.done":
			if interrupted || (id != "" && (blocked[id] || id != current)) {
				continue
			}
			if err := cb.Finish(generation); err != nil {
				return err
			}
			s.logAudioSummary(env.Type)
		case "error":
			return fmt.Errorf("model realtime session error")
		}
	}
}

// logAudioSummary 在一段 TTS 结束时输出累计 PCM 统计，用于核对采样率假设。
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

// AppendInputPCM 发送 24kHz/16k mono PCM16（Switch 按连接约定提供采样率）。
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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.conn.Write(ctx, websocket.MessageText, b)
}

// Close 正常关闭 Realtime WebSocket。
func (s *Session) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close(websocket.StatusNormalClosure, "done")
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
