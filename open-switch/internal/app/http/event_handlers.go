package http

import (
	"net/http"
	"open-switch/internal/errs"
	"open-switch/internal/scope"
	"open-switch/internal/store"
	"strconv"
	"time"
)

// handleEvents 按 after_id 增量返回持久化呼叫事件（主路径为 integrator callback；本接口用于对账）。
func (d SwitchRouterDeps) handleEvents(w http.ResponseWriter, r *http.Request) {
	after := int64(0)
	if raw := r.URL.Query().Get("after_id"); raw != "" {
		var err error
		after, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || after < 0 {
			writeErr(w, errs.InvalidRequest("after_id 无效"))
			return
		}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	waitSec, _ := strconv.Atoi(r.URL.Query().Get("wait_sec"))
	if waitSec < 0 {
		waitSec = 0
	}
	if waitSec > 30 {
		waitSec = 30
	}
	appID := scope.Application(r.Context())
	if appID != "" && after > 0 && d.Events != nil {
		minID, err := d.Events.MinRetainedEventID(r.Context(), appID)
		if err != nil {
			writeErr(w, err)
			return
		}
		if minID > 0 && after < minID {
			httpapiFailure(w, http.StatusGone, "cursor_expired", "事件游标早于保留窗口，请对账后重置游标；最早可用 event_id="+strconv.FormatInt(minID, 10))
			return
		}
	}
	if d.Events == nil {
		writeErr(w, errs.Internal("事件存储未配置"))
		return
	}
	deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
	var rows []store.CallEventRow
	for {
		rows, err := d.Events.List(r.Context(), after, r.URL.Query().Get("call_id"), limit)
		if err != nil {
			writeErr(w, err)
			return
		}
		if len(rows) > 0 || waitSec == 0 || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if rows == nil {
		rows = []store.CallEventRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func httpapiFailure(w http.ResponseWriter, status int, kind, message string) {
	writeJSON(w, status, map[string]any{"code": kind, "message": message, "error": kind})
}
