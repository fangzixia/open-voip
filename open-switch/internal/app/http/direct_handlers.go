package http

import (
	"net/http"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"

	"github.com/go-chi/chi/v5"
)

func (d SwitchRouterDeps) handleStubCall(w http.ResponseWriter, r *http.Request) {
	var req dto.StubCallRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.Direct.CreateStubCall(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

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
	ctx := scope.WithIdempotency(r.Context(), r.Header.Get("Idempotency-Key"))
	result, err := d.Direct.DialDirectSIP(ctx, chi.URLParam(r, "callId"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"command_id": result.CommandID,
		"leg_id":     result.LegID,
		"call":       result.View,
	})
}

func (d SwitchRouterDeps) handleDirectBridgeAliases(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LegIDs          []string `json:"leg_ids"`
		LegA            string   `json:"leg_a"`
		LegB            string   `json:"leg_b"`
		ExpectedVersion int64    `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	a, b := body.LegA, body.LegB
	if len(body.LegIDs) >= 2 {
		a, b = body.LegIDs[0], body.LegIDs[1]
	}
	ctx := callMutationContext(r, body.ExpectedVersion)
	if err := d.Direct.BridgeDirect(ctx, chi.URLParam(r, "callId"), a, b); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d SwitchRouterDeps) handleDirectBridge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LegA            string `json:"leg_a"`
		LegB            string `json:"leg_b"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	ctx := callMutationContext(r, body.ExpectedVersion)
	if err := d.Direct.BridgeDirect(ctx, chi.URLParam(r, "callId"), body.LegA, body.LegB); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d SwitchRouterDeps) handleDirectLeave(w http.ResponseWriter, r *http.Request) {
	ctx := callMutationContext(r, 0)
	if err := d.Direct.LeaveDirectLeg(ctx, chi.URLParam(r, "callId"), chi.URLParam(r, "legId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d SwitchRouterDeps) handleLegHold(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On              bool  `json:"on"`
		ExpectedVersion int64 `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	ctx := callMutationContext(r, body.ExpectedVersion)
	if err := d.Direct.HoldLeg(ctx, chi.URLParam(r, "callId"), chi.URLParam(r, "legId"), body.On); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d SwitchRouterDeps) handleLegReject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason          string `json:"reason"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	_ = decodeJSON(r, &body)
	ctx := callMutationContext(r, body.ExpectedVersion)
	if err := d.Direct.RejectLeg(ctx, chi.URLParam(r, "callId"), chi.URLParam(r, "legId"), body.Reason); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d SwitchRouterDeps) handleLegPlaybackStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AssetID         string `json:"asset_id"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	ctx := callMutationContext(r, body.ExpectedVersion)
	id, err := d.Direct.StartLegPlayback(ctx, chi.URLParam(r, "callId"), chi.URLParam(r, "legId"), body.AssetID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"playback_id": id})
}

func (d SwitchRouterDeps) handleLegPlaybackStop(w http.ResponseWriter, r *http.Request) {
	ctx := callMutationContext(r, 0)
	if err := d.Direct.StopLegPlayback(ctx, chi.URLParam(r, "callId"), chi.URLParam(r, "legId"), chi.URLParam(r, "playbackId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d SwitchRouterDeps) handlePutBridge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LegIDs          []string `json:"leg_ids"`
		LegA            string   `json:"leg_a"`
		LegB            string   `json:"leg_b"`
		ExpectedVersion int64    `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	a, b := body.LegA, body.LegB
	if len(body.LegIDs) >= 2 {
		a, b = body.LegIDs[0], body.LegIDs[1]
	}
	ctx := callMutationContext(r, body.ExpectedVersion)
	if err := d.Direct.ReplaceBridge(ctx, chi.URLParam(r, "callId"), chi.URLParam(r, "bridgeId"), a, b); err != nil {
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

func (d SwitchRouterDeps) handleDeleteBridge(w http.ResponseWriter, r *http.Request) {
	callID := chi.URLParam(r, "callId")
	bridgeID := chi.URLParam(r, "bridgeId")
	if err := d.Direct.EndBridge(r.Context(), callID, bridgeID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"bridge_id": bridgeID, "call_id": callID})
}
