package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"open-call/internal/errs"
	"open-call/internal/integration/switchapi"
)

// handleCallSurvey 选择业务评价流程，调用 Switch 通用 IVR 接续能力。
func (d RouterDeps) handleCallSurvey(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.IsGuest() || p.AgentID == "" {
		writeErr(w, errs.Forbidden("仅坐席可转满意度"))
		return
	}
	if d.Switch == nil {
		writeErr(w, errs.NotImplemented("交换服务未配置"))
		return
	}
	callID := chi.URLParam(r, "callId")
	view, err := d.Switch.GetCall(r.Context(), callID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ownsCall(p, callID, view) {
		writeErr(w, errs.Forbidden("无权操作该通话"))
		return
	}
	var body struct {
		FlowID          string `json:"flow_id"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.FlowID == "" {
		body.FlowID = view.PostCallIVRFlowID
	}
	if body.FlowID == "" {
		writeErr(w, errs.InvalidRequest("未配置满意度 IVR 流程"))
		return
	}
	if body.ExpectedVersion == 0 {
		body.ExpectedVersion = view.Version
	}
	ctx := switchapi.WithMutation(r.Context(), switchapi.Mutation{
		ExpectedVersion: body.ExpectedVersion,
		IdempotencyKey:  strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	out, err := d.Switch.EnterIVR(ctx, callID, body.FlowID)
	if err != nil {
		writeErr(w, err)
		return
	}
	d.writeAudit(r.Context(), p.UserID, "call.survey", callID, nil)
	writeJSON(w, http.StatusOK, out)
}
