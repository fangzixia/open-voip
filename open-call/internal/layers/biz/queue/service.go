package queue

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"uuid"

	"open-call/internal/errs"
	"open-call/internal/ports"
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

// UpdateInput 使用指针区分 PATCH 未提交的字段，并允许清空可空文本与 ID。
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

// Service 通过 Switch 维护队列配置；运行策略在 Switch 执行。
type Service struct {
	sw ports.SwitchAdminPort
	db *gorm.DB
}

func NewService(sw ports.SwitchAdminPort, db *gorm.DB) *Service { return &Service{sw: sw, db: db} }

// List 分页。
func (s *Service) List(ctx context.Context, page, pageSize int) (ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	rows, err := s.sw.ListQueueConfigs(ctx)
	if err != nil {
		return ListResult{}, err
	}
	total := int64(len(rows))
	start := (page - 1) * pageSize
	if start > len(rows) {
		start = len(rows)
	}
	end := start + pageSize
	if end > len(rows) {
		end = len(rows)
	}
	items := make([]DTO, 0, end-start)
	for _, r := range rows[start:end] {
		items = append(items, switchToDTO(r))
	}
	return ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// ListPublic 演示环境访客可见队列列表。
func (s *Service) ListPublic(ctx context.Context) ([]DTO, error) {
	rows, err := s.sw.ListQueueConfigs(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]DTO, 0, len(rows))
	for _, r := range rows {
		items = append(items, switchToDTO(r))
	}
	return items, nil
}

// Get 按 ID。
func (s *Service) Get(ctx context.Context, id string) (DTO, error) {
	row, err := s.sw.GetQueueConfig(ctx, id)
	if err != nil {
		return DTO{}, err
	}
	return switchToDTO(row), nil
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
	cfg := createToSwitch(in)
	cfg.ID = uuid.New().String()
	if err := s.validateReferences(ctx, cfg.ID, in.OverflowQueueID, in.IVRFlowID, in.SkillIDs); err != nil {
		return DTO{}, err
	}
	created, err := s.sw.CreateQueueConfig(ctx, cfg)
	if err != nil {
		return DTO{}, err
	}
	return switchToDTO(created), nil
}

// Update 部分更新。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (DTO, error) {
	current, err := s.sw.GetQueueConfig(ctx, id)
	if err != nil {
		return DTO{}, err
	}
	merged := current
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return DTO{}, errs.InvalidRequest("队列名称不能为空")
		}
		merged.Name = name
	}
	if in.MaxWaitSec != nil {
		if *in.MaxWaitSec <= 0 {
			return DTO{}, errs.InvalidRequest("max_wait_sec 必须大于 0")
		}
		merged.MaxWaitSec = *in.MaxWaitSec
	}
	if in.Strategy != nil {
		if *in.Strategy != "longest_idle" && *in.Strategy != "round_robin" {
			return DTO{}, errs.InvalidRequest("分配策略无效")
		}
		merged.DispatchStrategy = *in.Strategy
	}
	if in.Name != nil {
		merged.Name = strings.TrimSpace(*in.Name)
	}
	if in.VideoEnabled != nil {
		merged.VideoEnabled = *in.VideoEnabled
	}
	if in.MaxWaitSec != nil {
		merged.MaxWaitSec = *in.MaxWaitSec
	}
	if in.Strategy != nil {
		merged.DispatchStrategy = *in.Strategy
	}
	if in.OverflowPolicy != nil {
		merged.OverflowAction = strings.TrimSpace(*in.OverflowPolicy)
	}
	if in.RecordingPolicy != nil {
		merged.RecordingPolicy = strings.TrimSpace(*in.RecordingPolicy)
	}
	if in.WaitPrompt != nil {
		merged.WaitPrompt = *in.WaitPrompt
	}
	if in.AnnounceRecording != nil {
		merged.AnnounceRecording = *in.AnnounceRecording
	}
	if in.PriorityEnabled != nil {
		merged.PriorityEnabled = *in.PriorityEnabled
	}
	if in.BusinessHoursJSON != nil {
		merged.BusinessHoursJSON = *in.BusinessHoursJSON
	}
	if in.AfterHoursAction != nil {
		merged.AfterHoursAction = strings.TrimSpace(*in.AfterHoursAction)
	}
	if in.OverflowQueueID != nil {
		merged.OverflowQueueID = strings.TrimSpace(*in.OverflowQueueID)
	}
	if in.IVRFlowID != nil {
		merged.IVRFlowID = strings.TrimSpace(*in.IVRFlowID)
	}
	if in.ForceHangupOnCheckout != nil {
		merged.ForceHangupOnCheckout = *in.ForceHangupOnCheckout
	}
	if in.ListenAnnounce != nil {
		merged.ListenAnnounce = *in.ListenAnnounce
	}
	skills := merged.SkillIDs
	if in.SkillIDs != nil {
		skills = *in.SkillIDs
		merged.SkillIDs = skills
	}
	if err := validateQueueValues(merged.RecordingPolicy, merged.OverflowAction, merged.AfterHoursAction, merged.BusinessHoursJSON); err != nil {
		return DTO{}, err
	}
	if err := s.validateReferences(ctx, id, merged.OverflowQueueID, merged.IVRFlowID, skills); err != nil {
		return DTO{}, err
	}
	updated, err := s.sw.UpdateQueueConfig(ctx, id, merged)
	if err != nil {
		return DTO{}, err
	}
	return switchToDTO(updated), nil
}

// Delete 删除队列。
func (s *Service) Delete(ctx context.Context, id string) error {
	dids, err := s.sw.ListDIDConfigs(ctx)
	if err != nil {
		return err
	}
	for _, d := range dids {
		if d.TargetType == "queue" && d.TargetID == id {
			return errs.Conflict("队列仍被 DID 路由引用", "")
		}
	}
	queues, err := s.sw.ListQueueConfigs(ctx)
	if err != nil {
		return err
	}
	for _, q := range queues {
		if q.OverflowQueueID == id {
			return errs.Conflict("队列仍被其他队列的溢出策略引用", "")
		}
	}
	var refs int64
	if err := s.db.WithContext(ctx).Model(&models.GuestSession{}).Where("queue_id = ? AND expires_at > ?", id, time.Now().UTC()).Count(&refs).Error; err != nil {
		return err
	}
	if refs > 0 {
		return errs.Conflict("队列仍有有效访客会话", "")
	}
	return s.sw.DeleteQueueConfig(ctx, id)
}

// BindAgents 覆盖绑定坐席。
func (s *Service) BindAgents(ctx context.Context, queueID string, agentIDs []string) error {
	ids := uniqueStrings(agentIDs)
	if len(ids) > 0 {
		agents, err := s.sw.ListAgentConfigs(ctx)
		if err != nil {
			return err
		}
		set := map[string]bool{}
		for _, a := range agents {
			set[a.ID] = true
		}
		for _, id := range ids {
			if !set[id] {
				return errs.InvalidRequest("agent_ids 包含不存在的坐席")
			}
		}
	}
	_, err := s.sw.SetQueueAgents(ctx, queueID, ids)
	return err
}

// AgentIDs 队列已绑定坐席。
func (s *Service) AgentIDs(ctx context.Context, queueID string) ([]string, error) {
	q, err := s.sw.GetQueueConfig(ctx, queueID)
	if err != nil {
		return nil, err
	}
	if q.AgentIDs == nil {
		return []string{}, nil
	}
	return q.AgentIDs, nil
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
	if strings.TrimSpace(hours) != "" && hours != "always" && !json.Valid([]byte(hours)) {
		return errs.InvalidRequest("business_hours_json 不是有效 JSON")
	}
	return nil
}

func (s *Service) validateReferences(ctx context.Context, queueID, overflowID, ivrID string, skills []string) error {
	if overflowID != "" {
		if overflowID == queueID {
			return errs.InvalidRequest("队列不能溢出到自身")
		}
		if _, err := s.sw.GetQueueConfig(ctx, overflowID); err != nil {
			return errs.InvalidRequest("overflow_queue_id 不存在")
		}
	}
	if ivrID != "" {
		if _, err := s.sw.GetIVRFlow(ctx, ivrID); err != nil {
			return errs.InvalidRequest("ivr_flow_id 尚未发布到 Switch")
		}
	}
	if len(skills) > 0 {
		all, err := s.sw.ListSkillConfigs(ctx)
		if err != nil {
			return err
		}
		set := map[string]bool{}
		for _, sk := range all {
			set[sk.ID] = true
		}
		for _, id := range uniqueStrings(skills) {
			if !set[id] {
				return errs.InvalidRequest("skill_ids 包含不存在的技能")
			}
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
