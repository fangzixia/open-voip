// Package platform 暴露 /platform/v1 供 open-switch 回调。
package platform

import (
	"encoding/json"
	"net/http"

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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	out, err := d.Queues.RequestAgent(r.Context(), req)
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	if err := d.Agents.SetCallState(r.Context(), body.CallID, chi.URLParam(r, "agentId"), body.FromState, body.ToState, body.Reason); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	if err := d.CDR.Upsert(r.Context(), req); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleRecording(w http.ResponseWriter, r *http.Request) {
	var req ports.RecordingMeta
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	if err := d.Recordings.Save(r.Context(), req); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleCallEvent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type    string         `json:"type"`
		CallID  string         `json:"call_id"`
		AgentID string         `json:"agent_id"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apphttp.WriteErr(w, err)
		return
	}
	if d.Hub != nil {
		_ = d.Hub.PublishCallEvent(r.Context(), ports.CallEvent{
			Type: body.Type, CallID: body.CallID, AgentID: body.AgentID, Payload: body.Payload,
		})
	}
	w.WriteHeader(http.StatusNoContent)
}
