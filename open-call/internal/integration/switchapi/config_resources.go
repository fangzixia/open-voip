package switchapi

import (
	"context"
	"net/http"
	"net/url"

	"open-call/internal/ports"
)

func (c *Client) GetActiveConfiguration(ctx context.Context) (ports.SwitchActiveConfiguration, error) {
	var out ports.SwitchActiveConfiguration
	err := c.do(ctx, http.MethodGet, "/switch/v1/configuration/active", nil, &out)
	return out, err
}

func (c *Client) GetActiveConfigurationSummary(ctx context.Context) (ports.SwitchConfigVersion, error) {
	var out ports.SwitchConfigVersion
	err := c.do(ctx, http.MethodGet, "/switch/v1/configuration/active/summary", nil, &out)
	return out, err
}

func listItems[T any](c *Client, ctx context.Context, path string) ([]T, error) {
	var wrap struct {
		Items []T `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &wrap); err != nil {
		return nil, err
	}
	if wrap.Items == nil {
		return []T{}, nil
	}
	return wrap.Items, nil
}

func (c *Client) ListQueueConfigs(ctx context.Context) ([]ports.SwitchQueueConfig, error) {
	return listItems[ports.SwitchQueueConfig](c, ctx, "/switch/v1/queues")
}

func (c *Client) CreateQueueConfig(ctx context.Context, in ports.SwitchQueueConfig) (ports.SwitchQueueConfig, error) {
	var out ports.SwitchQueueConfig
	err := c.do(ctx, http.MethodPost, "/switch/v1/queues", in, &out)
	return out, err
}

func (c *Client) GetQueueConfig(ctx context.Context, id string) (ports.SwitchQueueConfig, error) {
	var out ports.SwitchQueueConfig
	err := c.do(ctx, http.MethodGet, "/switch/v1/queues/"+url.PathEscape(id)+"/config", nil, &out)
	return out, err
}

func (c *Client) UpdateQueueConfig(ctx context.Context, id string, in ports.SwitchQueueConfig) (ports.SwitchQueueConfig, error) {
	var out ports.SwitchQueueConfig
	err := c.do(ctx, http.MethodPatch, "/switch/v1/queues/"+url.PathEscape(id)+"/config", in, &out)
	return out, err
}

func (c *Client) DeleteQueueConfig(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/switch/v1/queues/"+url.PathEscape(id)+"/config", nil, nil)
}

func (c *Client) SetQueueAgents(ctx context.Context, queueID string, agentIDs []string) (ports.SwitchQueueConfig, error) {
	var out ports.SwitchQueueConfig
	err := c.do(ctx, http.MethodPut, "/switch/v1/queues/"+url.PathEscape(queueID)+"/agents", map[string]any{"agent_ids": agentIDs}, &out)
	return out, err
}

func (c *Client) SetQueueSkills(ctx context.Context, queueID string, skillIDs []string) (ports.SwitchQueueConfig, error) {
	var out ports.SwitchQueueConfig
	err := c.do(ctx, http.MethodPut, "/switch/v1/queues/"+url.PathEscape(queueID)+"/skills", map[string]any{"skill_ids": skillIDs}, &out)
	return out, err
}

func (c *Client) ListSkillConfigs(ctx context.Context) ([]ports.SwitchSkillConfig, error) {
	return listItems[ports.SwitchSkillConfig](c, ctx, "/switch/v1/skills")
}

func (c *Client) CreateSkillConfig(ctx context.Context, in ports.SwitchSkillConfig) (ports.SwitchSkillConfig, error) {
	var out ports.SwitchSkillConfig
	err := c.do(ctx, http.MethodPost, "/switch/v1/skills", in, &out)
	return out, err
}

func (c *Client) UpdateSkillConfig(ctx context.Context, id, name string) (ports.SwitchSkillConfig, error) {
	var out ports.SwitchSkillConfig
	err := c.do(ctx, http.MethodPatch, "/switch/v1/skills/"+url.PathEscape(id), map[string]string{"name": name}, &out)
	return out, err
}

func (c *Client) DeleteSkillConfig(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/switch/v1/skills/"+url.PathEscape(id), nil, nil)
}

func (c *Client) ListAgentConfigs(ctx context.Context) ([]ports.SwitchAgentConfig, error) {
	return listItems[ports.SwitchAgentConfig](c, ctx, "/switch/v1/agents/config")
}

func (c *Client) UpsertAgentConfig(ctx context.Context, in ports.SwitchAgentConfig) (ports.SwitchAgentConfig, error) {
	var out ports.SwitchAgentConfig
	err := c.do(ctx, http.MethodPut, "/switch/v1/agents/"+url.PathEscape(in.ID)+"/config", in, &out)
	return out, err
}

func (c *Client) DeleteAgentConfig(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/switch/v1/agents/"+url.PathEscape(id)+"/config", nil, nil)
}

func (c *Client) SetAgentSkills(ctx context.Context, agentID string, skillIDs []string) (ports.SwitchAgentConfig, error) {
	var out ports.SwitchAgentConfig
	err := c.do(ctx, http.MethodPut, "/switch/v1/agents/"+url.PathEscape(agentID)+"/skills", map[string]any{"skill_ids": skillIDs}, &out)
	return out, err
}

func (c *Client) ListDIDConfigs(ctx context.Context) ([]ports.SwitchDIDConfig, error) {
	return listItems[ports.SwitchDIDConfig](c, ctx, "/switch/v1/did-routes")
}

func (c *Client) UpsertDIDConfig(ctx context.Context, in ports.SwitchDIDConfig) (ports.SwitchDIDConfig, error) {
	var out ports.SwitchDIDConfig
	err := c.do(ctx, http.MethodPost, "/switch/v1/did-routes", in, &out)
	return out, err
}

func (c *Client) DeleteDIDConfig(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/switch/v1/did-routes/"+url.PathEscape(id), nil, nil)
}

func (c *Client) ListIVRFlows(ctx context.Context) ([]ports.SwitchIVRPublishedView, error) {
	return listItems[ports.SwitchIVRPublishedView](c, ctx, "/switch/v1/ivr/flows")
}

func (c *Client) GetIVRFlow(ctx context.Context, flowID string) (ports.SwitchIVRPublishedView, error) {
	var out ports.SwitchIVRPublishedView
	err := c.do(ctx, http.MethodGet, "/switch/v1/ivr/flows/"+url.PathEscape(flowID), nil, &out)
	return out, err
}

func (c *Client) ValidateIVRFlowPayload(ctx context.Context, payloadJSON string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/ivr/flows/validate", map[string]string{"payload_json": payloadJSON}, nil)
}

func (c *Client) UpsertIVRFlow(ctx context.Context, flowID, payloadJSON string) (ports.SwitchIVRPublishedView, error) {
	var out ports.SwitchIVRPublishedView
	path := "/switch/v1/ivr/flows"
	method := http.MethodPost
	body := map[string]string{"payload_json": payloadJSON}
	if flowID != "" {
		path = "/switch/v1/ivr/flows/" + url.PathEscape(flowID)
		method = http.MethodPut
	}
	err := c.do(ctx, method, path, body, &out)
	return out, err
}

func (c *Client) DeleteIVRFlow(ctx context.Context, flowID string) error {
	return c.do(ctx, http.MethodDelete, "/switch/v1/ivr/flows/"+url.PathEscape(flowID), nil, nil)
}
