package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"open-call/internal/errs"
	"open-call/internal/observability"
)

const (
	maxClientEventBody  = 256 << 10
	maxClientEventBatch = 100
)

type clientTraceEvent struct {
	Type            string         `json:"type"`
	Timestamp       string         `json:"timestamp,omitempty"`
	CallID          string         `json:"call_id,omitempty"`
	LegID           string         `json:"leg_id,omitempty"`
	QueueID         string         `json:"queue_id,omitempty"`
	ClientSessionID string         `json:"client_session_id,omitempty"`
	Fields          map[string]any `json:"fields,omitempty"`
}

func (d RouterDeps) handleClientEvents(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxClientEventBody)
	var body struct {
		Events []clientTraceEvent `json:"events"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeErr(w, errs.InvalidRequest("客户端事件 JSON 无法解析或超过 256 KiB"))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeErr(w, errs.InvalidRequest("请求只能包含一个 JSON 对象"))
		return
	}
	if len(body.Events) == 0 || len(body.Events) > maxClientEventBatch {
		writeErr(w, errs.InvalidRequest("events 数量必须在 1-100 之间"))
		return
	}

	views := map[string]bool{}
	for i := range body.Events {
		event := &body.Events[i]
		event.Type = strings.TrimSpace(event.Type)
		if event.Type == "" || len(event.Type) > 96 {
			writeErr(w, errs.InvalidRequest("事件 type 必填且不能超过 96 字符"))
			return
		}
		event.CallID = observability.NormalizeID(event.CallID)
		event.LegID = observability.NormalizeID(event.LegID)
		event.QueueID = observability.NormalizeID(event.QueueID)
		event.ClientSessionID = observability.NormalizeID(event.ClientSessionID)
		if event.CallID == "" {
			event.CallID = p.GuestCallID
		}
		if p.IsGuest() {
			if event.CallID == "" {
				writeErr(w, errs.InvalidRequest("访客事件 call_id 必填"))
				return
			}
			if event.CallID != p.GuestCallID {
				writeErr(w, errs.Forbidden("访客无权上报该通话事件"))
				return
			}
			continue
		}
		// 坐席登录、WebSocket 建连等通话前事件允许没有 call_id。
		if event.CallID == "" {
			continue
		}
		if p.Has("status.read") {
			continue
		}
		if p.AgentID == "" || d.Calls == nil {
			writeErr(w, errs.Forbidden("无法校验通话归属"))
			return
		}
		if allowed, exists := views[event.CallID]; exists {
			if !allowed {
				writeErr(w, errs.Forbidden("坐席无权上报该通话事件"))
				return
			}
			continue
		}
		view, err := d.Calls.GetCall(r.Context(), event.CallID)
		allowed := err == nil && view.AgentID == p.AgentID
		if !allowed && err == nil {
			for _, leg := range view.Legs {
				if leg.AgentID == p.AgentID {
					allowed = true
					break
				}
			}
		}
		views[event.CallID] = allowed
		if !allowed {
			writeErr(w, errs.Forbidden("坐席无权上报该通话事件"))
			return
		}
	}

	for _, event := range body.Events {
		ctx := observability.With(r.Context(), observability.Context{
			CallID: event.CallID, LegID: event.LegID, AgentID: p.AgentID,
			QueueID: event.QueueID, ClientSessionID: event.ClientSessionID,
		})
		fields := event.Fields
		if fields == nil {
			fields = map[string]any{}
		}
		fields["client_timestamp"] = event.Timestamp
		fields["principal_role"] = p.Role
		observability.Emit(ctx, "client."+event.Type, fields)
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": len(body.Events)})
}
