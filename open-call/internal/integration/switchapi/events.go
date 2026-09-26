package switchapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

type Event struct {
	ID         int64          `json:"id"`
	CallID     string         `json:"call_id"`
	Seq        int64          `json:"seq"`
	AgentID    string         `json:"agent_id"`
	TargetOnly bool           `json:"target_only"`
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  string         `json:"created_at"`
}

func (c *Client) ListEvents(ctx context.Context, afterID int64) ([]Event, error) {
	var out struct {
		Items []Event `json:"items"`
	}
	q := url.Values{"after_id": {strconv.FormatInt(afterID, 10)}, "limit": {"100"}}
	err := c.do(ctx, http.MethodGet, "/switch/v1/events?"+q.Encode(), nil, &out)
	return out.Items, err
}
