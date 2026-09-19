package http

import (
	"encoding/csv"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"open-voip/internal/errs"
	"open-voip/internal/layers/biz/configio"
	"open-voip/internal/layers/biz/ivr"
	"open-voip/internal/ports/dto"
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
	w.WriteHeader(http.StatusAccepted)
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

func (d RouterDeps) handleWrapUp(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.AgentID == "" {
		writeErr(w, errs.Forbidden("仅坐席可提交小结"))
		return
	}
	var body struct {
		Notes string `json:"notes"`
		Text  string `json:"text"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	notes := body.Notes
	if notes == "" {
		notes = body.Text
	}
	if d.Recordings == nil {
		writeErr(w, errs.NotImplemented("小结"))
		return
	}
	out, err := d.Recordings.SaveWrapUp(r.Context(), chi.URLParam(r, "callId"), p.AgentID, notes)
	if err != nil {
		writeErr(w, err)
		return
	}
	if d.Agents != nil {
		if sess, err := d.Agents.UpdateState(r.Context(), p.AgentID, "idle", "wrap-up"); err == nil {
			_ = sess
		}
	}
	if d.Audit != nil {
		d.Audit.Write(r.Context(), p.UserID, "wrap_up", chi.URLParam(r, "callId"), nil)
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleWrapUpList(w http.ResponseWriter, r *http.Request) {
	if d.Recordings == nil {
		writeErr(w, errs.NotImplemented("小结"))
		return
	}
	out, err := d.Recordings.ListWrapUps(r.Context(), r.URL.Query().Get("call_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (d RouterDeps) handleForceCheckout(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
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
	_ = d.Agents.CheckOut(r.Context(), chi.URLParam(r, "agentId"))
	if d.Audit != nil {
		d.Audit.Write(r.Context(), p.UserID, "force_check_out", chi.URLParam(r, "agentId"), body)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleListen(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.AgentID == "" {
		writeErr(w, errs.Forbidden("班长需要坐席资料"))
		return
	}
	legID, err := d.CallControl.SupervisorListen(r.Context(), chi.URLParam(r, "callId"), p.AgentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if d.Audit != nil {
		d.Audit.Write(r.Context(), p.UserID, "listen", chi.URLParam(r, "callId"), nil)
	}
	writeJSON(w, http.StatusOK, map[string]string{"leg_id": legID, "call_id": chi.URLParam(r, "callId")})
}

func (d RouterDeps) handleCDRExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := d.CDR.ExportCSV(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=cdr.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"call_id", "direction", "caller", "callee", "queue_id", "agent_id", "started_at", "answered_at", "ended_at", "duration_sec", "wait_sec", "result", "session_type"})
	for _, it := range items {
		ans, end := "", ""
		if it.AnsweredAt != nil {
			ans = it.AnsweredAt.Format("2006-01-02T15:04:05Z")
		}
		if it.EndedAt != nil {
			end = it.EndedAt.Format("2006-01-02T15:04:05Z")
		}
		_ = cw.Write([]string{it.CallID, it.Direction, it.Caller, it.Callee, it.QueueID, it.AgentID, it.StartedAt.Format("2006-01-02T15:04:05Z"), ans, end, strconv.Itoa(it.DurationSec), strconv.Itoa(it.WaitSec), it.Result, it.SessionType})
	}
	cw.Flush()
}

func (d RouterDeps) handleRecordingList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	out, err := d.Recordings.List(r.Context(), page, size, r.URL.Query().Get("call_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleRecordingDownload(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	row, err := d.Recordings.Get(r.Context(), chi.URLParam(r, "recordingId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if d.Audit != nil {
		d.Audit.Write(r.Context(), p.UserID, "recording_download", row.ID, map[string]string{"call_id": row.CallID})
	}
	f, err := os.Open(row.FilePath)
	if err != nil {
		writeErr(w, errs.NotFound("录音文件不存在"))
		return
	}
	defer func() { _ = f.Close() }()
	name := row.ID + filepath.Ext(row.FilePath)
	if name == row.ID {
		name = row.ID + ".ogg"
	}
	ctype := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(row.FilePath)) {
	case ".ogg":
		ctype = "audio/ogg"
	case ".webm":
		ctype = "video/webm"
	case ".ivf":
		ctype = "video/x-ivf"
	case ".wav":
		ctype = "audio/wav"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	_, _ = io.Copy(w, f)
}

func (d RouterDeps) handlePurgeRecordings(w http.ResponseWriter, r *http.Request) {
	n, err := d.Recordings.PurgeExpired(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if p, ok := principal(r); ok && d.Audit != nil {
		d.Audit.Write(r.Context(), p.UserID, "recording_purge", "", map[string]int{"deleted": n})
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted_count": n})
}

func (d RouterDeps) handleLiveReport(w http.ResponseWriter, r *http.Request) {
	out, err := d.Reports.Live(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleHistoricalReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := d.Reports.Historical(r.Context(), q.Get("from"), q.Get("to"), q.Get("queue_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleAgentUtil(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := d.Reports.AgentUtilization(r.Context(), q.Get("from"), q.Get("to"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (d RouterDeps) handleSkillList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Skills.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleSkillCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Skills.Create(r.Context(), body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleAgentSkills(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SkillIDs []string `json:"skill_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.Skills.BindAgent(r.Context(), chi.URLParam(r, "agentId"), body.SkillIDs); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleAgentSkillsGet(w http.ResponseWriter, r *http.Request) {
	ids, err := d.Skills.AgentSkills(r.Context(), chi.URLParam(r, "agentId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skill_ids": ids})
}

func (d RouterDeps) handleAgentList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Agents.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (d RouterDeps) handleIVRList(w http.ResponseWriter, r *http.Request) {
	out, err := d.IVR.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleIVRCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string  `json:"name"`
		Draft ivr.Doc `json:"draft"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.IVR.Create(r.Context(), body.Name, body.Draft)
	if err != nil {
		writeErr(w, err)
		return
	}
	if p, ok := principal(r); ok && d.Audit != nil {
		d.Audit.Write(r.Context(), p.UserID, "ivr_create", out.ID, nil)
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleIVRPublish(w http.ResponseWriter, r *http.Request) {
	out, err := d.IVR.Publish(r.Context(), chi.URLParam(r, "flowId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if p, ok := principal(r); ok && d.Audit != nil {
		d.Audit.Write(r.Context(), p.UserID, "ivr_publish", out.FlowID, map[string]int{"version": out.Version})
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleWebhookList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Webhooks.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleWebhookCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL        string   `json:"url"`
		EventTypes []string `json:"event_types"`
		Secret     string   `json:"secret"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Webhooks.Create(r.Context(), body.URL, body.EventTypes, body.Secret)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleWebhookRetry(w http.ResponseWriter, r *http.Request) {
	if err := d.Webhooks.Retry(r.Context(), chi.URLParam(r, "deliveryId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (d RouterDeps) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	out, err := d.ConfigIO.Export(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	var out configio.Bundle
	if err := decodeJSON(r, &out); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.ConfigIO.Import(r.Context(), out); err != nil {
		writeErr(w, err)
		return
	}
	if p, ok := principal(r); ok && d.Audit != nil {
		d.Audit.Write(r.Context(), p.UserID, "config_import", "", nil)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleAuditList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	q := r.URL.Query()
	out, err := d.Audit.List(r.Context(), page, size, q.Get("user_id"), q.Get("action"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleQAList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Recordings.ListQAMarks(r.Context(), chi.URLParam(r, "callId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (d RouterDeps) handleQACreate(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok {
		writeErr(w, errs.Unauthorized("未认证或令牌失效"))
		return
	}
	var body struct {
		OffsetSec int    `json:"offset_sec"`
		Label     string `json:"label"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Recordings.AddQAMark(r.Context(), chi.URLParam(r, "callId"), p.UserID, body.OffsetSec, body.Label)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleDIDList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Snapshots.ListDID(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleDIDUpsert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DID         string `json:"did"`
		QueueID     string `json:"queue_id"`
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Snapshots.UpsertDID(r.Context(), body.DID, body.QueueID, body.DisplayName)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
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
	w.WriteHeader(http.StatusNoContent)
}
