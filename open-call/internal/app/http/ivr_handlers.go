// 本文件负责IVR 流程管理接口。
package http

import (
	"net/http"
	"open-call/internal/layers/biz/ivr"

	"github.com/go-chi/chi/v5"
)

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
