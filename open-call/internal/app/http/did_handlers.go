// 本文件负责DID 路由接口。
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

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
		TrunkID    string `json:"trunk_id"`
		DID        string `json:"did"`
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Snapshots.UpsertDID(r.Context(), body.TrunkID, body.DID, body.TargetType, body.TargetID)
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
