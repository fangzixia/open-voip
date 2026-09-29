package tts

import (
	"context"
	"fmt"

	"open-call/internal/config"
)

// Engine 封装合成与 IVR 素材 WAV 规范化。
type Engine struct {
	cfg      config.TTSConfig
	provider Provider
	ffmpeg   string
}

// NewEngine 在 TTS 启用时构造引擎；未启用返回 (nil, nil)。ffmpegBin 为启动时已解析的 FFmpeg 路径。
func NewEngine(cfg config.TTSConfig, ffmpegBin string) (*Engine, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	p, err := NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &Engine{cfg: cfg, provider: p, ffmpeg: ffmpegBin}, nil
}

// Enabled 是否已配置并可用。
func (e *Engine) Enabled() bool {
	return e != nil && e.provider != nil
}

// Config 返回只读配置快照（供 tts-options）。
func (e *Engine) Config() config.TTSConfig {
	if e == nil {
		return config.TTSConfig{}
	}
	return e.cfg
}

// DefaultVoice 当前 provider 默认发音人。
func (e *Engine) DefaultVoice() string {
	if e == nil || e.provider == nil {
		return ""
	}
	return e.provider.DefaultVoice()
}

// SynthesizeToPromptWAV 合成文本并转为 8/16 kHz 单声道 PCM WAV。
func (e *Engine) SynthesizeToPromptWAV(ctx context.Context, text, voice string) ([]byte, error) {
	if !e.Enabled() {
		return nil, fmt.Errorf("TTS 未配置")
	}
	raw, _, err := e.provider.Synthesize(ctx, text, voice)
	if err != nil {
		return nil, err
	}
	return NormalizeToPromptWAV(ctx, e.ffmpeg, raw, e.cfg.SampleRate)
}
