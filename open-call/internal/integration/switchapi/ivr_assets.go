package switchapi

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"open-call/internal/httpapi"
	"open-call/internal/observability"
)

// IVRAsset Switch 侧已上传的语音素材元数据。
type IVRAsset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// UploadIVRAsset 将 WAV 上传至 Switch prompts 存储（与浏览器直传 multipart 等价）。
func (c *Client) UploadIVRAsset(ctx context.Context, filename string, wav []byte) (IVRAsset, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return IVRAsset{}, err
	}
	if _, err := io.Copy(part, bytes.NewReader(wav)); err != nil {
		return IVRAsset{}, err
	}
	if err := w.Close(); err != nil {
		return IVRAsset{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/switch/v1/ivr-assets", &buf)
	if err != nil {
		return IVRAsset{}, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	setTraceHeaders(req, ctx)
	observability.Emit(ctx, "switch.request.started", map[string]any{"method": http.MethodPost, "path": "/switch/v1/ivr-assets"})
	client := &http.Client{Timeout: 120 * time.Second}
	if c.http != nil && c.http.Transport != nil {
		client.Transport = c.http.Transport
	}
	res, err := client.Do(req)
	if err != nil {
		observability.Emit(ctx, "switch.request.failed", map[string]any{"method": http.MethodPost, "path": "/switch/v1/ivr-assets", "error": err.Error()})
		return IVRAsset{}, err
	}
	defer func() { _ = res.Body.Close() }()
	var out IVRAsset
	err = httpapi.Decode(res, &out)
	fields := map[string]any{"method": http.MethodPost, "path": "/switch/v1/ivr-assets", "status": res.StatusCode}
	if err != nil {
		fields["error"] = err.Error()
		observability.Emit(ctx, "switch.request.failed", fields)
		return IVRAsset{}, err
	}
	observability.Emit(ctx, "switch.request.completed", fields)
	return out, nil
}

// DownloadIVRAsset 下载 Switch 侧 IVR 素材 WAV（assetRef 可为 uuid 或 uuid.wav）。
func (c *Client) DownloadIVRAsset(ctx context.Context, assetRef string) ([]byte, error) {
	id := strings.TrimSpace(assetRef)
	id = strings.TrimSuffix(id, ".wav")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/switch/v1/ivr-assets/"+id, nil)
	if err != nil {
		return nil, err
	}
	setTraceHeaders(req, ctx)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, httpapi.Decode(res, nil)
	}
	return io.ReadAll(io.LimitReader(res.Body, 32<<20))
}
