package app

import (
	"encoding/json"
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
		if actions != nil && ev.Type == "business_action.requested" {
			_ = actions.HandleRequested(r.Context(), ev, client)
		}
		if err := DeliverSwitchEvents(r.Context(), db, hub); err != nil {
			httpapi.Error(w, err)
			return
		}
		httpapi.Write(w, http.StatusOK, map[string]any{"accepted": true, "event_id": ev.ID})
	}
}
