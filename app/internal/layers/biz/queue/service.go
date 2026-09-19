package queue

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"github.com/google/uuid"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
	"open-voip/internal/ports/dto"
	"open-voip/internal/store/models"
)

// DTO 队列对外表示。
type DTO struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	VideoEnabled          bool     `json:"video_enabled"`
	MaxWaitSec            int      `json:"max_wait_sec"`
	Strategy              string   `json:"strategy"`
	OverflowPolicy        string   `json:"overflow_policy,omitempty"`
	OverflowQueueID       string   `json:"overflow_queue_id,omitempty"`
	RecordingPolicy       string   `json:"recording_policy"`
	IVRFlowID             string   `json:"ivr_flow_id,omitempty"`
	WaitPrompt            string   `json:"wait_prompt,omitempty"`
	AnnounceRecording     bool     `json:"announce_recording"`
	PriorityEnabled       bool     `json:"priority_enabled"`
	SkillIDs              []string `json:"skill_ids,omitempty"`
	BusinessHoursJSON     string   `json:"business_hours_json,omitempty"`
	AfterHoursAction      string   `json:"after_hours_action,omitempty"`
	ForceHangupOnCheckout bool     `json:"force_hangup_on_checkout"`
	ListenAnnounce        bool     `json:"listen_announce"`
}

// CreateInput 创建队列。
type CreateInput struct {
	Name                  string   `json:"name"`
	VideoEnabled          bool     `json:"video_enabled"`
	MaxWaitSec            int      `json:"max_wait_sec"`
	Strategy              string   `json:"strategy"`
	OverflowPolicy        string   `json:"overflow_policy"`
	OverflowQueueID       string   `json:"overflow_queue_id"`
	RecordingPolicy       string   `json:"recording_policy"`
	IVRFlowID             string   `json:"ivr_flow_id"`
	WaitPrompt            string   `json:"wait_prompt"`
	AnnounceRecording     bool     `json:"announce_recording"`
	PriorityEnabled       bool     `json:"priority_enabled"`
	SkillIDs              []string `json:"skill_ids"`
	BusinessHoursJSON     string   `json:"business_hours_json"`
	AfterHoursAction      string   `json:"after_hours_action"`
	ForceHangupOnCheckout *bool    `json:"force_hangup_on_checkout"`
	ListenAnnounce        *bool    `json:"listen_announce"`
}

// ListResult 分页队列。
type ListResult struct {
	Items    []DTO `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// PolicyDefaults 无队列通话及队列策略共用的组织级录音默认值（L4 配置，不交给 L3 拼装）。
type PolicyDefaults struct {
	// Mode 无队列时的默认录音模式，通常为 audio。
	Mode string
	// NotifyMessage 录音告知文案。
	NotifyMessage string
	// RetainDays 保留天数。
	RetainDays int
}

// Service 队列 CRUD、ACD 与录音策略。
type Service struct {
	db       *gorm.DB
	events   ports.AgentEventPublisher
	defaults PolicyDefaults
	mu       sync.Mutex
	lastRR   map[string]string
}

// NewService 创建队列/ACD 服务。
func NewService(db *gorm.DB, events ports.AgentEventPublisher, defaults PolicyDefaults) *Service {
	if defaults.Mode == "" {
		defaults.Mode = "audio"
	}
	if defaults.RetainDays <= 0 {
		defaults.RetainDays = 90
	}
	return &Service{db: db, events: events, defaults: defaults, lastRR: map[string]string{}}
}

var _ ports.ACDDispatchPort = (*Service)(nil)
var _ ports.RecordingPolicyPort = (*Service)(nil)

// List 分页。
func (s *Service) List(ctx context.Context, page, pageSize int) (ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&models.Queue{}).Count(&total).Error; err != nil {
		return ListResult{}, err
	}
	var rows []models.Queue
	if err := s.db.WithContext(ctx).Order("created_at").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return ListResult{}, err
	}
	items := make([]DTO, 0, len(rows))
	for _, r := range rows {
		d := toDTO(r)
		d.SkillIDs = queueSkills(s.db.WithContext(ctx), r.ID)
		items = append(items, d)
	}
	return ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// ListPublic Demo 访客可见队列。
func (s *Service) ListPublic(ctx context.Context) ([]DTO, error) {
	var rows []models.Queue
	if err := s.db.WithContext(ctx).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]DTO, 0, len(rows))
	for _, r := range rows {
		items = append(items, toDTO(r))
	}
	return items, nil
}

// Get 按 ID。
func (s *Service) Get(ctx context.Context, id string) (DTO, error) {
	var row models.Queue
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DTO{}, errs.NotFound("队列不存在")
		}
		return DTO{}, err
	}
	d := toDTO(row)
	d.SkillIDs = queueSkills(s.db.WithContext(ctx), row.ID)
	return d, nil
}

// Create 创建队列。
func (s *Service) Create(ctx context.Context, in CreateInput) (DTO, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return DTO{}, errs.InvalidRequest("队列名称必填")
	}
	if in.MaxWaitSec <= 0 {
		in.MaxWaitSec = 300
	}
	if in.Strategy == "" {
		in.Strategy = "longest_idle"
	}
	if in.Strategy != "longest_idle" && in.Strategy != "round_robin" {
		return DTO{}, errs.InvalidRequest("分配策略无效")
	}
	if in.RecordingPolicy == "" {
		in.RecordingPolicy = "off"
	}
	now := time.Now().UTC()
	force := true
	if in.ForceHangupOnCheckout != nil {
		force = *in.ForceHangupOnCheckout
	}
	row := models.Queue{
		ID:                    uuid.New().String(),
		Name:                  in.Name,
		VideoEnabled:          in.VideoEnabled,
		MaxWaitSec:            in.MaxWaitSec,
		DispatchStrategy:      in.Strategy,
		RecordingPolicy:       in.RecordingPolicy,
		OverflowAction:        in.OverflowPolicy,
		WaitPrompt:            in.WaitPrompt,
		AnnounceRecording:     in.AnnounceRecording,
		PriorityEnabled:       in.PriorityEnabled,
		BusinessHoursJSON:     in.BusinessHoursJSON,
		AfterHoursAction:      in.AfterHoursAction,
		ForceHangupOnCheckout: force,
		ListenAnnounce:        in.ListenAnnounce != nil && *in.ListenAnnounce,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if in.OverflowQueueID != "" {
		id := in.OverflowQueueID
		row.OverflowQueueID = &id
	}
	if in.IVRFlowID != "" {
		id := in.IVRFlowID
		row.IVRFlowID = &id
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return replaceQueueSkills(tx, row.ID, in.SkillIDs)
	})
	if err != nil {
		return DTO{}, err
	}
	return s.Get(ctx, row.ID)
}

// Update 部分更新。
func (s *Service) Update(ctx context.Context, id string, in CreateInput) (DTO, error) {
	var row models.Queue
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DTO{}, errs.NotFound("队列不存在")
		}
		return DTO{}, err
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if in.Name != "" {
		updates["name"] = in.Name
	}
	updates["video_enabled"] = in.VideoEnabled
	if in.MaxWaitSec > 0 {
		updates["max_wait_sec"] = in.MaxWaitSec
	}
	if in.Strategy != "" {
		updates["dispatch_strategy"] = in.Strategy
	}
	if in.OverflowPolicy != "" {
		updates["overflow_action"] = in.OverflowPolicy
	}
	if in.RecordingPolicy != "" {
		updates["recording_policy"] = in.RecordingPolicy
	}
	if in.WaitPrompt != "" {
		updates["wait_prompt"] = in.WaitPrompt
	}
	updates["announce_recording"] = in.AnnounceRecording
	updates["priority_enabled"] = in.PriorityEnabled
	if in.BusinessHoursJSON != "" {
		updates["business_hours_json"] = in.BusinessHoursJSON
	}
	if in.AfterHoursAction != "" {
		updates["after_hours_action"] = in.AfterHoursAction
	}
	if in.OverflowQueueID != "" {
		updates["overflow_queue_id"] = in.OverflowQueueID
	}
	if in.IVRFlowID != "" {
		updates["ivr_flow_id"] = in.IVRFlowID
	}
	if in.ForceHangupOnCheckout != nil {
		updates["force_hangup_on_checkout"] = *in.ForceHangupOnCheckout
	}
	if in.ListenAnnounce != nil {
		updates["listen_announce"] = *in.ListenAnnounce
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&row).Updates(updates).Error; err != nil {
			return err
		}
		if in.SkillIDs != nil {
			return replaceQueueSkills(tx, id, in.SkillIDs)
		}
		return nil
	})
	if err != nil {
		return DTO{}, err
	}
	return s.Get(ctx, id)
}

// Delete 删除队列。
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Where("queue_id = ?", id).Delete(&models.QueueAgent{})
		tx.Where("queue_id = ?", id).Delete(&models.AgentSessionQueue{})
		res := tx.Delete(&models.Queue{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.NotFound("队列不存在")
		}
		return nil
	})
}

// BindAgents 覆盖绑定坐席。
func (s *Service) BindAgents(ctx context.Context, queueID string, agentIDs []string) error {
	if _, err := s.Get(ctx, queueID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("queue_id = ?", queueID).Delete(&models.QueueAgent{}).Error; err != nil {
			return err
		}
		for _, aid := range agentIDs {
			if err := tx.Create(&models.QueueAgent{QueueID: queueID, AgentID: aid}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// AgentIDs 队列已绑定坐席。
func (s *Service) AgentIDs(ctx context.Context, queueID string) ([]string, error) {
	var ids []string
	if err := s.db.WithContext(ctx).Model(&models.QueueAgent{}).Where("queue_id = ?", queueID).Pluck("agent_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// RequestAgent 原子选人并标记 ringing。
func (s *Service) RequestAgent(ctx context.Context, req dto.DispatchRequest) (dto.DispatchResult, error) {
	var q models.Queue
	if err := s.db.WithContext(ctx).First(&q, "id = ?", req.QueueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.DispatchResult{}, errs.NotFound("队列不存在")
		}
		return dto.DispatchResult{}, err
	}
	var picked string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Model(&models.AgentSession{}).
			Joins("JOIN agents ON agents.id = agent_sessions.agent_id").
			Joins("JOIN agent_session_queues ON agent_session_queues.agent_id = agent_sessions.agent_id").
			Where("agent_sessions.state = ?", "idle").
			Where("agent_session_queues.queue_id = ?", req.QueueID)
		if req.RequireVideo {
			query = query.Where("agents.video_capable = ?", true)
		}
		var needSkills []string
		_ = tx.Model(&models.QueueSkill{}).Where("queue_id = ?", req.QueueID).Pluck("skill_id", &needSkills)
		if len(req.SkillIDs) > 0 {
			needSkills = req.SkillIDs
		}
		if len(needSkills) > 0 {
			query = query.Joins("JOIN agent_skills ON agent_skills.agent_id = agent_sessions.agent_id").
				Where("agent_skills.skill_id IN ?", needSkills)
		}
		if q.DispatchStrategy == "round_robin" {
			query = query.Order("agents.id ASC")
		} else {
			query = query.Order("agent_sessions.updated_at ASC")
		}
		var sessions []models.AgentSession
		if err := query.Find(&sessions).Error; err != nil {
			return err
		}
		if len(sessions) == 0 {
			return nil
		}
		choice := sessions[0]
		if q.DispatchStrategy == "round_robin" {
			s.mu.Lock()
			last := s.lastRR[req.QueueID]
			s.mu.Unlock()
			for _, sess := range sessions {
				if sess.AgentID > last {
					choice = sess
					break
				}
			}
		}
		now := time.Now().UTC()
		res := tx.Model(&models.AgentSession{}).
			Where("agent_id = ? AND state = ?", choice.AgentID, "idle").
			Updates(map[string]any{"state": "ringing", "updated_at": now, "busy_reason": ""})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if err := tx.Create(&models.AgentStateLog{
			ID:        uuid.New().String(),
			AgentID:   choice.AgentID,
			FromState: "idle",
			ToState:   "ringing",
			Reason:    "acd:" + req.CallID,
			CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		picked = choice.AgentID
		if q.DispatchStrategy == "round_robin" {
			s.mu.Lock()
			s.lastRR[req.QueueID] = choice.AgentID
			s.mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return dto.DispatchResult{}, err
	}
	if picked != "" && s.events != nil {
		_ = s.events.PublishAgentEvent(ctx, ports.AgentEvent{
			Type:    "agent.state_changed",
			AgentID: picked,
			Payload: map[string]any{"agent_id": picked, "state": "ringing", "busy_reason": ""},
		})
	}
	return dto.DispatchResult{AgentID: picked}, nil
}

// ForQueue 返回队列录音策略；无队列时用组织默认，未知队列视为关闭。
func (s *Service) ForQueue(ctx context.Context, queueID string) (dto.RecordingPolicy, error) {
	if queueID == "" {
		return composeRecordingPolicy(nil, s.defaults), nil
	}
	var q models.Queue
	if err := s.db.WithContext(ctx).First(&q, "id = ?", queueID).Error; err != nil {
		off := s.defaults
		off.Mode = "off"
		return composeRecordingPolicy(nil, off), nil
	}
	return composeRecordingPolicy(&q, s.defaults), nil
}

func composeRecordingPolicy(q *models.Queue, d PolicyDefaults) dto.RecordingPolicy {
	out := dto.RecordingPolicy{
		Mode:          d.Mode,
		NotifyMessage: d.NotifyMessage,
		RetainDays:    d.RetainDays,
	}
	if q == nil {
		return out
	}
	mode := q.RecordingPolicy
	if mode == "" {
		mode = "off"
	}
	out.Mode = mode
	out.NotifyGuest = q.AnnounceRecording
	return out
}

func queueSkills(db *gorm.DB, queueID string) []string {
	var ids []string
	_ = db.Model(&models.QueueSkill{}).Where("queue_id = ?", queueID).Pluck("skill_id", &ids)
	if ids == nil {
		return []string{}
	}
	return ids
}

func replaceQueueSkills(tx *gorm.DB, queueID string, skillIDs []string) error {
	if err := tx.Where("queue_id = ?", queueID).Delete(&models.QueueSkill{}).Error; err != nil {
		return err
	}
	for _, id := range skillIDs {
		if err := tx.Create(&models.QueueSkill{QueueID: queueID, SkillID: id}).Error; err != nil {
			return err
		}
	}
	return nil
}

func toDTO(row models.Queue) DTO {
	out := DTO{
		ID:                    row.ID,
		Name:                  row.Name,
		VideoEnabled:          row.VideoEnabled,
		MaxWaitSec:            row.MaxWaitSec,
		Strategy:              row.DispatchStrategy,
		OverflowPolicy:        row.OverflowAction,
		RecordingPolicy:       row.RecordingPolicy,
		WaitPrompt:            row.WaitPrompt,
		AnnounceRecording:     row.AnnounceRecording,
		PriorityEnabled:       row.PriorityEnabled,
		BusinessHoursJSON:     row.BusinessHoursJSON,
		AfterHoursAction:      row.AfterHoursAction,
		ForceHangupOnCheckout: row.ForceHangupOnCheckout,
		ListenAnnounce:        row.ListenAnnounce,
		SkillIDs:              []string{},
	}
	if row.OverflowQueueID != nil {
		out.OverflowQueueID = *row.OverflowQueueID
	}
	if row.IVRFlowID != nil {
		out.IVRFlowID = *row.IVRFlowID
	}
	return out
}
