package tts

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"open-call/internal/config"
)

type xunfeiProvider struct {
	cfg        config.TTSXunfeiConfig
	sampleRate int
}

func newXunfei(cfg config.TTSXunfeiConfig, sampleRate int) (Provider, error) {
	return &xunfeiProvider{cfg: cfg, sampleRate: sampleRate}, nil
}

func (p *xunfeiProvider) DefaultVoice() string {
	return strings.TrimSpace(p.cfg.Voice)
}

func (p *xunfeiProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, string, error) {
	voice = strings.TrimSpace(voice)
	if voice == "" {
		voice = p.DefaultVoice()
	}
	host := strings.TrimSpace(p.cfg.Host)
	if host == "" {
		host = "tts-api.xfyun.cn"
	}
	authURL, err := xunfeiWSAuthURL(host, p.cfg.APIKey, p.cfg.APISecret)
	if err != nil {
		return nil, "", err
	}
	dialCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(dialCtx, authURL, nil)
	if err != nil {
		if resp != nil && resp.Body != nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return nil, "", fmt.Errorf("讯飞 WebSocket 握手失败 HTTP %d: %s", resp.StatusCode, trimErr(body))
		}
		return nil, "", fmt.Errorf("讯飞 WebSocket 连接失败: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	aue := strings.TrimSpace(p.cfg.Aue)
	if aue == "" {
		aue = "lame"
	}
	business := map[string]any{
		"aue": aue,
		"vcn": voice,
		"tte": "utf8",
	}
	if strings.EqualFold(aue, "lame") {
		business["sfl"] = 1
	}
	if strings.EqualFold(aue, "raw") {
		business["auf"] = fmt.Sprintf("audio/L16;rate=%d", p.sampleRate)
	}
	reqBody := map[string]any{
		"common":   map[string]string{"app_id": p.cfg.AppID},
		"business": business,
		"data": map[string]any{
			"status": 2,
			"text":   base64.StdEncoding.EncodeToString([]byte(text)),
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", err
	}
	if err := conn.Write(dialCtx, websocket.MessageText, payload); err != nil {
		return nil, "", fmt.Errorf("讯飞发送合成请求失败: %w", err)
	}

	var audio []byte
	for {
		_, data, err := conn.Read(dialCtx)
		if err != nil {
			return nil, "", fmt.Errorf("讯飞读取合成结果失败: %w", err)
		}
		var parsed struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Audio  string `json:"audio"`
				Status int    `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, "", fmt.Errorf("讯飞响应解析失败: %w", err)
		}
		if parsed.Code != 0 {
			return nil, "", fmt.Errorf("讯飞语音合成错误 %d: %s", parsed.Code, parsed.Message)
		}
		if parsed.Data.Audio != "" {
			chunk, err := base64.StdEncoding.DecodeString(parsed.Data.Audio)
			if err != nil {
				return nil, "", err
			}
			audio = append(audio, chunk...)
		}
		if parsed.Data.Status == 2 {
			break
		}
	}
	if len(audio) == 0 {
		return nil, "", fmt.Errorf("讯飞返回空音频")
	}
	ct := "audio/mpeg"
	if strings.EqualFold(aue, "raw") {
		ct = "audio/L16"
	}
	return audio, ct, nil
}

func xunfeiWSAuthURL(host, apiKey, apiSecret string) (string, error) {
	path := "/v2/tts"
	date := time.Now().UTC().Format(http.TimeFormat)
	signOrigin := fmt.Sprintf("host: %s\ndate: %s\nGET %s HTTP/1.1", host, date, path)
	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(signOrigin))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	authOrigin := fmt.Sprintf(`api_key="%s", algorithm="hmac-sha256", headers="host date request-line", signature="%s"`,
		apiKey, signature)
	authorization := base64.StdEncoding.EncodeToString([]byte(authOrigin))
	u := url.URL{Scheme: "wss", Host: host, Path: path}
	q := u.Query()
	q.Set("host", host)
	q.Set("date", date)
	q.Set("authorization", authorization)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
