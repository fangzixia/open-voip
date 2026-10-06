package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"open-call/internal/config"
)

// Session OpenAI Realtime 双向音频会话。
type Session struct {
	conn *websocket.Conn
	mu   sync.Mutex
	onAudio func(pcm []byte)
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
	s := &Session{conn: conn}
	update := map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"modalities":        []string{"text", "audio"},
			"instructions":      systemPrompt,
			"voice":             cfg.Voice,
			"input_audio_format":  "pcm16",
			"output_audio_format": "pcm16",
			"turn_detection":    map[string]any{"type": "server_vad"},
		},
	}
	if err := s.writeJSON(ctx, update); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "session.update failed")
		return nil, err
	}
	go s.readLoop()
	return s, nil
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
			s.mu.Lock()
			fn := s.onAudio
			s.mu.Unlock()
			if fn != nil {
				fn(pcm)
			}
		case "error":
			return
		}
	}
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

// ResampleSimple 简单线性重采样（单声道）。
func ResampleSimple(in []int16, fromRate, toRate int) []int16 {
	if fromRate <= 0 || toRate <= 0 || fromRate == toRate {
		return in
	}
	outLen := len(in) * toRate / fromRate
	if outLen == 0 {
		return nil
	}
	out := make([]int16, outLen)
	for i := range out {
		src := i * fromRate / toRate
		if src >= len(in) {
			src = len(in) - 1
		}
		out[i] = in[src]
	}
	return out
}

// RealtimeInputRate OpenAI Realtime 默认输入采样率。
const RealtimeInputRate = 24000

// RealtimeOutputRate 输出 PCM 采样率。
const RealtimeOutputRate = 24000

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
