package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"open-switch/internal/errs"
	"open-switch/internal/ports/dto"
)

func (d RouterDeps) handleCallGet(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Signaling.GetCall(r.Context(), chi.URLParam(r, "callId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (d RouterDeps) handleCallAnswer(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.AgentID == "" {
		writeErr(w, errs.Forbidden("仅坐席可接听"))
		return
	}
	if err := d.CallControl.Answer(r.Context(), chi.URLParam(r, "callId"), p.AgentID); err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Signaling.GetCall(r.Context(), chi.URLParam(r, "callId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (d RouterDeps) handleCallHangup(w http.ResponseWriter, r *http.Request) {
	if err := d.authorizeCall(r, chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = decodeJSON(r, &body)
	reason := dto.HangupReason(body.Reason)
	if reason == "" {
		reason = dto.HangupReasonNormal
	}
	if err := d.CallControl.Hangup(r.Context(), chi.URLParam(r, "callId"), reason); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d RouterDeps) handleOffer(w http.ResponseWriter, r *http.Request) {
	callID := chi.URLParam(r, "callId")
	legID := chi.URLParam(r, "legId")
	if err := d.authorizeLeg(r, callID, legID); err != nil {
		writeErr(w, err)
		return
	}
	offer, err := d.Signaling.JoinWebRTC(r.Context(), callID, legID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sdp": offer.SDP, "type": offer.Type})
}

func (d RouterDeps) handleAnswerSDP(w http.ResponseWriter, r *http.Request) {
	callID := chi.URLParam(r, "callId")
	legID := chi.URLParam(r, "legId")
	if err := d.authorizeLeg(r, callID, legID); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		SDP  string `json:"sdp"`
		Type string `json:"type"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.Signaling.AcceptAnswer(r.Context(), callID, legID, body.SDP); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (d RouterDeps) handleICE(w http.ResponseWriter, r *http.Request) {
	callID := chi.URLParam(r, "callId")
	legID := chi.URLParam(r, "legId")
	if err := d.authorizeLeg(r, callID, legID); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Candidate     string `json:"candidate"`
		SDPMid        string `json:"sdp_mid"`
		SDPMLineIndex *int   `json:"sdp_mline_index"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	var idx *uint16
	if body.SDPMLineIndex != nil {
		idx = new(uint16(*body.SDPMLineIndex))
	}
	if err := d.Signaling.TrickleICE(r.Context(), callID, legID, dto.ICECandidateInit{
		Candidate:     body.Candidate,
		SDPMid:        body.SDPMid,
		SDPMLineIndex: idx,
	}); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d RouterDeps) handleMute(w http.ResponseWriter, r *http.Request) {
	callID := chi.URLParam(r, "callId")
	legID := chi.URLParam(r, "legId")
	if err := d.authorizeLeg(r, callID, legID); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Audio bool `json:"audio"`
		Video bool `json:"video"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.Signaling.SetTrackMuted(r.Context(), callID, legID, body.Audio, body.Video); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) authorizeCall(r *http.Request, callID string) error {
	p, ok := principal(r)
	if !ok {
		return errs.Unauthorized("未认证或令牌失效")
	}
	if p.Role == "admin" || p.Role == "supervisor" {
		return nil
	}
	view, err := d.Signaling.GetCall(r.Context(), callID)
	if err != nil {
		return err
	}
	if p.IsGuest() {
		if p.GuestCallID == "" || p.GuestCallID != callID {
			return errs.Forbidden("不能操作他人通话")
		}
		return nil
	}
	if p.AgentID != "" && view.AgentID == p.AgentID {
		return nil
	}
	if p.AgentID != "" {
		for _, leg := range view.Legs {
			if leg.AgentID == p.AgentID {
				return nil
			}
		}
	}
	return errs.Forbidden("不能操作该通话")
}

// authorizeLeg 防止同一通话的参与者覆盖他人的 SDP、ICE 或静音状态。
func (d RouterDeps) authorizeLeg(r *http.Request, callID, legID string) error {
	if err := d.authorizeCall(r, callID); err != nil {
		return err
	}
	p, _ := principal(r)
	view, err := d.Signaling.GetCall(r.Context(), callID)
	if err != nil {
		return err
	}
	for _, leg := range view.Legs {
		if leg.ID != legID {
			continue
		}
		if p.IsGuest() && leg.Role == dto.LegRoleCustomer {
			return nil
		}
		if p.AgentID != "" && leg.AgentID == p.AgentID {
			return nil
		}
		return errs.Forbidden("不能操作他人的媒体腿")
	}
	return errs.NotFound("媒体腿不存在")
}
