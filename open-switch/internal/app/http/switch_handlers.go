// 本文件负责交换服务管理接口处理。
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"open-switch/internal/errs"
	"open-switch/internal/ports/dto"
)

func (d RouterDeps) handleOutbound(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentID     string `json:"agent_id"`
		Destination string `json:"destination"`
		TrunkID     string `json:"trunk_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.AgentID == "" {
		writeErr(w, errs.InvalidRequest("agent_id 必填"))
		return
	}
	id, err := d.CallControl.Outbound(r.Context(), dto.OutboundRequest{AgentID: body.AgentID, Destination: body.Destination, TrunkID: body.TrunkID})
	if err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Signaling.GetCall(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (d RouterDeps) handleHold(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		On bool `json:"on"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.CallControl.Hold(r.Context(), chi.URLParam(r, "callId"), body.On); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleTransfer(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	var req dto.TransferRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.CallControl.Transfer(r.Context(), chi.URLParam(r, "callId"), req); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleCompleteTransfer(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.CallControl.CompleteTransfer(r.Context(), chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleVideoRequest(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		FromLegID string `json:"from_leg_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.FromLegID == "" {
		writeErr(w, errs.InvalidRequest("from_leg_id 必填"))
		return
	}
	if err := d.authorizeLeg(r, chi.URLParam(r, "callId"), body.FromLegID); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.CallControl.RequestVideo(r.Context(), chi.URLParam(r, "callId"), body.FromLegID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, nil)
}

func (d RouterDeps) handleVideoRespond(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Accept bool `json:"accept"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.CallControl.RespondVideo(r.Context(), chi.URLParam(r, "callId"), body.Accept); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleVideoDowngrade(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.CallControl.DowngradeVideo(r.Context(), chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleScreenShare(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		On    bool   `json:"on"`
		LegID string `json:"leg_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.authorizeLeg(r, chi.URLParam(r, "callId"), body.LegID); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.CallControl.ScreenShare(r.Context(), chi.URLParam(r, "callId"), body.LegID, body.On); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleConference(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		AgentID string `json:"agent_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.CallControl.ConferenceInvite(r.Context(), chi.URLParam(r, "callId"), body.AgentID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleDTMF(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		LegID string `json:"leg_id"`
		Digit string `json:"digit"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.authorizeLeg(r, chi.URLParam(r, "callId"), body.LegID); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.CallControl.SendDTMF(r.Context(), chi.URLParam(r, "callId"), body.LegID, body.Digit); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleTURN(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	sub := r.URL.Query().Get("subject")
	if sub == "" {
		writeErr(w, errs.InvalidRequest("subject 必填"))
		return
	}
	cfg, err := d.Signaling.IssueTURNCredentials(r.Context(), chi.URLParam(r, "callId"), sub)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"urls": cfg.URLs, "username": cfg.Username, "credential": cfg.Credential, "ttl_sec": int(cfg.TTL.Seconds()),
	})
}

func (d RouterDeps) handleForceCheckout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Policy string `json:"policy"`
	}
	_ = decodeJSON(r, &body)
	if err := d.CallControl.ForceReleaseAgent(r.Context(), chi.URLParam(r, "agentId"), body.Policy); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleListen(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentID string `json:"agent_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.AgentID == "" {
		writeErr(w, errs.InvalidRequest("agent_id 必填"))
		return
	}
	legID, err := d.CallControl.SupervisorListen(r.Context(), chi.URLParam(r, "callId"), body.AgentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"leg_id": legID, "call_id": chi.URLParam(r, "callId")})
}

func (d RouterDeps) handleDecline(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentID string `json:"agent_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.AgentID == "" {
		writeErr(w, errs.InvalidRequest("agent_id 必填"))
		return
	}
	if err := d.CallControl.Decline(r.Context(), chi.URLParam(r, "callId"), body.AgentID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
