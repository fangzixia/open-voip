// 本文件负责通话记录导出接口。
package http

import (
	"encoding/csv"
	"net/http"
	"open-call/internal/datetime"
	"open-call/internal/errs"
	"open-call/internal/layers/biz/cdr"
	"strconv"
)

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
