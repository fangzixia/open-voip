package switchapi

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"open-call/internal/errs"
	"open-call/internal/httpapi"
	"path/filepath"
	"time"
)

// recordingPath 构造 Switch 内部录音下载/删除 API 路径。
func recordingPath(callID, id, path string) string {
	return "/switch/v1/internal/recordings/" + url.PathEscape(callID) + "/" + url.PathEscape(id) + "?ext=" + url.QueryEscape(filepath.Ext(path))
}

// OpenRecording 从 Switch 拉取录音文件流。
func (c *Client) OpenRecordingAs(ctx context.Context, callID, id, path, format string) (io.ReadCloser, error) {
	endpoint := recordingPath(callID, id, path)
	if format != "" {
		endpoint += "&format=" + url.QueryEscape(format)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	setTraceHeaders(req, ctx)
	client := &http.Client{Transport: c.http.Transport, Timeout: 10 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		if err := httpapi.Decode(res, nil); err != nil {
			return nil, err
		}
		return nil, errs.Internal("交换服务返回了非预期的录音响应")
	}
	return res.Body, nil
}

// DeleteRecording 请求 Switch 删除指定录音文件。
func (c *Client) DeleteRecording(ctx context.Context, callID, id, path string) error {
	return c.do(ctx, http.MethodDelete, recordingPath(callID, id, path), nil, nil)
}
