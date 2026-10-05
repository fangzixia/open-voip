package configio

import (
	"context"
	"sort"

	"gorm.io/gorm"

	"open-call/internal/ports"
	"open-call/internal/store/models"
)

func bundleFromActive(active ports.SwitchActiveConfiguration) Bundle {
	b := active.Bundle
	out := Bundle{
		Queues:      make([]models.Queue, 0, len(b.Queues)),
		Skills:      make([]models.Skill, 0, len(b.Skills)),
		Agents:      make([]models.Agent, 0, len(b.Agents)),
		DIDs:        make([]models.DIDRoute, 0, len(b.DIDs)),
		IVRFlows:    make([]models.IVRFlow, 0, len(b.IVRs)),
		IVRVersions: make([]models.IVRPublishedSnapshot, 0, len(b.IVRs)),
		AgentSkills: []models.AgentSkill{},
		QueueSkills: []models.QueueSkill{},
		QueueAgents: []models.QueueAgent{},
	}
	for _, sk := range b.Skills {
		out.Skills = append(out.Skills, models.Skill{ID: sk.ID, Name: sk.Name})
	}
	for _, ag := range b.Agents {
		out.Agents = append(out.Agents, models.Agent{ID: ag.ID, UserID: ag.UserRef, Extension: ag.Extension, VideoCapable: ag.VideoCapable, TerminalType: ag.TerminalType, SIPUsername: ag.SIPUsername})
		for _, sid := range ag.SkillIDs {
			out.AgentSkills = append(out.AgentSkills, models.AgentSkill{AgentID: ag.ID, SkillID: sid})
		}
	}
	for _, q := range b.Queues {
		row := models.Queue{
			ID: q.ID, Name: q.Name, VideoEnabled: q.VideoEnabled, MaxWaitSec: q.MaxWaitSec,
			DispatchStrategy: q.DispatchStrategy, RecordingPolicy: q.RecordingPolicy, OverflowAction: q.OverflowAction,
			WaitPrompt: q.WaitPrompt, AnnounceRecording: q.AnnounceRecording, PriorityEnabled: q.PriorityEnabled,
			BusinessHoursJSON: q.BusinessHoursJSON, AfterHoursAction: q.AfterHoursAction,
			ForceHangupOnCheckout: q.ForceHangupOnCheckout, ListenAnnounce: q.ListenAnnounce,
		}
		if q.OverflowQueueID != "" {
			row.OverflowQueueID = &q.OverflowQueueID
		}
		if q.IVRFlowID != "" {
			row.IVRFlowID = &q.IVRFlowID
		}
		if q.PostCallIVRFlowID != "" {
			row.PostCallIVRFlowID = &q.PostCallIVRFlowID
		}
		out.Queues = append(out.Queues, row)
		for _, sid := range q.SkillIDs {
			out.QueueSkills = append(out.QueueSkills, models.QueueSkill{QueueID: q.ID, SkillID: sid})
		}
		for _, aid := range q.AgentIDs {
			out.QueueAgents = append(out.QueueAgents, models.QueueAgent{QueueID: q.ID, AgentID: aid})
		}
	}
	for _, d := range b.DIDs {
		var target *string
		if d.TargetID != "" {
			target = &d.TargetID
		}
		out.DIDs = append(out.DIDs, models.DIDRoute{ID: d.ID, TrunkID: d.TrunkID, DID: d.DID, TargetType: d.TargetType, TargetID: target})
	}
	for _, ivr := range b.IVRs {
		out.IVRVersions = append(out.IVRVersions, models.IVRPublishedSnapshot{FlowID: ivr.FlowID, Version: ivr.Version, PayloadJSON: ivr.PayloadJSON})
		out.IVRFlows = append(out.IVRFlows, models.IVRFlow{ID: ivr.FlowID, Name: ivr.FlowID, DraftJSON: ivr.PayloadJSON})
	}
	return out
}

func compileSwitchBundle(ctx context.Context, db *gorm.DB, bundle Bundle) (ports.SwitchConfigBundle, error) {
	displayName := map[string]string{}
	for _, u := range bundle.Users {
		displayName[u.ID] = importUsername(u)
	}
	skillByAgent := map[string][]string{}
	for _, row := range bundle.AgentSkills {
		skillByAgent[row.AgentID] = append(skillByAgent[row.AgentID], row.SkillID)
	}
	agentsByQueue := map[string][]string{}
	skillsByQueue := map[string][]string{}
	for _, row := range bundle.QueueAgents {
		agentsByQueue[row.QueueID] = append(agentsByQueue[row.QueueID], row.AgentID)
	}
	for _, row := range bundle.QueueSkills {
		skillsByQueue[row.QueueID] = append(skillsByQueue[row.QueueID], row.SkillID)
	}
	out := ports.SwitchConfigBundle{
		Queues: []ports.SwitchQueueConfig{}, Skills: []ports.SwitchSkillConfig{},
		Agents: []ports.SwitchAgentConfig{}, DIDs: []ports.SwitchDIDConfig{}, IVRs: []ports.SwitchIVRConfig{},
	}
	for _, sk := range bundle.Skills {
		out.Skills = append(out.Skills, ports.SwitchSkillConfig{ID: sk.ID, Name: sk.Name})
	}
	for _, ag := range bundle.Agents {
		terminal := ag.TerminalType
		if terminal == "" {
			terminal = "webrtc"
		}
		disabled := false
		for _, u := range bundle.Users {
			if u.ID == ag.UserID {
				disabled = u.Disabled
				break
			}
		}
		out.Agents = append(out.Agents, ports.SwitchAgentConfig{
			ID: ag.ID, UserRef: ag.UserID, Extension: ag.Extension, DisplayName: displayName[ag.UserID],
			VideoCapable: ag.VideoCapable, TerminalType: terminal, SIPUsername: ag.SIPUsername,
			Enabled: !disabled, SkillIDs: uniqueStrings(skillByAgent[ag.ID]),
		})
	}
	for _, row := range bundle.Queues {
		q := ports.SwitchQueueConfig{
			ID: row.ID, Name: row.Name, VideoEnabled: row.VideoEnabled, MaxWaitSec: row.MaxWaitSec,
			DispatchStrategy: row.DispatchStrategy, RecordingPolicy: row.RecordingPolicy, OverflowAction: row.OverflowAction,
			WaitPrompt: row.WaitPrompt, AudioProfile: row.AudioProfile, AnnounceRecording: row.AnnounceRecording, PriorityEnabled: row.PriorityEnabled,
			BusinessHoursJSON: row.BusinessHoursJSON, AfterHoursAction: row.AfterHoursAction,
			ForceHangupOnCheckout: row.ForceHangupOnCheckout, ListenAnnounce: row.ListenAnnounce,
			SkillIDs: uniqueStrings(skillsByQueue[row.ID]), AgentIDs: uniqueStrings(agentsByQueue[row.ID]),
		}
		if row.OverflowQueueID != nil {
			q.OverflowQueueID = *row.OverflowQueueID
		}
		if row.IVRFlowID != nil {
			q.IVRFlowID = *row.IVRFlowID
		}
		if row.PostCallIVRFlowID != nil {
			q.PostCallIVRFlowID = *row.PostCallIVRFlowID
		}
		out.Queues = append(out.Queues, q)
	}
	for _, row := range bundle.DIDs {
		targetID := ""
		if row.TargetID != nil {
			targetID = *row.TargetID
		}
		out.DIDs = append(out.DIDs, ports.SwitchDIDConfig{ID: row.ID, TrunkID: row.TrunkID, DID: row.DID, TargetType: row.TargetType, TargetID: targetID})
	}
	latestIVR := map[string]models.IVRPublishedSnapshot{}
	for _, snap := range bundle.IVRVersions {
		if cur, ok := latestIVR[snap.FlowID]; !ok || snap.Version > cur.Version {
			latestIVR[snap.FlowID] = snap
		}
	}
	flowIDs := make([]string, 0, len(latestIVR))
	for id := range latestIVR {
		flowIDs = append(flowIDs, id)
	}
	sort.Strings(flowIDs)
	for _, id := range flowIDs {
		snap := latestIVR[id]
		out.IVRs = append(out.IVRs, ports.SwitchIVRConfig{FlowID: snap.FlowID, Version: snap.Version, PayloadJSON: snap.PayloadJSON})
	}
	return out, nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
