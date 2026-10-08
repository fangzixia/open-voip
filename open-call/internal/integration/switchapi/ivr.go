package switchapi

import (
	"context"
	"net/http"
	"net/url"

	"open-call/internal/ports"
)

// EnterIVR 请求通用的客户 IVR 接续；业务流程选择由调用方负责。
func (c *Client) EnterIVR(ctx context.Context, callID, flowID string) (ports.CallView, error) {
	var view ports.CallView
	body := map[string]any{"flow_id": flowID}
	applyMutation(ctx, body)
	err := c.do(ctx, http.MethodPost, "/switch/v1/calls/"+url.PathEscape(callID)+"/ivr", body, &view)
	return view, err
}
