// Package tts 提供 IVR 素材文本转语音适配。
package tts

import (
	"context"
	"fmt"
	"strings"

	"open-call/internal/config"
)

// Provider 将文本合成为原始音频。
type Provider interface {
	Synthesize(ctx context.Context, text, voice string) (audio []byte, contentType string, err error)
	DefaultVoice() string
}

// NewProvider 按配置构造当前启用的 Provider。
func NewProvider(cfg config.TTSConfig) (Provider, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("TTS 未启用")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "aliyun":
		return newAliyun(cfg.Aliyun, cfg.SampleRate)
	case "xunfei":
		return newXunfei(cfg.Xunfei, cfg.SampleRate)
	case "openai_compatible":
		return newOpenAICompatible(cfg.OpenAI)
	default:
		return nil, fmt.Errorf("不支持的 tts.provider: %q", cfg.Provider)
	}
}
