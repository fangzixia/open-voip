package switchapi

import (
	"context"
	"fmt"
	"net/http"

	"open-call/internal/ports"
)

func (c *Client) HoldLeg(ctx context.Context, callID, legID string, on bool) error {
	body := map[string]any{"on": on}
	applyMutation(ctx, body)
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/switch/v1/calls/%s/legs/%s/hold", callID, legID), body, nil)
}

func (c *Client) RejectLeg(ctx context.Context, callID, legID, reason string) error {
	body := map[string]any{"reason": reason}
	applyMutation(ctx, body)
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/switch/v1/calls/%s/legs/%s/reject", callID, legID), body, nil)
}

func (c *Client) StartLegPlayback(ctx context.Context, callID, legID, assetID string) (string, error) {
	body := map[string]any{"asset_id": assetID}
	applyMutation(ctx, body)
	var out map[string]string
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/switch/v1/calls/%s/legs/%s/playbacks", callID, legID), body, &out); err != nil {
		return "", err
	}
	return out["playback_id"], nil
}

func (c *Client) StopLegPlayback(ctx context.Context, callID, legID, playbackID string) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/switch/v1/calls/%s/legs/%s/playbacks/%s", callID, legID, playbackID), nil, nil)
}

func (c *Client) ReplaceBridge(ctx context.Context, callID, bridgeID, legA, legB string) error {
	body := map[string]any{"leg_ids": []string{legA, legB}, "leg_a": legA, "leg_b": legB}
	applyMutation(ctx, body)
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/switch/v1/calls/%s/bridges/%s", callID, bridgeID), body, nil)
}

func (c *Client) EndBridge(ctx context.Context, callID, bridgeID string) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/switch/v1/calls/%s/bridges/%s", callID, bridgeID), nil, nil)
}

func (c *Client) BridgeCall(ctx context.Context, callID, legA, legB string) error {
	body := map[string]any{"leg_ids": []string{legA, legB}, "leg_a": legA, "leg_b": legB}
	applyMutation(ctx, body)
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/switch/v1/calls/%s/bridges", callID), body, nil)
}

func (c *Client) ListOpenCalls(ctx context.Context) ([]ports.CallView, error) {
	var out struct {
		Items []ports.CallView `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/switch/v1/calls?status=open", nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}
