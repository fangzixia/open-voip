package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
)

func (d SwitchRouterDeps) handleQueueConfigList(w http.ResponseWriter, r *http.Request) {
	items, err := d.Admin.ListQueueConfigs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (d SwitchRouterDeps) handleQueueConfigCreate(w http.ResponseWriter, r *http.Request) {
	var body ports.QueueConfig
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Admin.CreateQueueConfig(r.Context(), body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d SwitchRouterDeps) handleQueueConfigGet(w http.ResponseWriter, r *http.Request) {
	out, err := d.Admin.GetQueueConfig(r.Context(), chi.URLParam(r, "queueId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d SwitchRouterDeps) handleQueueConfigPatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "queueId")
	current, err := d.Admin.GetQueueConfig(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	var patch ports.QueueConfig
	if err := decodeJSON(r, &patch); err != nil {
		writeErr(w, err)
		return
	}
	merged := mergeQueueConfig(current, patch)
	out, err := d.Admin.UpdateQueueConfig(r.Context(), id, merged)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func mergeQueueConfig(base, patch ports.QueueConfig) ports.QueueConfig {
	if patch.Name != "" {
		base.Name = patch.Name
	}
	if patch.MaxWaitSec > 0 {
		base.MaxWaitSec = patch.MaxWaitSec
	}
	if patch.DispatchStrategy != "" {
		base.DispatchStrategy = patch.DispatchStrategy
	}
	if patch.RecordingPolicy != "" {
		base.RecordingPolicy = patch.RecordingPolicy
	}
	if patch.OverflowAction != "" {
		base.OverflowAction = patch.OverflowAction
	}
	if patch.AfterHoursAction != "" {
		base.AfterHoursAction = patch.AfterHoursAction
	}
	if patch.BusinessHoursJSON != "" {
		base.BusinessHoursJSON = patch.BusinessHoursJSON
	}
	base.VideoEnabled = patch.VideoEnabled
	base.AnnounceRecording = patch.AnnounceRecording
	base.PriorityEnabled = patch.PriorityEnabled
	base.ForceHangupOnCheckout = patch.ForceHangupOnCheckout
	base.ListenAnnounce = patch.ListenAnnounce
	base.WaitPrompt = patch.WaitPrompt
	if patch.OverflowQueueID != "" {
		base.OverflowQueueID = patch.OverflowQueueID
	}
	if patch.IVRFlowID != "" {
		base.IVRFlowID = patch.IVRFlowID
	}
	if patch.SkillIDs != nil {
		base.SkillIDs = patch.SkillIDs
	}
	if patch.AgentIDs != nil {
		base.AgentIDs = patch.AgentIDs
	}
	return base
}

func (d SwitchRouterDeps) handleQueueConfigDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Admin.DeleteQueueConfig(r.Context(), chi.URLParam(r, "queueId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d SwitchRouterDeps) handleQueueConfigAgents(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentIDs []string `json:"agent_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Admin.SetQueueAgents(r.Context(), chi.URLParam(r, "queueId"), body.AgentIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d SwitchRouterDeps) handleQueueConfigSkills(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SkillIDs []string `json:"skill_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Admin.SetQueueSkills(r.Context(), chi.URLParam(r, "queueId"), body.SkillIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d SwitchRouterDeps) handleSkillConfigList(w http.ResponseWriter, r *http.Request) {
	items, err := d.Admin.ListSkillConfigs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (d SwitchRouterDeps) handleSkillConfigCreate(w http.ResponseWriter, r *http.Request) {
	var body ports.SkillConfig
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Admin.CreateSkillConfig(r.Context(), body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d SwitchRouterDeps) handleSkillConfigPatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Admin.UpdateSkillConfig(r.Context(), chi.URLParam(r, "skillId"), body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d SwitchRouterDeps) handleSkillConfigDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Admin.DeleteSkillConfig(r.Context(), chi.URLParam(r, "skillId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d SwitchRouterDeps) handleAgentConfigList(w http.ResponseWriter, r *http.Request) {
	items, err := d.Admin.ListAgentConfigs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (d SwitchRouterDeps) handleAgentConfigUpsert(w http.ResponseWriter, r *http.Request) {
	var body ports.AgentConfig
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if body.ID == "" {
		body.ID = chi.URLParam(r, "agentId")
	}
	out, err := d.Admin.UpsertAgentConfig(r.Context(), body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d SwitchRouterDeps) handleAgentConfigDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Admin.DeleteAgentConfig(r.Context(), chi.URLParam(r, "agentId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d SwitchRouterDeps) handleAgentConfigSkills(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SkillIDs []string `json:"skill_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Admin.SetAgentSkills(r.Context(), chi.URLParam(r, "agentId"), body.SkillIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d SwitchRouterDeps) handleDIDConfigList(w http.ResponseWriter, r *http.Request) {
	items, err := d.Admin.ListDIDConfigs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (d SwitchRouterDeps) handleDIDConfigUpsert(w http.ResponseWriter, r *http.Request) {
	var body ports.DIDConfig
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Admin.UpsertDIDConfig(r.Context(), body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d SwitchRouterDeps) handleDIDConfigPatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "didId")
	items, err := d.Admin.ListDIDConfigs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	var current ports.DIDConfig
	found := false
	for _, d := range items {
		if d.ID == id {
			current = d
			found = true
			break
		}
	}
	if !found {
		writeErr(w, errs.NotFound("DID 不存在"))
		return
	}
	var patch ports.DIDConfig
	if err := decodeJSON(r, &patch); err != nil {
		writeErr(w, err)
		return
	}
	if patch.TrunkID != "" {
		current.TrunkID = patch.TrunkID
	}
	if patch.DID != "" {
		current.DID = patch.DID
	}
	if patch.TargetType != "" {
		current.TargetType = patch.TargetType
	}
	if patch.TargetID != "" {
		current.TargetID = patch.TargetID
	}
	out, err := d.Admin.UpsertDIDConfig(r.Context(), current)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d SwitchRouterDeps) handleDIDConfigDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Admin.DeleteDIDConfig(r.Context(), chi.URLParam(r, "didId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d SwitchRouterDeps) handleIVRFlowList(w http.ResponseWriter, r *http.Request) {
	items, err := d.Admin.ListIVRFlows(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (d SwitchRouterDeps) handleIVRFlowGet(w http.ResponseWriter, r *http.Request) {
	out, err := d.Admin.GetIVRFlow(r.Context(), chi.URLParam(r, "flowId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func decodeIVRPayload(r *http.Request) (flowID, payload string, err error) {
	var body struct {
		ID          string          `json:"id"`
		FlowID      string          `json:"flow_id"`
		PayloadJSON string          `json:"payload_json"`
		Payload     json.RawMessage `json:"payload"`
	}
	if err = decodeJSON(r, &body); err != nil {
		return "", "", err
	}
	flowID = strings.TrimSpace(body.FlowID)
	if flowID == "" {
		flowID = strings.TrimSpace(body.ID)
	}
	payload = strings.TrimSpace(body.PayloadJSON)
	if payload == "" && len(body.Payload) > 0 {
		payload = string(body.Payload)
	}
	if payload == "" {
		return "", "", errs.InvalidRequest("payload_json 必填")
	}
	return flowID, payload, nil
}

func (d SwitchRouterDeps) handleIVRFlowCreate(w http.ResponseWriter, r *http.Request) {
	flowID, payload, err := decodeIVRPayload(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Admin.UpsertIVRFlow(r.Context(), flowID, payload)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (d SwitchRouterDeps) handleIVRFlowUpsert(w http.ResponseWriter, r *http.Request) {
	_, payload, err := decodeIVRPayload(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Admin.UpsertIVRFlow(r.Context(), chi.URLParam(r, "flowId"), payload)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d SwitchRouterDeps) handleIVRFlowDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Admin.DeleteIVRFlow(r.Context(), chi.URLParam(r, "flowId")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
