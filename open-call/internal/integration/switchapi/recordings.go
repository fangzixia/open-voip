package switchapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
)

// recordingPath 构造 Switch 内部录音下载/删除 API 路径。
func recordingPath(callID, id, path string) string {
	return "/switch/v1/internal/recordings/" + url.PathEscape(callID) + "/" + url.PathEscape(id) + "?ext=" + url.QueryEscape(filepath.Ext(path))
}

// OpenRecording 从 Switch 拉取录音文件流。
func (c *Client) OpenRecording(ctx context.Context, callID, id, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+recordingPath(callID, id, path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("交换服务读取录音失败: HTTP %d", res.StatusCode)
	}
	return res.Body, nil
}

// DeleteRecording 请求 Switch 删除指定录音文件。
func (c *Client) DeleteRecording(ctx context.Context, callID, id, path string) error {
	return c.do(ctx, http.MethodDelete, recordingPath(callID, id, path), nil, "", nil)
}
