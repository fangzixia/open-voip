package switchapi

import (
	"context"
	"net/http"
	"open-call/internal/ports"
)

// ListCalls 读取 Switch 的权威运行时，不直接连接交换数据库。
func (c *Client) ListCalls(ctx context.Context) ([]ports.CallView, error) {
	var out []ports.CallView
	err := c.do(ctx, http.MethodGet, "/switch/v1/internal/calls", nil, &out)
	return out, err
}
