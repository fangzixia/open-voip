package http

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"open-call/internal/errs"
)

type ivrAssetSynthesizeRequest struct {
	Name  string `json:"name"`
	Text  string `json:"text"`
	Voice string `json:"voice"`
}

func (d RouterDeps) handleIVRAssetTTSOptions(w http.ResponseWriter, r *http.Request) {
	enabled := d.TTS != nil && d.TTS.Enabled()
	provider := strings.TrimSpace(d.Config.TTS.Provider)
	label := provider
	switch strings.ToLower(provider) {
	case "aliyun":
		label = "阿里云"
	case "xunfei":
		label = "讯飞"
	case "openai_compatible":
		label = "语音大模型"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":        enabled,
		"provider":       provider,
		"provider_label": label,
		"default_voice": func() string {
			if d.TTS == nil {
				return ""
			}
			return d.TTS.DefaultVoice()
		}(),
		"sample_rate": d.Config.TTS.SampleRate,
	})
}

func (d RouterDeps) handleIVRAssetSynthesize(w http.ResponseWriter, r *http.Request) {
	if d.TTS == nil || !d.TTS.Enabled() {
		writeErr(w, errs.InvalidRequest("未配置 TTS，请在 open-call 配置中启用 tts"))
		return
	}
	if d.Switch == nil {
		writeErr(w, errs.Unprocessable("交换服务未配置", ""))
		return
	}
	var req ivrAssetSynthesizeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, errs.InvalidRequest("请求体无效"))
		return
	}
	name := strings.TrimSpace(req.Name)
	text := strings.TrimSpace(req.Text)
	if name == "" {
		writeErr(w, errs.InvalidRequest("请填写素材名称"))
		return
	}
	if text == "" {
		writeErr(w, errs.InvalidRequest("请填写合成文本"))
		return
	}
	maxChars := d.Config.TTS.MaxTextChars
	if maxChars <= 0 {
		maxChars = 2000
	}
	if utf8.RuneCountInString(text) > maxChars {
		writeErr(w, errs.InvalidRequest("合成文本过长"))
		return
	}
	if len(name) > 160 {
		name = name[:160]
	}
	wav, err := d.TTS.SynthesizeToPromptWAV(r.Context(), text, strings.TrimSpace(req.Voice))
	if err != nil {
		writeErr(w, errs.InvalidRequest("语音合成失败: "+err.Error()))
		return
	}
	filename := name
	if !strings.HasSuffix(strings.ToLower(filename), ".wav") {
		filename = name + ".wav"
	}
	filename = filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	asset, err := d.Switch.UploadIVRAsset(r.Context(), filename, wav)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, asset)
}
