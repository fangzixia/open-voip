package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"open-call/internal/config"
)

type openAIProvider struct {
	cfg    config.TTSOpenAICompatibleConfig
	client *http.Client
	url    string
}

func newOpenAICompatible(cfg config.TTSOpenAICompatibleConfig) (Provider, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("openai_compatible.base_url ????")
	}
	return &openAIProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: 120 * time.Second},
		url:    base + "/v1/audio/speech",
	}, nil
}

func (p *openAIProvider) DefaultVoice() string {
	return strings.TrimSpace(p.cfg.Voice)
}

func (p *openAIProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, string, error) {
	voice = strings.TrimSpace(voice)
	if voice == "" {
		voice = p.DefaultVoice()
	}
	body := map[string]string{
		"model": p.cfg.Model,
		"input": text,
	}
	if voice != "" {
		body["voice"] = voice
	}
	if f := strings.TrimSpace(p.cfg.ResponseFormat); f != "" {
		body["response_format"] = f
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.cfg.APIKey))
	for k, v := range p.cfg.ExtraHeaders {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
	res, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return nil, "", fmt.Errorf("????????? HTTP %d: %s", res.StatusCode, msg)
	}
	ct := res.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	return raw, ct, nil
}
