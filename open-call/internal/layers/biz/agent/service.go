package agent

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-call/internal/errs"
	"open-call/internal/ports"
	"open-call/internal/store/models"
)

const (
	StateOffline = "offline"
	StateIdle    = "idle"
	StateBusy    = "busy"
	StateRinging = "ringing"
	StateOnCall  = "on_call"
	StateACW     = "acw"
)

// SessionDTO 签入会话对外表示。
type SessionDTO struct {
	AgentID    string   `json:"agent_id"`
	State      string   `json:"state"`
	BusyReason string   `json:"busy_reason,omitempty"`
	QueueIDs   []string `json:"queue_ids"`
}

// MeDTO 当前坐席资料。
type MeDTO struct {
	TerminalType string     `json:"terminal_type"`
	SIPUsername  string     `json:"sip_username,omitempty"`
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Role         string     `json:"role,omitempty"`
	Extension    string     `json:"extension"`
	VideoCapable bool       `json:"video_capable"`
	DisplayName  string     `json:"display_name,omitempty"`
	Session      SessionDTO `json:"session"`
}

// Service 坐席目录、签入状态与 AgentDirectoryPort。
type Service struct {
	db     *gorm.DB
	events ports.AgentEventPublisher
}

// NewService 创建坐席服务。
func NewService(db *gorm.DB, events ports.AgentEventPublisher) *Service {
	return &Service{db: db, events: events}
}

var _ ports.AgentDirectoryPort = (*Service)(nil)

// ByExtension 按分机查询。
func (s *Service) ByExtension(ctx context.Context, extension string) (ports.AgentInfo, error) {
	var ag models.Agent
	if err := s.db.WithContext(ctx).Where("extension = ?", extension).First(&ag).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.AgentInfo{
				TerminalType: ag.TerminalType, SIPUsername: ag.SIPUsername}, errs.NotFound("坐席不存在")
		}
		return ports.AgentInfo{
			TerminalType: ag.TerminalType, SIPUsername: ag.SIPUsername}, err
	}
	return s.info(ctx, ag)
}

// ByID 按 ID 查询。
func (s *Service) ByID(ctx context.Context, agentID string) (ports.AgentInfo, error) {
	var ag models.Agent
	if err := s.db.WithContext(ctx).First(&ag, "id = ?", agentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.AgentInfo{
				TerminalType: ag.TerminalType, SIPUsername: ag.SIPUsername}, errs.NotFound("坐席不存在")
		}
		return ports.AgentInfo{
			TerminalType: ag.TerminalType, SIPUsername: ag.SIPUsername}, err
	}
	return s.info(ctx, ag)
}

// SetState 乐观锁迁移坐席状态。
func (s *Service) SetState(ctx context.Context, agentID, fromState, toState, reason string) error {
	return s.SetCallState(ctx, "", agentID, fromState, toState, reason)
}

// SetCallState 按预期旧状态更新坐席会话并记录当前通话，避免并发状态覆盖。
func (s *Service) SetCallState(ctx context.Context, callID, agentID, fromState, toState, reason string) error {
	if toState == "" {
		return errs.InvalidRequest("目标状态不能为空")
	}
	now := time.Now().UTC()
	changed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sess models.AgentSession
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("agent_id = ?", agentID)
		if err := q.First(&sess).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if callID != "" && toState != "ringing" && toState != "on_call" {
					return nil
				}
				return errs.Conflict("坐席未签入", errs.CodeAgentNotIdle)
			}
			return err
		}
		if reason == "force-check-out" && toState == "offline" && (sess.State == StateRinging || sess.State == StateOnCall) {
			return tx.Model(&sess).Update("pending_checkout", true).Error
		}
		if callID != "" {
			acquiring := toState == StateRinging || toState == StateOnCall
			if !acquiring && sess.CurrentCallID != callID {
				return nil
			}
			if acquiring && sess.CurrentCallID != "" && sess.CurrentCallID != callID {
				return errs.Conflict("坐席正在处理另一通话", errs.CodeAgentBusy)
			}
			if acquiring && sess.CurrentCallID == callID && sess.State == toState {
				return nil
			}
			if acquiring {
				sess.CurrentCallID = callID
			} else {
				sess.CurrentCallID = ""
				if sess.PendingCheckout {
					toState = StateOffline
				}
			}
		}
		if fromState != "" && sess.State != fromState {
			return errs.Conflict("坐席状态已变更", errs.CodeAgentNotIdle)
		}
		from := sess.State
		if err := tx.Model(&sess).Updates(map[string]any{
			"state":           toState,
			"current_call_id": sess.CurrentCallID,
			"busy_reason":     reason,
			"updated_at":      now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.AgentStateLog{
			ID:        uuid.New().String(),
			AgentID:   agentID,
			FromState: from,
			ToState:   toState,
			Reason:    reason,
			CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err == nil && changed {
		s.publish(ctx, agentID, toState, reason)
	}
	return err
}

// Me 当前登录坐席。
func (s *Service) Me(ctx context.Context, agentID string) (MeDTO, error) {
	info, err := s.ByID(ctx, agentID)
	if err != nil {
		return MeDTO{}, err
	}
	sess := s.sessionDTO(ctx, agentID)
	var role string
	var u models.User
	if err := s.db.WithContext(ctx).First(&u, "id = ?", info.UserID).Error; err == nil {
		role = u.Role
	}
	return MeDTO{
		TerminalType: info.TerminalType, SIPUsername: info.SIPUsername,
		ID:           info.AgentID,
		UserID:       info.UserID,
		Role:         role,
		Extension:    info.Extension,
		VideoCapable: info.VideoCapable,
		DisplayName:  info.DisplayName,
		Session:      sess,
	}, nil
}

// CheckIn 验证队列绑定并创建坐席在线会话。
func (s *Service) CheckIn(ctx context.Context, agentID string, queueIDs []string) (SessionDTO, error) {
	if _, err := s.ByID(ctx, agentID); err != nil {
		return SessionDTO{}, err
	}
	if len(queueIDs) == 0 {
		if err := s.db.WithContext(ctx).Model(&models.QueueAgent{}).Where("agent_id = ?", agentID).Pluck("queue_id", &queueIDs).Error; err != nil {
			return SessionDTO{}, err
		}
	}
	if len(queueIDs) == 0 {
		return SessionDTO{}, errs.InvalidRequest("没有可签入的队列")
	}
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seen := map[string]bool{}
		for _, qid := range queueIDs {
			if seen[qid] {
				return errs.InvalidRequest("队列重复")
			}
			seen[qid] = true
			var count int64
			if err := tx.Model(&models.QueueAgent{}).Where("agent_id = ? AND queue_id = ?", agentID, qid).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return errs.Forbidden("未绑定该队列")
			}
		}
		var exist models.AgentSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("agent_id = ?", agentID).First(&exist).Error; err == nil {
			if exist.State == StateOnCall || exist.State == StateRinging {
				return errs.Conflict("通话或振铃中不能重复签入", errs.CodeAgentBusy)
			}
			if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentSessionQueue{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&exist).Updates(map[string]any{
				"state":           StateIdle,
				"busy_reason":     "",
				"checked_in_at":   now,
				"current_call_id": "", "pending_checkout": false,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&models.AgentSession{
				ID:          uuid.New().String(),
				AgentID:     agentID,
				State:       StateIdle,
				CheckedInAt: now,
				UpdatedAt:   now,
			}).Error; err != nil {
				return err
			}
		} else {
			return err
		}
		for _, qid := range queueIDs {
			if err := tx.Create(&models.AgentSessionQueue{AgentID: agentID, QueueID: qid}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&models.AgentStateLog{
			ID:        uuid.New().String(),
			AgentID:   agentID,
			FromState: StateOffline,
			ToState:   StateIdle,
			Reason:    "check-in",
			CreatedAt: now,
		}).Error
	})
	if err != nil {
		return SessionDTO{}, err
	}
	s.publish(ctx, agentID, StateIdle, "")
	return s.sessionDTO(ctx, agentID), nil
}

// CheckOut 在没有振铃或通话时结束坐席在线会话。
func (s *Service) CheckOut(ctx context.Context, agentID string) error {
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if sess.State == StateRinging || sess.State == StateOnCall {
		return errs.Conflict("通话中不能签出", errs.CodeAgentBusy)
	}
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentSessionQueue{}).Error; err != nil {
			return err
		}
		if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentSession{}).Error; err != nil {
			return err
		}
		return tx.Create(&models.AgentStateLog{
			ID:        uuid.New().String(),
			AgentID:   agentID,
			FromState: sess.State,
			ToState:   StateOffline,
			Reason:    "check-out",
			CreatedAt: now,
		}).Error
	})
	if err != nil {
		return err
	}
	s.publish(ctx, agentID, StateOffline, "")
	return nil
}

// UpdateState 坐席主动示闲/示忙/ACW。
func (s *Service) UpdateState(ctx context.Context, agentID, state, reason string) (SessionDTO, error) {
	switch state {
	case StateIdle, StateBusy, StateACW:
	default:
		return SessionDTO{}, errs.InvalidRequest("不允许设置该状态")
	}
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SessionDTO{}, errs.Conflict("请先签入", errs.CodeAgentNotIdle)
		}
		return SessionDTO{}, err
	}
	if sess.State == StateRinging || sess.State == StateOnCall {
		return SessionDTO{}, errs.Conflict("振铃或通话中不能切换该状态", errs.CodeAgentBusy)
	}
	if err := s.SetState(ctx, agentID, sess.State, state, reason); err != nil {
		return SessionDTO{}, err
	}
	return s.sessionDTO(ctx, agentID), nil
}

// DirectoryItem 坐席目录条目（转接/外呼选择）。
type DirectoryItem struct {
	ID           string `json:"id"`
	Extension    string `json:"extension"`
	DisplayName  string `json:"display_name"`
	VideoCapable bool   `json:"video_capable"`
	State        string `json:"state"`
}

// List 列出坐席目录。
func (s *Service) List(ctx context.Context) ([]DirectoryItem, error) {
	var rows []models.Agent
	if err := s.db.WithContext(ctx).Order("extension").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DirectoryItem, 0, len(rows))
	for _, ag := range rows {
		info, err := s.info(ctx, ag)
		if err != nil {
			continue
		}
		out = append(out, DirectoryItem{
			ID: info.AgentID, Extension: info.Extension, DisplayName: info.DisplayName,
			VideoCapable: info.VideoCapable, State: info.State,
		})
	}
	return out, nil
}

func (s *Service) info(ctx context.Context, ag models.Agent) (ports.AgentInfo, error) {
	var u models.User
	if err := s.db.WithContext(ctx).First(&u, "id = ?", ag.UserID).Error; err != nil {
		return ports.AgentInfo{}, err
	}
	if u.Disabled {
		return ports.AgentInfo{}, errs.Forbidden("用户已禁用")
	}
	state := StateOffline
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("agent_id = ?", ag.ID).First(&sess).Error; err == nil {
		state = sess.State
	}
	return ports.AgentInfo{
		TerminalType: ag.TerminalType, SIPUsername: ag.SIPUsername,
		AgentID:      ag.ID,
		UserID:       ag.UserID,
		Extension:    ag.Extension,
		VideoCapable: ag.VideoCapable,
		DisplayName:  u.DisplayName,
		State:        state,
	}, nil
}

func (s *Service) sessionDTO(ctx context.Context, agentID string) SessionDTO {
	out := SessionDTO{AgentID: agentID, State: StateOffline, QueueIDs: []string{}}
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&sess).Error; err != nil {
		return out
	}
	out.State = sess.State
	out.BusyReason = sess.BusyReason
	_ = s.db.WithContext(ctx).Model(&models.AgentSessionQueue{}).Where("agent_id = ?", agentID).Pluck("queue_id", &out.QueueIDs)
	if out.QueueIDs == nil {
		out.QueueIDs = []string{}
	}
	return out
}

func (s *Service) publish(ctx context.Context, agentID, state, reason string) {
	if s.events == nil {
		return
	}
	_ = s.events.PublishAgentEvent(ctx, ports.AgentEvent{
		Type:    "agent.state_changed",
		AgentID: agentID,
		Payload: map[string]any{
			"agent_id":    agentID,
			"state":       state,
			"busy_reason": reason,
		},
	})
}
