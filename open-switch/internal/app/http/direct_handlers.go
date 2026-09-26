package http

import (
	"net/http"
	"open-switch/internal/ports/dto"

	"github.com/go-chi/chi/v5"
)

func (d SwitchRouterDeps) handleDirectCreate(w http.ResponseWriter, r *http.Request) {
	var req dto.DirectCallRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Direct.CreateDirect(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (d SwitchRouterDeps) handleDirectLeg(w http.ResponseWriter, r *http.Request) {
	var req dto.DirectLegRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Direct.AddDirectLeg(r.Context(), chi.URLParam(r, "callId"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (d SwitchRouterDeps) handleDirectSIP(w http.ResponseWriter, r *http.Request) {
	var req dto.DirectSIPRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Direct.DialDirectSIP(r.Context(), chi.URLParam(r, "callId"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, view)
}

func (d SwitchRouterDeps) handleDirectBridge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LegA string `json:"leg_a"`
		LegB string `json:"leg_b"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.Direct.BridgeDirect(r.Context(), chi.URLParam(r, "callId"), body.LegA, body.LegB); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d SwitchRouterDeps) handleDirectLeave(w http.ResponseWriter, r *http.Request) {
	if err := d.Direct.LeaveDirectLeg(r.Context(), chi.URLParam(r, "callId"), chi.URLParam(r, "legId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d SwitchRouterDeps) handleDirectRecordingStart(w http.ResponseWriter, r *http.Request) {
	var policy dto.RecordingPolicy
	if err := decodeJSON(r, &policy); err != nil {
		writeErr(w, err)
		return
	}
	meta, err := d.Direct.StartDirectRecording(r.Context(), chi.URLParam(r, "callId"), policy)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (d SwitchRouterDeps) handleDirectRecordingStop(w http.ResponseWriter, r *http.Request) {
	meta, err := d.Direct.StopDirectRecording(r.Context(), chi.URLParam(r, "callId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}
