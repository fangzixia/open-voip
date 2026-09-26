// 本文件负责运行状态接口处理。
package http

import (
	"net/http"
	"open-call/internal/errs"
	"open-call/internal/ports"
)

func (d RouterDeps) handleMyCalls(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(r)
	if !ok || p.AgentID == "" {
		writeErr(w, errs.Forbidden("仅坐席可读取"))
		return
	}
	if d.Status.Runtime == nil {
		writeErr(w, errs.NotImplemented("交换服务未配置"))
		return
	}
	calls, err := d.Status.Runtime.ListCalls(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := []ports.CallView{}
	for _, call := range calls {
		mine := call.AgentID == p.AgentID
		for _, leg := range call.Legs {
			mine = mine || leg.AgentID == p.AgentID
		}
		if mine {
			out = append(out, call)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
