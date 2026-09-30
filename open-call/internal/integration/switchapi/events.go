package switchapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Event 是 Switch /switch/v1/events 游标 API 返回的一条持久化呼叫事件。
type Event struct {
	Version    int64          `json:"version"`
	ID         int64          `json:"id"`
	CallID     string         `json:"call_id"`
	Seq        int64          `json:"seq"`
	AgentID    string         `json:"agent_id"`
	TargetOnly bool           `json:"target_only"`
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  string         `json:"created_at"`
}

// ListEvents 按 after_id 增量拉取事件（运维对账；主路径为 Switch HTTP callback）。
func (c *Client) ListEvents(ctx context.Context, afterID int64) ([]Event, error) {
	var out struct {
		Items []Event `json:"items"`
	}
	q := url.Values{"after_id": {strconv.FormatInt(afterID, 10)}, "limit": {"100"}}
	err := c.do(ctx, http.MethodGet, "/switch/v1/events?"+q.Encode(), nil, &out)
	return out.Items, err
}
