package http

import (
	"encoding/csv"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"open-call/internal/datetime"
	"open-call/internal/errs"
	"open-call/internal/layers/biz/cdr"
	"open-call/internal/layers/biz/configio"
	"open-call/internal/layers/biz/ivr"
	"open-call/internal/layers/biz/webhook"
)

func (d RouterDeps) handleWrapUp(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.AgentID == "" {
		writeErr(w, errs.Forbidden("仅坐席可提交小结"))
		return
	}
	var body struct {
		Notes           string   `json:"notes"`
		Text            string   `json:"text"`
		DispositionCode string   `json:"disposition_code"`
		Tags            []string `json:"tags"`
		Complete        bool     `json:"complete"`
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
	out, err := d.Recordings.SaveWrapUp(r.Context(), chi.URLParam(r, "callId"), p.AgentID, notes, body.DispositionCode, body.Tags, body.Complete)
	if err != nil {
		writeErr(w, err)
		return
	}
	if body.Complete && d.Agents != nil {
		if sess, err := d.Agents.UpdateState(r.Context(), p.AgentID, "idle", "wrap-up"); err == nil {
			_ = sess
		}
	}
	d.writeAudit(r.Context(), p.UserID, "wrap_up", chi.URLParam(r, "callId"), nil)
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

func (d RouterDeps) handleCDRExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if _, err := datetime.Parse(q.Get("from")); err != nil {
		writeErr(w, errs.InvalidRequest("from 必须为 YYYY-MM-DD HH:MM:SS (UTC) 时间"))
		return
	}
	if _, err := datetime.Parse(q.Get("to")); err != nil {
		writeErr(w, errs.InvalidRequest("to 必须为 YYYY-MM-DD HH:MM:SS (UTC) 时间"))
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=cdr.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"call_id", "direction", "caller", "callee", "queue_id", "agent_id", "started_at", "answered_at", "ended_at", "duration_sec", "wait_sec", "result", "session_type"})
	err := d.CDR.Stream(r.Context(), cdr.Filter{
		From: q.Get("from"), To: q.Get("to"), QueueID: q.Get("queue_id"),
		AgentID: q.Get("agent_id"), Direction: q.Get("direction"), Result: q.Get("result"),
	}, func(it cdr.Item) error {
		ans, end := "", ""
		if it.AnsweredAt != nil {
			ans = datetime.Format(*it.AnsweredAt)
		}
		if it.EndedAt != nil {
			end = datetime.Format(*it.EndedAt)
		}
		return cw.Write([]string{it.CallID, it.Direction, it.Caller, it.Callee, it.QueueID, it.AgentID, datetime.Format(it.StartedAt), ans, end, strconv.Itoa(it.DurationSec), strconv.Itoa(it.WaitSec), it.Result, it.SessionType})
	})
	cw.Flush()
	if err != nil {
		return
	}
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
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "" {
		if (format != "webm" && format != "mp4") || row.MediaType != "video_composite" ||
			(filepath.Ext(row.FilePath) != ".webm" && filepath.Ext(row.FilePath) != ".mp4") {
			writeErr(w, errs.InvalidRequest("录像格式仅支持 webm 或 mp4"))
			return
		}
	}
	d.writeAudit(r.Context(), p.UserID, "recording_download", row.ID, map[string]string{"call_id": row.CallID, "format": format})
	f, err := d.Recordings.Open(r.Context(), row, format)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = f.Close() }()
	ext := filepath.Ext(row.FilePath)
	if format != "" {
		ext = "." + format
	}
	name := row.ID + ext
	if name == row.ID {
		name = row.ID + ".ogg"
	}
	ctype := "application/octet-stream"
	switch strings.ToLower(ext) {
	case ".ogg":
		ctype = "audio/ogg"
	case ".webm":
		ctype = "video/webm"
	case ".mp4":
		ctype = "video/mp4"
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
	if p, ok := principal(r); ok {
		d.writeAudit(r.Context(), p.UserID, "recording_purge", "", map[string]int{"deleted": n})
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

// handleSkillUpdate 修改技能名称。
func (d RouterDeps) handleSkillUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Skills.Update(r.Context(), chi.URLParam(r, "skillId"), body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSkillDelete 删除未被引用的技能。
func (d RouterDeps) handleSkillDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Skills.Delete(r.Context(), chi.URLParam(r, "skillId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
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
	if p, ok := principal(r); ok {
		d.writeAudit(r.Context(), p.UserID, "ivr_create", out.ID, nil)
	}
	writeJSON(w, http.StatusCreated, out)
}

// handleIVRGet 读取单个 IVR 草稿及最新发布版本。
func (d RouterDeps) handleIVRGet(w http.ResponseWriter, r *http.Request) {
	out, err := d.IVR.Get(r.Context(), chi.URLParam(r, "flowId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleIVRUpdate 修改 IVR 名称或草稿。
func (d RouterDeps) handleIVRUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string   `json:"name"`
		Draft *ivr.Doc `json:"draft"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.IVR.Update(r.Context(), chi.URLParam(r, "flowId"), body.Name, body.Draft)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleIVRDelete 删除未被队列使用的 IVR。
func (d RouterDeps) handleIVRDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.IVR.Delete(r.Context(), chi.URLParam(r, "flowId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

// handleIVRVersions 列出不可变发布版本。
func (d RouterDeps) handleIVRVersions(w http.ResponseWriter, r *http.Request) {
	out, err := d.IVR.ListSnapshots(r.Context(), chi.URLParam(r, "flowId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// handleIVRRollback 将历史版本复制为新的已发布版本。
func (d RouterDeps) handleIVRRollback(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version int `json:"version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.IVR.Rollback(r.Context(), chi.URLParam(r, "flowId"), body.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleIVRPublish(w http.ResponseWriter, r *http.Request) {
	out, err := d.IVR.Publish(r.Context(), chi.URLParam(r, "flowId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if p, ok := principal(r); ok {
		d.writeAudit(r.Context(), p.UserID, "ivr_publish", out.FlowID, map[string]int{"version": out.Version})
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

// handleWebhookUpdate 更新、停用或轮换订阅密钥。
func (d RouterDeps) handleWebhookUpdate(w http.ResponseWriter, r *http.Request) {
	var in webhook.UpdateInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	out, secret, err := d.Webhooks.Update(r.Context(), chi.URLParam(r, "subscriptionId"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	response := map[string]any{"subscription": out}
	if secret != "" {
		response["secret"] = secret
	}
	writeJSON(w, http.StatusOK, response)
}

// handleWebhookDelete 删除订阅和所属投递历史。
func (d RouterDeps) handleWebhookDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Webhooks.Delete(r.Context(), chi.URLParam(r, "subscriptionId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

// handleWebhookDeliveryList 分页查询投递任务和死信。
func (d RouterDeps) handleWebhookDeliveryList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	query := r.URL.Query()
	out, err := d.Webhooks.ListDeliveries(r.Context(), page, size, query.Get("status"), query.Get("event_type"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleWebhookRetry(w http.ResponseWriter, r *http.Request) {
	if err := d.Webhooks.Retry(r.Context(), chi.URLParam(r, "deliveryId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, nil)
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
	var bundle configio.Bundle
	if err := decodeJSON(r, &bundle); err != nil {
		writeErr(w, err)
		return
	}
	options := configio.ImportOptions{DryRun: r.URL.Query().Get("dry_run") == "true", Mode: r.URL.Query().Get("mode")}
	report, err := d.ConfigIO.ImportWithOptions(r.Context(), bundle, options)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !options.DryRun {
		if p, ok := principal(r); ok && d.Audit != nil {
			if err := d.Audit.Write(r.Context(), p.UserID, "config_import", "", map[string]string{"outcome": "success"}); err != nil {
				writeErr(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, report)
}

func (d RouterDeps) handleAuditList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	q := r.URL.Query()
	out, err := d.Audit.List(r.Context(), page, size, q.Get("user_id"), q.Get("action"),
		q.Get("resource"), q.Get("outcome"), q.Get("from"), q.Get("to"))
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
		Score     *int   `json:"score"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Recordings.AddQAMark(r.Context(), chi.URLParam(r, "callId"), p.UserID, body.OffsetSec, body.Label, body.Score)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d RouterDeps) handleQAUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OffsetSec int    `json:"offset_sec"`
		Label     string `json:"label"`
		Score     *int   `json:"score"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Recordings.UpdateQAMark(r.Context(), chi.URLParam(r, "markId"), chi.URLParam(r, "callId"), body.OffsetSec, body.Label, body.Score)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleQADelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Recordings.DeleteQAMark(r.Context(), chi.URLParam(r, "markId"), chi.URLParam(r, "callId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
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

// handleDIDDelete 删除 DID 路由。
func (d RouterDeps) handleDIDDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Snapshots.DeleteDID(r.Context(), chi.URLParam(r, "didId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
