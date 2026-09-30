package switchapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"open-call/internal/ports"
)

func (c *Client) StoreConfig(ctx context.Context, bundle ports.SwitchConfigBundle) (ports.SwitchConfigVersion, error) {
	var out ports.SwitchConfigVersion
	err := c.do(ctx, http.MethodPost, "/switch/v1/configuration/versions", bundle, &out)
	return out, err
}

func (c *Client) ActivateConfig(ctx context.Context, version int64) (ports.SwitchConfigVersion, error) {
	var out ports.SwitchConfigVersion
	path := "/switch/v1/configuration/versions/" + strconv.FormatInt(version, 10) + "/activate"
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out, err
}

func (c *Client) CheckIn(ctx context.Context, agentID string, queueIDs []string) (ports.SwitchAgentSession, error) {
	var out ports.SwitchAgentSession
	err := c.do(ctx, http.MethodPost, "/switch/v1/agents/"+url.PathEscape(agentID)+"/check-in", map[string]any{"queue_ids": queueIDs}, &out)
	return out, err
}

func (c *Client) CheckOut(ctx context.Context, agentID string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/agents/"+url.PathEscape(agentID)+"/check-out", nil, nil)
}

func (c *Client) SetPresence(ctx context.Context, agentID, state, reason string) (ports.SwitchAgentSession, error) {
	var out ports.SwitchAgentSession
	err := c.do(ctx, http.MethodPut, "/switch/v1/agents/"+url.PathEscape(agentID)+"/presence", map[string]string{"state": state, "reason": reason}, &out)
	return out, err
}

func (c *Client) AgentSession(ctx context.Context, agentID string) (ports.SwitchAgentSession, error) {
	var out ports.SwitchAgentSession
	err := c.do(ctx, http.MethodGet, "/switch/v1/agents/"+url.PathEscape(agentID)+"/session", nil, &out)
	return out, err
}

var _ ports.SwitchAdminPort = (*Client)(nil)
