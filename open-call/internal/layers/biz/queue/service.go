package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-call/internal/errs"
	"open-call/internal/observability"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
	"open-call/internal/store/models"
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

// UpdateInput uses pointers so PATCH preserves omitted booleans and can clear nullable text/IDs.
type UpdateInput struct {
	Name                  *string   `json:"name"`
	VideoEnabled          *bool     `json:"video_enabled"`
	MaxWaitSec            *int      `json:"max_wait_sec"`
	Strategy              *string   `json:"strategy"`
	OverflowPolicy        *string   `json:"overflow_policy"`
	OverflowQueueID       *string   `json:"overflow_queue_id"`
	RecordingPolicy       *string   `json:"recording_policy"`
	IVRFlowID             *string   `json:"ivr_flow_id"`
	WaitPrompt            *string   `json:"wait_prompt"`
	AnnounceRecording     *bool     `json:"announce_recording"`
	PriorityEnabled       *bool     `json:"priority_enabled"`
	SkillIDs              *[]string `json:"skill_ids"`
	BusinessHoursJSON     *string   `json:"business_hours_json"`
	AfterHoursAction      *string   `json:"after_hours_action"`
	ForceHangupOnCheckout *bool     `json:"force_hangup_on_checkout"`
	ListenAnnounce        *bool     `json:"listen_announce"`
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
	if err := validateQueueValues(in.RecordingPolicy, in.OverflowPolicy, in.AfterHoursAction, in.BusinessHoursJSON); err != nil {
		return DTO{}, err
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
		row.OverflowQueueID = new(in.OverflowQueueID)
	}
	if in.IVRFlowID != "" {
		row.IVRFlowID = new(in.IVRFlowID)
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateReferences(tx, row.ID, in.OverflowQueueID, in.IVRFlowID, in.SkillIDs); err != nil {
			return err
		}
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
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (DTO, error) {
	var row models.Queue
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DTO{}, errs.NotFound("队列不存在")
		}
		return DTO{}, err
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return DTO{}, errs.InvalidRequest("队列名称不能为空")
		}
		updates["name"] = name
	}
	if in.VideoEnabled != nil {
		updates["video_enabled"] = *in.VideoEnabled
	}
	if in.MaxWaitSec != nil {
		if *in.MaxWaitSec <= 0 {
			return DTO{}, errs.InvalidRequest("max_wait_sec 必须大于 0")
		}
		updates["max_wait_sec"] = *in.MaxWaitSec
	}
	if in.Strategy != nil {
		if *in.Strategy != "longest_idle" && *in.Strategy != "round_robin" {
			return DTO{}, errs.InvalidRequest("分配策略无效")
		}
		updates["dispatch_strategy"] = *in.Strategy
	}
	if in.OverflowPolicy != nil {
		updates["overflow_action"] = strings.TrimSpace(*in.OverflowPolicy)
	}
	if in.RecordingPolicy != nil {
		updates["recording_policy"] = strings.TrimSpace(*in.RecordingPolicy)
	}
	if in.WaitPrompt != nil {
		updates["wait_prompt"] = *in.WaitPrompt
	}
	if in.AnnounceRecording != nil {
		updates["announce_recording"] = *in.AnnounceRecording
	}
	if in.PriorityEnabled != nil {
		updates["priority_enabled"] = *in.PriorityEnabled
	}
	if in.BusinessHoursJSON != nil {
		updates["business_hours_json"] = *in.BusinessHoursJSON
	}
	if in.AfterHoursAction != nil {
		updates["after_hours_action"] = strings.TrimSpace(*in.AfterHoursAction)
	}
	if in.OverflowQueueID != nil {
		if strings.TrimSpace(*in.OverflowQueueID) == "" {
			updates["overflow_queue_id"] = nil
		} else {
			updates["overflow_queue_id"] = strings.TrimSpace(*in.OverflowQueueID)
		}
	}
	if in.IVRFlowID != nil {
		if strings.TrimSpace(*in.IVRFlowID) == "" {
			updates["ivr_flow_id"] = nil
		} else {
			updates["ivr_flow_id"] = strings.TrimSpace(*in.IVRFlowID)
		}
	}
	if in.ForceHangupOnCheckout != nil {
		updates["force_hangup_on_checkout"] = *in.ForceHangupOnCheckout
	}
	if in.ListenAnnounce != nil {
		updates["listen_announce"] = *in.ListenAnnounce
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		recording, overflow, after, hours := row.RecordingPolicy, row.OverflowAction, row.AfterHoursAction, row.BusinessHoursJSON
		if in.RecordingPolicy != nil {
			recording = strings.TrimSpace(*in.RecordingPolicy)
		}
		if in.OverflowPolicy != nil {
			overflow = strings.TrimSpace(*in.OverflowPolicy)
		}
		if in.AfterHoursAction != nil {
			after = strings.TrimSpace(*in.AfterHoursAction)
		}
		if in.BusinessHoursJSON != nil {
			hours = *in.BusinessHoursJSON
		}
		if err := validateQueueValues(recording, overflow, after, hours); err != nil {
			return err
		}
		overflowID, ivrID := "", ""
		if row.OverflowQueueID != nil {
			overflowID = *row.OverflowQueueID
		}
		if row.IVRFlowID != nil {
			ivrID = *row.IVRFlowID
		}
		if in.OverflowQueueID != nil {
			overflowID = strings.TrimSpace(*in.OverflowQueueID)
		}
		if in.IVRFlowID != nil {
			ivrID = strings.TrimSpace(*in.IVRFlowID)
		}
		skills := queueSkills(tx, id)
		if in.SkillIDs != nil {
			skills = *in.SkillIDs
		}
		if err := validateReferences(tx, id, overflowID, ivrID, skills); err != nil {
			return err
		}
		if err := tx.Model(&row).Updates(updates).Error; err != nil {
			return err
		}
		if in.SkillIDs != nil {
			return replaceQueueSkills(tx, id, *in.SkillIDs)
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
		var refs int64
		if err := tx.Model(&models.DIDRoute{}).Where("queue_id = ?", id).Count(&refs).Error; err != nil {
			return err
		}
		if refs > 0 {
			return errs.Conflict("队列仍被 DID 路由引用", "")
		}
		if err := tx.Model(&models.Queue{}).Where("overflow_queue_id = ?", id).Count(&refs).Error; err != nil {
			return err
		}
		if refs > 0 {
			return errs.Conflict("队列仍被其他队列的溢出策略引用", "")
		}
		if err := tx.Model(&models.GuestSession{}).Where("queue_id = ? AND expires_at > ?", id, time.Now().UTC()).Count(&refs).Error; err != nil {
			return err
		}
		if refs > 0 {
			return errs.Conflict("队列仍有有效访客会话", "")
		}
		if err := tx.Model(&models.IVRFlow{}).Where("draft_json LIKE ?", "%"+id+"%").Count(&refs).Error; err != nil {
			return err
		}
		if refs > 0 {
			return errs.Conflict("队列仍被 IVR 草稿引用", "")
		}
		tx.Where("queue_id = ?", id).Delete(&models.QueueAgent{})
		tx.Where("queue_id = ?", id).Delete(&models.AgentSessionQueue{})
		tx.Where("queue_id = ?", id).Delete(&models.QueueSkill{})
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
		if len(agentIDs) > 0 {
			var n int64
			if err := tx.Model(&models.Agent{}).Where("id IN ?", agentIDs).Count(&n).Error; err != nil {
				return err
			}
			if n != int64(len(uniqueStrings(agentIDs))) {
				return errs.InvalidRequest("agent_ids 包含不存在的坐席")
			}
		}
		if err := tx.Where("queue_id = ?", queueID).Delete(&models.QueueAgent{}).Error; err != nil {
			return err
		}
		for _, aid := range uniqueStrings(agentIDs) {
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
	ctx = observability.With(ctx, observability.Context{CallID: req.CallID, QueueID: req.QueueID})
	observability.Emit(ctx, "acd.dispatch.requested", map[string]any{"require_video": req.RequireVideo, "skill_ids": req.SkillIDs})
	if req.CallID == "" {
		return dto.DispatchResult{}, errs.InvalidRequest("call_id 必填")
	}
	var q models.Queue
	if err := s.db.WithContext(ctx).First(&q, "id = ?", req.QueueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.DispatchResult{}, errs.NotFound("队列不存在")
		}
		return dto.DispatchResult{}, err
	}
	var picked string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 同一通话的重复派单请求共用数据库锁，避免同时占用多个坐席。
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", req.CallID).Error; err != nil {
			return err
		}
		var existing models.AgentSession
		if err := tx.Where("current_call_id = ? AND state IN ?", req.CallID, []string{"ringing", "on_call"}).First(&existing).Error; err == nil {
			picked = existing.AgentID
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// 跳过其他事务正分配的坐席，仅从空闲且满足视频、队列、技能条件的会话中选人。
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: clause.CurrentTable}, Options: "SKIP LOCKED"}).
			Model(&models.AgentSession{}).
			Joins("JOIN oc_agents ON oc_agents.id = oc_agent_sessions.agent_id").
			Joins("JOIN oc_agent_session_queues ON oc_agent_session_queues.agent_id = oc_agent_sessions.agent_id").
			Where("oc_agent_sessions.state = ? AND oc_agent_sessions.pending_checkout = false", "idle").
			Where("EXISTS (SELECT 1 FROM oc_users u WHERE u.id = oc_agents.user_id AND u.disabled = false)").
			Where("oc_agent_session_queues.queue_id = ?", req.QueueID)
		if req.RequireVideo {
			query = query.Where("oc_agents.video_capable = ?", true)
		}
		var needSkills []string
		if err := tx.Model(&models.QueueSkill{}).Where("queue_id = ?", req.QueueID).Pluck("skill_id", &needSkills).Error; err != nil {
			return err
		}
		if len(req.SkillIDs) > 0 {
			needSkills = req.SkillIDs
		}
		if len(needSkills) > 0 {
			query = query.Where("NOT EXISTS (SELECT 1 FROM oc_queue_skills qs WHERE qs.queue_id = ? AND NOT EXISTS (SELECT 1 FROM oc_agent_skills a WHERE a.agent_id = oc_agent_sessions.agent_id AND a.skill_id = qs.skill_id))", req.QueueID)
			for _, skillID := range needSkills {
				query = query.Where("EXISTS (SELECT 1 FROM oc_agent_skills a WHERE a.agent_id = oc_agent_sessions.agent_id AND a.skill_id = ?)", skillID)
			}
		}
		// 轮询策略用上次选中的坐席作为游标；其他策略选最久未更新的空闲坐席。
		if q.DispatchStrategy == "round_robin" {
			query = query.Order("oc_agents.id ASC")
		} else {
			query = query.Order("oc_agent_sessions.updated_at ASC")
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
			Updates(map[string]any{"state": "ringing", "updated_at": now, "busy_reason": "", "current_call_id": req.CallID})
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
		observability.Emit(ctx, "acd.dispatch.failed", map[string]any{"error": err.Error()})
		return dto.DispatchResult{}, err
	}
	if picked != "" && s.events != nil {
		_ = s.events.PublishAgentEvent(ctx, ports.AgentEvent{
			Type:    "agent.state_changed",
			AgentID: picked,
			Payload: map[string]any{"agent_id": picked, "state": "ringing", "busy_reason": "", "current_call_id": req.CallID},
		})
	}
	resultCtx := observability.With(ctx, observability.Context{AgentID: picked})
	if picked == "" {
		observability.Emit(resultCtx, "acd.dispatch.no_agent", nil)
	} else {
		observability.Emit(resultCtx, "acd.dispatch.assigned", map[string]any{"strategy": q.DispatchStrategy})
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
	if len(skillIDs) > 0 {
		var n int64
		if err := tx.Model(&models.Skill{}).Where("id IN ?", skillIDs).Count(&n).Error; err != nil {
			return err
		}
		if n != int64(len(uniqueStrings(skillIDs))) {
			return errs.InvalidRequest("skill_ids 包含不存在的技能")
		}
	}
	if err := tx.Where("queue_id = ?", queueID).Delete(&models.QueueSkill{}).Error; err != nil {
		return err
	}
	for _, id := range uniqueStrings(skillIDs) {
		if err := tx.Create(&models.QueueSkill{QueueID: queueID, SkillID: id}).Error; err != nil {
			return err
		}
	}
	return nil
}

func validateQueueValues(recording, overflow, after, hours string) error {
	if recording != "off" && recording != "audio" && recording != "video_composite" {
		return errs.InvalidRequest("recording_policy 无效")
	}
	if overflow != "" && overflow != "hangup" && overflow != "voicemail" && overflow != "queue" {
		return errs.InvalidRequest("overflow_policy 无效")
	}
	if after != "" && after != "hangup" && after != "voicemail" {
		return errs.InvalidRequest("after_hours_action 无效")
	}
	if strings.TrimSpace(hours) != "" && !json.Valid([]byte(hours)) {
		return errs.InvalidRequest("business_hours_json 不是有效 JSON")
	}
	return nil
}

func validateReferences(tx *gorm.DB, queueID, overflowID, ivrID string, skills []string) error {
	if overflowID != "" {
		if overflowID == queueID {
			return errs.InvalidRequest("队列不能溢出到自身")
		}
		var n int64
		if err := tx.Model(&models.Queue{}).Where("id = ?", overflowID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errs.InvalidRequest("overflow_queue_id 不存在")
		}
	}
	if ivrID != "" {
		var n int64
		if err := tx.Model(&models.IVRFlow{}).Where("id = ?", ivrID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return errs.InvalidRequest("ivr_flow_id 不存在")
		}
	}
	if len(skills) > 0 {
		var n int64
		if err := tx.Model(&models.Skill{}).Where("id IN ?", skills).Count(&n).Error; err != nil {
			return err
		}
		if n != int64(len(uniqueStrings(skills))) {
			return errs.InvalidRequest("skill_ids 包含不存在的技能")
		}
	}
	return nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v != "" {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				out = append(out, v)
			}
		}
	}
	return out
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
