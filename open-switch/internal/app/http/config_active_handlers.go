package http

import (
	"net/http"
)

func (d SwitchRouterDeps) handleConfigActive(w http.ResponseWriter, r *http.Request) {
	view, err := d.Admin.GetActiveConfiguration(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (d SwitchRouterDeps) handleConfigActiveSummary(w http.ResponseWriter, r *http.Request) {
	view, err := d.Admin.GetActiveConfigurationSummary(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
