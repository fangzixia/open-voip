// 本文件负责交换服务管理接口处理。
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"open-switch/internal/errs"
	"open-switch/internal/ports/dto"
)

func (d RouterDeps) handleOutbound(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.AgentID == "" {
		writeErr(w, errs.Forbidden("仅坐席可外呼"))
		return
	}
	var body struct {
		Destination string `json:"destination"`
		TrunkID     string `json:"trunk_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	id, err := d.CallControl.Outbound(r.Context(), dto.OutboundRequest{AgentID: p.AgentID, Destination: body.Destination, TrunkID: body.TrunkID})
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
	from := ""
	if p, ok := principal(r); ok {
		view, _ := d.Signaling.GetCall(r.Context(), chi.URLParam(r, "callId"))
		for _, l := range view.Legs {
			if l.AgentID == p.AgentID || (p.IsGuest() && l.Role == "customer") {
				from = l.ID
			}
		}
	}
	if err := d.CallControl.RequestVideo(r.Context(), chi.URLParam(r, "callId"), from); err != nil {
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
	p, _ := principal(r)
	sub := p.UserID
	if sub == "" {
		sub = p.GuestID
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
	p, ok := principal(r)
	if !ok || !p.Has("agents.force_checkout") {
		writeErr(w, errs.Forbidden("仅管理员或班长可强制签出"))
		return
	}
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
	p, ok := principal(r)
	if !ok || p.AgentID == "" || !p.Has("calls.listen") {
		writeErr(w, errs.Forbidden("班长需要坐席资料"))
		return
	}
	legID, err := d.CallControl.SupervisorListen(r.Context(), chi.URLParam(r, "callId"), p.AgentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"leg_id": legID, "call_id": chi.URLParam(r, "callId")})
}

func (d RouterDeps) handleDecline(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.AgentID == "" {
		writeErr(w, errs.Forbidden("仅坐席可拒接"))
		return
	}
	if err := d.CallControl.Decline(r.Context(), chi.URLParam(r, "callId"), p.AgentID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
