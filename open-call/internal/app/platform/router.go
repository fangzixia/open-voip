// Package platform 暴露 /platform/v1 供 open-switch 回调。
package platform

import (
	"net/http"
	"open-call/internal/datetime"
	"open-call/internal/errs"
	"open-call/internal/httpapi"
	"open-call/internal/observability"

	"github.com/go-chi/chi/v5"

	apphttp "open-call/internal/app/http"
	"open-call/internal/app/http/middleware"
	"open-call/internal/app/ws"
	"open-call/internal/layers/biz/agent"
	"open-call/internal/layers/biz/cdr"
	"open-call/internal/layers/biz/configpub"
	"open-call/internal/layers/biz/queue"
	"open-call/internal/layers/biz/recmeta"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
)

// Deps Platform API 依赖。
type Deps struct {
	Secret     string
	Queues     *queue.Service
	Agents     *agent.Service
	Snapshots  *configpub.SnapshotService
	CDR        *cdr.RecorderService
	Recordings *recmeta.Service
	Hub        *ws.Hub
}

// NewRouter 构建 /platform/v1 路由。
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(httpapi.RequestID)
	r.Use(httpapi.Recover)
	r.NotFound(httpapi.NotFound)
	r.MethodNotAllowed(httpapi.MethodNotAllowed)
	r.Use(middleware.IntegrationAuth(d.Secret))

	r.Post("/acd/dispatch", d.handleDispatch)
	r.Get("/agents/{agentId}", d.handleAgentGet)
	r.Get("/agents/by-extension/{extension}", d.handleAgentByExt)
	r.Post("/agents/{agentId}/state", d.handleAgentState)
	r.Get("/queues/{queueId}/snapshot", d.handleQueueSnapshot)
	r.Get("/ivr/flows/{flowId}/snapshot/latest", d.handleIVRLatest)
	r.Get("/queues/{queueId}/business-hours", d.handleBusinessHours)
	r.Get("/queues/{queueId}/recording-policy", d.handleRecordingPolicy)
	r.Get("/dids/resolve", d.handleResolveDID)
	r.Post("/cdr", d.handleCDR)
	r.Post("/recordings", d.handleRecording)
	r.Post("/events/call", d.handleCallEvent)

	return r
}

func (d Deps) handleDispatch(w http.ResponseWriter, r *http.Request) {
	var req dto.DispatchRequest
	if err := datetime.Decode(r.Body, &req); err != nil {
		apphttp.WriteErr(w, errs.InvalidRequest("JSON 无法解析"))
		return
	}
	ctx := observability.With(r.Context(), observability.Context{CallID: req.CallID, QueueID: req.QueueID})
	observability.Emit(ctx, "platform.acd_dispatch.received", nil)
	out, err := d.Queues.RequestAgent(ctx, req)
	if err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, out)
}

func (d Deps) handleAgentGet(w http.ResponseWriter, r *http.Request) {
	info, err := d.Agents.ByID(r.Context(), chi.URLParam(r, "agentId"))
	if err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, info)
}

func (d Deps) handleAgentByExt(w http.ResponseWriter, r *http.Request) {
	info, err := d.Agents.ByExtension(r.Context(), chi.URLParam(r, "extension"))
	if err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, info)
}

func (d Deps) handleAgentState(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CallID    string `json:"call_id"`
		FromState string `json:"from_state"`
		ToState   string `json:"to_state"`
		Reason    string `json:"reason"`
	}
	if err := datetime.Decode(r.Body, &body); err != nil {
		apphttp.WriteErr(w, errs.InvalidRequest("JSON 无法解析"))
		return
	}
	agentID := chi.URLParam(r, "agentId")
	ctx := observability.With(r.Context(), observability.Context{CallID: body.CallID, AgentID: agentID})
	observability.Emit(ctx, "platform.agent_state.received", map[string]any{"from": body.FromState, "to": body.ToState, "reason": body.Reason})
	if err := d.Agents.SetCallState(ctx, body.CallID, agentID, body.FromState, body.ToState, body.Reason); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, nil)
}

func (d Deps) handleQueueSnapshot(w http.ResponseWriter, r *http.Request) {
	out, err := d.Snapshots.GetQueue(r.Context(), chi.URLParam(r, "queueId"))
	if err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, out)
}

func (d Deps) handleIVRLatest(w http.ResponseWriter, r *http.Request) {
	out, err := d.Snapshots.GetLatestIVR(r.Context(), chi.URLParam(r, "flowId"))
	if err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, out)
}

func (d Deps) handleBusinessHours(w http.ResponseWriter, r *http.Request) {
	out, err := d.Snapshots.GetBusinessHours(r.Context(), chi.URLParam(r, "queueId"))
	if err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, out)
}

func (d Deps) handleRecordingPolicy(w http.ResponseWriter, r *http.Request) {
	out, err := d.Queues.ForQueue(r.Context(), chi.URLParam(r, "queueId"))
	if err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, out)
}

func (d Deps) handleResolveDID(w http.ResponseWriter, r *http.Request) {
	qid, err := d.Snapshots.ResolveDID(r.Context(), r.URL.Query().Get("did"))
	if err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, map[string]string{"queue_id": qid})
}

func (d Deps) handleCDR(w http.ResponseWriter, r *http.Request) {
	var req ports.CDRWriteRequest
	if err := datetime.Decode(r.Body, &req); err != nil {
		apphttp.WriteErr(w, errs.InvalidRequest("JSON 无法解析"))
		return
	}
	ctx := observability.With(r.Context(), observability.Context{CallID: req.CallID, AgentID: req.AgentID, QueueID: req.QueueID})
	observability.Emit(ctx, "platform.cdr.received", map[string]any{"result": req.Result, "direction": req.Direction})
	if err := d.CDR.Upsert(ctx, req); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, nil)
}

func (d Deps) handleRecording(w http.ResponseWriter, r *http.Request) {
	var req ports.RecordingMeta
	if err := datetime.Decode(r.Body, &req); err != nil {
		apphttp.WriteErr(w, errs.InvalidRequest("JSON 无法解析"))
		return
	}
	if err := d.Recordings.Save(r.Context(), req); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	apphttp.WriteJSON(w, http.StatusOK, nil)
}

func (d Deps) handleCallEvent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type    string         `json:"type"`
		CallID  string         `json:"call_id"`
		AgentID string         `json:"agent_id"`
		Payload map[string]any `json:"payload"`
	}
	if err := datetime.Decode(r.Body, &body); err != nil {
		apphttp.WriteErr(w, errs.InvalidRequest("JSON 无法解析"))
		return
	}
	ctx := observability.With(r.Context(), observability.Context{CallID: body.CallID, AgentID: body.AgentID})
	observability.Emit(ctx, "platform.call_event.received", map[string]any{"type": body.Type, "payload": body.Payload})
	if d.Hub != nil {
		_ = d.Hub.PublishCallEvent(ctx, ports.CallEvent{
			Type: body.Type, CallID: body.CallID, AgentID: body.AgentID, Payload: body.Payload,
		})
	}
	apphttp.WriteJSON(w, http.StatusOK, nil)
}
