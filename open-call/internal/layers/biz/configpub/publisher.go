package configpub

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/ports"
	"open-call/internal/store/models"
)

// Publisher compiles business drafts into one complete immutable Switch snapshot.
type Publisher struct {
	db          *gorm.DB
	switchAdmin ports.SwitchAdminPort
}

func NewPublisher(db *gorm.DB, switchAdmin ports.SwitchAdminPort) *Publisher {
	return &Publisher{db: db, switchAdmin: switchAdmin}
}

func (p *Publisher) Publish(ctx context.Context) (ports.SwitchConfigVersion, error) {
	bundle, err := p.Build(ctx)
	if err != nil {
		return ports.SwitchConfigVersion{}, err
	}
	stored, err := p.switchAdmin.StoreConfig(ctx, bundle)
	if err != nil {
		return ports.SwitchConfigVersion{}, err
	}
	return p.switchAdmin.ActivateConfig(ctx, stored.Version)
}

func (p *Publisher) Build(ctx context.Context) (ports.SwitchConfigBundle, error) {
	var out ports.SwitchConfigBundle
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reader := &Publisher{db: tx}
		var err error
		out, err = reader.build(ctx)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}

func (p *Publisher) build(ctx context.Context) (ports.SwitchConfigBundle, error) {
	var queues []models.Queue
	var skills []models.Skill
	var agents []models.Agent
	var dids []models.DIDRoute
	if err := p.db.WithContext(ctx).Order("id").Find(&queues).Error; err != nil {
		return ports.SwitchConfigBundle{}, err
	}
	if err := p.db.WithContext(ctx).Order("id").Find(&skills).Error; err != nil {
		return ports.SwitchConfigBundle{}, err
	}
	if err := p.db.WithContext(ctx).Order("id").Find(&agents).Error; err != nil {
		return ports.SwitchConfigBundle{}, err
	}
	if err := p.db.WithContext(ctx).Order("id").Find(&dids).Error; err != nil {
		return ports.SwitchConfigBundle{}, err
	}

	out := ports.SwitchConfigBundle{Queues: []ports.SwitchQueueConfig{}, Skills: []ports.SwitchSkillConfig{}, Agents: []ports.SwitchAgentConfig{}, DIDs: []ports.SwitchDIDConfig{}, IVRs: []ports.SwitchIVRConfig{}}
	for _, row := range skills {
		out.Skills = append(out.Skills, ports.SwitchSkillConfig{ID: row.ID, Name: row.Name})
	}
	for _, row := range agents {
		var skillIDs []string
		if err := p.db.WithContext(ctx).Model(&models.AgentSkill{}).Where("agent_id = ?", row.ID).Order("skill_id").Pluck("skill_id", &skillIDs).Error; err != nil {
			return out, err
		}
		var user models.User
		if err := p.db.WithContext(ctx).First(&user, "id = ?", row.UserID).Error; err != nil {
			return out, err
		}
		terminal := row.TerminalType
		if terminal == "" {
			terminal = "webrtc"
		}
		out.Agents = append(out.Agents, ports.SwitchAgentConfig{ID: row.ID, UserRef: row.UserID, Extension: row.Extension, DisplayName: user.DisplayName, VideoCapable: row.VideoCapable, TerminalType: terminal, SIPUsername: row.SIPUsername, Enabled: !user.Disabled, SkillIDs: skillIDs})
	}
	flows := map[string]bool{}
	for _, row := range queues {
		var skillIDs, agentIDs []string
		if err := p.db.WithContext(ctx).Model(&models.QueueSkill{}).Where("queue_id = ?", row.ID).Order("skill_id").Pluck("skill_id", &skillIDs).Error; err != nil {
			return out, err
		}
		if err := p.db.WithContext(ctx).Model(&models.QueueAgent{}).Where("queue_id = ?", row.ID).Order("agent_id").Pluck("agent_id", &agentIDs).Error; err != nil {
			return out, err
		}
		q := ports.SwitchQueueConfig{ID: row.ID, Name: row.Name, VideoEnabled: row.VideoEnabled, MaxWaitSec: row.MaxWaitSec, DispatchStrategy: row.DispatchStrategy, RecordingPolicy: row.RecordingPolicy, OverflowAction: row.OverflowAction, WaitPrompt: row.WaitPrompt, AnnounceRecording: row.AnnounceRecording, PriorityEnabled: row.PriorityEnabled, BusinessHoursJSON: row.BusinessHoursJSON, AfterHoursAction: row.AfterHoursAction, ForceHangupOnCheckout: row.ForceHangupOnCheckout, ListenAnnounce: row.ListenAnnounce, SkillIDs: skillIDs, AgentIDs: agentIDs}
		if row.OverflowQueueID != nil {
			q.OverflowQueueID = *row.OverflowQueueID
		}
		if row.IVRFlowID != nil {
			q.IVRFlowID = *row.IVRFlowID
			flows[*row.IVRFlowID] = true
		}
		out.Queues = append(out.Queues, q)
	}
	for _, row := range dids {
		targetID := ""
		if row.TargetID != nil {
			targetID = *row.TargetID
		}
		out.DIDs = append(out.DIDs, ports.SwitchDIDConfig{ID: row.ID, TrunkID: row.TrunkID, DID: row.DID, TargetType: row.TargetType, TargetID: targetID})
		if row.TargetType == "ivr" && targetID != "" {
			flows[targetID] = true
		}
	}
	flowIDs := make([]string, 0, len(flows))
	for id := range flows {
		flowIDs = append(flowIDs, id)
	}
	sort.Strings(flowIDs)
	for _, flowID := range flowIDs {
		var snap models.IVRPublishedSnapshot
		if err := p.db.WithContext(ctx).Where("flow_id = ?", flowID).Order("version DESC").First(&snap).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return out, errs.InvalidRequest("队列引用了未发布的 IVR")
			}
			return out, err
		}
		out.IVRs = append(out.IVRs, ports.SwitchIVRConfig{FlowID: snap.FlowID, Version: snap.Version, PayloadJSON: snap.PayloadJSON})
	}
	return out, nil
}
