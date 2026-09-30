package switchapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"open-call/internal/ports/dto"
)

func (c *Client) JoinWebRTCOffer(ctx context.Context, callID, legID string) (map[string]string, error) {
	var out map[string]string
	err := c.do(ctx, http.MethodPost, fmt.Sprintf("/switch/v1/calls/%s/legs/%s/offer", callID, legID), nil, &out)
	return out, err
}

func (c *Client) AcceptLegAnswer(ctx context.Context, callID, legID, sdp, typ string) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/switch/v1/calls/%s/legs/%s/answer", callID, legID), map[string]string{
		"sdp": sdp, "type": typ,
	}, nil)
}

func (c *Client) TrickleLegICE(ctx context.Context, callID, legID string, body map[string]any) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/switch/v1/calls/%s/legs/%s/ice", callID, legID), body, nil)
}

func (c *Client) MuteLeg(ctx context.Context, callID, legID string, body map[string]any) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/switch/v1/calls/%s/legs/%s/mute", callID, legID), body, nil)
}

func (c *Client) TURNCredentials(ctx context.Context, callID, subject string) (dto.TURNConfig, error) {
	var out dto.TURNConfig
	path := fmt.Sprintf("/switch/v1/calls/%s/turn-credentials?subject=%s", callID, url.QueryEscape(subject))
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
