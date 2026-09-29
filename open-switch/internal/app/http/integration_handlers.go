package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"open-switch/internal/errs"
	"open-switch/internal/store"
)

func (d SwitchRouterDeps) handleIntegrationRegister(w http.ResponseWriter, r *http.Request) {
	if d.Applications == nil {
		writeErr(w, errs.Internal("应用登记未配置"))
		return
	}
	var body struct {
		ApplicationID      string `json:"application_id"`
		EventsCallbackURL  string `json:"events_callback_url"`
		EventRetentionDays int    `json:"event_retention_days"`
		MaxConcurrentCalls int    `json:"max_concurrent_calls"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Applications.Register(r.Context(), store.RegisterApplicationInput{
		ApplicationID:      body.ApplicationID,
		EventsCallbackURL:  body.EventsCallbackURL,
		EventRetentionDays: body.EventRetentionDays,
		MaxConcurrentCalls: body.MaxConcurrentCalls,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusOK
	data := map[string]any{"application_id": out.ApplicationID, "created": out.Created}
	if out.Created {
		status = http.StatusCreated
		data["secret"] = out.Secret
	}
	writeJSON(w, status, data)
}

func (d SwitchRouterDeps) handleIntegrationRotateSecret(w http.ResponseWriter, r *http.Request) {
	if d.Applications == nil {
		writeErr(w, errs.Internal("应用登记未配置"))
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "applicationId"))
	secret, err := d.Applications.RotateSecret(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"application_id": id, "secret": secret})
}
