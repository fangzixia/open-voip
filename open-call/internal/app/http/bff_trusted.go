package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"open-call/internal/errs"
	"open-call/internal/integration/switchapi"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
	"strings"
)

// Switch 信任服务命令；最终用户的通话及媒体腿权限由 BFF 在转发前检查。
func prepareSwitchRequest(r *http.Request, p auth.Principal, client *switchapi.Client) error {
	path := r.URL.Path
	if path == "/switch/v1/calls/outbound" {
		if p.AgentID == "" || p.IsGuest() {
			return errs.Forbidden("仅坐席可外呼")
		}
		return setCommandFields(r, map[string]string{"agent_id": p.AgentID})
	}
	if strings.HasPrefix(path, "/switch/v1/ivr-assets") || strings.HasPrefix(path, "/switch/v1/supervisor/agents/") {
		return nil
	}
	if strings.HasPrefix(path, "/switch/v1/supervisor/calls/") {
		if p.AgentID == "" {
			return errs.Forbidden("班长需要坐席资料")
		}
		return setCommandFields(r, map[string]string{"agent_id": p.AgentID})
	}
	callID := switchCallID(path)
	if callID == "" {
		return nil
	}
	view, err := client.GetCall(r.Context(), callID)
	if err != nil {
		return err
	}
	if !ownsCall(p, callID, view) {
		return errs.Forbidden("不能操作该通话")
	}
	if strings.Contains(path, "/legs/") {
		parts := strings.Split(strings.TrimPrefix(path, "/switch/v1/calls/"), "/")
		if len(parts) >= 3 && !ownsLeg(p, parts[2], view) {
			return errs.Forbidden("不能操作他人的媒体腿")
		}
	}
	if strings.HasSuffix(path, "/turn-credentials") {
		subject := p.UserID
		if subject == "" {
			subject = p.GuestID
		}
		q := r.URL.Query()
		q.Set("subject", subject)
		r.URL.RawQuery = q.Encode()
		return nil
	}
	if !strings.Contains(path, "/legs/") && (strings.HasSuffix(path, "/answer") || strings.HasSuffix(path, "/decline")) {
		if p.AgentID == "" || p.IsGuest() {
			return errs.Forbidden("仅坐席可操作")
		}
		return setCommandFields(r, map[string]string{"agent_id": p.AgentID})
	}
	if strings.HasSuffix(path, "/video/request") {
		for _, leg := range view.Legs {
			if (p.IsGuest() && leg.Role == dto.LegRoleCustomer) || (p.AgentID != "" && leg.AgentID == p.AgentID) {
				return setCommandFields(r, map[string]string{"from_leg_id": leg.ID})
			}
		}
		return errs.Forbidden("没有可用的媒体腿")
	}
	if strings.HasSuffix(path, "/screen-share") || strings.HasSuffix(path, "/dtmf") {
		body, err := readCommandBody(r)
		if err != nil {
			return err
		}
		legID, _ := body["leg_id"].(string)
		if !ownsLeg(p, legID, view) {
			return errs.Forbidden("不能操作他人的媒体腿")
		}
		return replaceCommandBody(r, body)
	}
	return nil
}

func ownsCall(p auth.Principal, callID string, view ports.CallView) bool {
	if p.IsGuest() {
		return p.GuestCallID != "" && p.GuestCallID == callID
	}
	if p.AgentID != "" && view.AgentID == p.AgentID {
		return true
	}
	for _, leg := range view.Legs {
		if p.AgentID != "" && leg.AgentID == p.AgentID {
			return true
		}
	}
	return false
}

func ownsLeg(p auth.Principal, legID string, view ports.CallView) bool {
	for _, leg := range view.Legs {
		if leg.ID != legID {
			continue
		}
		return (p.IsGuest() && leg.Role == dto.LegRoleCustomer) || (p.AgentID != "" && leg.AgentID == p.AgentID)
	}
	return false
}

func setCommandFields(r *http.Request, fields map[string]string) error {
	body, err := readCommandBody(r)
	if err != nil {
		return err
	}
	for k, v := range fields {
		body[k] = v
	}
	return replaceCommandBody(r, body)
}

func readCommandBody(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	_ = r.Body.Close()
	if err != nil || len(raw) > 64<<10 {
		return nil, errs.InvalidRequest("请求体过大")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil || body == nil {
		return nil, errs.InvalidRequest("JSON 无法解析")
	}
	return body, nil
}

func replaceCommandBody(r *http.Request, body map[string]any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	r.ContentLength = int64(len(raw))
	r.Header.Set("Content-Type", "application/json")
	return nil
}
