package app

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"open-call/internal/errs"
	"open-call/internal/httpapi"
	"open-call/internal/integration/switchapi"
	"open-call/internal/layers/biz/businessaction"
	"open-call/internal/ports"

	"gorm.io/gorm"
)

// SwitchEventHTTP 返回 open-switch 事件 callback 处理器。
func SwitchEventHTTP(db *gorm.DB, hub ports.CallEventPublisher, actions *businessaction.Service, client *switchapi.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var ev switchapi.Event
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			httpapi.Error(w, err)
			return
		}
		if ev.ID <= 0 {
			httpapi.Error(w, errs.InvalidRequest("event id 无效"))
			return
		}
		if err := CommitSwitchEvent(r.Context(), db, ev, hub); err != nil {
			httpapi.Error(w, err)
			return
		}
		// 投影已提交；业务动作失败须返回错误，便于 Switch 重投同一事件（inbox 幂等，会再次执行 HandleRequested）。
		if actions != nil && ev.Type == "business_action.requested" {
			if err := actions.HandleRequested(r.Context(), ev, client); err != nil {
				slog.ErrorContext(r.Context(), "business_action 处理失败，等待 Switch 重投",
					"event_id", ev.ID, "call_id", ev.CallID, "err", err)
				httpapi.Error(w, err)
				return
			}
		}
		if err := DeliverSwitchEvents(r.Context(), db, hub); err != nil {
			httpapi.Error(w, err)
			return
		}
		httpapi.Write(w, http.StatusOK, map[string]any{"accepted": true, "event_id": ev.ID})
	}
}
