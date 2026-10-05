// 本文件负责技能与坐席技能接口。
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (d RouterDeps) handleSkillList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Skills.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleSkillCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Skills.Create(r.Context(), body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// handleSkillUpdate 修改技能名称。
func (d RouterDeps) handleSkillUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Skills.Update(r.Context(), chi.URLParam(r, "skillId"), body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSkillDelete 删除未被引用的技能。
func (d RouterDeps) handleSkillDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Skills.Delete(r.Context(), chi.URLParam(r, "skillId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (d RouterDeps) handleAgentSkills(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SkillIDs []string `json:"skill_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.Skills.BindAgent(r.Context(), chi.URLParam(r, "agentId"), body.SkillIDs); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (d RouterDeps) handleAgentSkillsGet(w http.ResponseWriter, r *http.Request) {
	ids, err := d.Skills.AgentSkills(r.Context(), chi.URLParam(r, "agentId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skill_ids": ids})
}

func (d RouterDeps) handleAgentList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Agents.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	sessions, err := d.AgentRuntime.ListAgentSessions(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	states := make(map[string]string, len(sessions))
	for _, s := range sessions {
		states[s.AgentID] = s.State
	}
	for i := range out {
		out[i].State = "offline"
		if st, ok := states[out[i].ID]; ok {
			out[i].State = st
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
