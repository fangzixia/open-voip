package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"open-call/internal/config"
)

type aliyunProvider struct {
	cfg        config.TTSAliyunConfig
	sampleRate int
	client     *http.Client
	mu         sync.Mutex
	nlsToken   string
	tokenExp   time.Time
}

func newAliyun(cfg config.TTSAliyunConfig, sampleRate int) (Provider, error) {
	if strings.TrimSpace(cfg.AppKey) == "" {
		return nil, fmt.Errorf("aliyun.app_key ????")
	}
	return &aliyunProvider{
		cfg:        cfg,
		sampleRate: sampleRate,
		client:     &http.Client{Timeout: 120 * time.Second},
	}, nil
}

func (p *aliyunProvider) DefaultVoice() string {
	return strings.TrimSpace(p.cfg.Voice)
}

func (p *aliyunProvider) nlsAccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.nlsToken != "" && time.Now().Before(p.tokenExp.Add(-60*time.Second)) {
		return p.nlsToken, nil
	}
	url, err := aliyunMetaSignedURL(p.cfg.MetaURL, p.cfg.AccessKeyID, p.cfg.AccessKeySecret)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	res, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("??? CreateToken ?? HTTP %d: %s", res.StatusCode, trimErr(body))
	}
	var parsed struct {
		Token struct {
			ID         string `json:"Id"`
			ExpireTime int64  `json:"ExpireTime"`
		} `json:"Token"`
		ErrMsg string `json:"ErrMsg"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if parsed.Token.ID == "" {
		return "", fmt.Errorf("??? CreateToken ? Token: %s", parsed.ErrMsg)
	}
	p.nlsToken = parsed.Token.ID
	if parsed.Token.ExpireTime > 0 {
		p.tokenExp = time.Unix(parsed.Token.ExpireTime, 0)
	} else {
		p.tokenExp = time.Now().Add(23 * time.Hour)
	}
	return p.nlsToken, nil
}

func (p *aliyunProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, string, error) {
	tok, err := p.nlsAccessToken(ctx)
	if err != nil {
		return nil, "", err
	}
	voice = strings.TrimSpace(voice)
	if voice == "" {
		voice = p.DefaultVoice()
	}
	payload := map[string]any{
		"appkey":      p.cfg.AppKey,
		"text":        text,
		"format":      strings.TrimSpace(p.cfg.Format),
		"sample_rate": p.sampleRate,
		"voice":       voice,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	gateway := strings.TrimRight(p.cfg.GatewayURL, "/")
	url := gateway + "/stream/v1/tts"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NLS-Token", tok)
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
		return nil, "", fmt.Errorf("????????? HTTP %d: %s", res.StatusCode, trimErr(raw))
	}
	ct := res.Header.Get("Content-Type")
	if ct == "" {
		ct = "audio/wav"
	}
	return raw, ct, nil
}

func trimErr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[:400]
	}
	return s
}
