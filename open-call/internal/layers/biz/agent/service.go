package agent

import (
	"context"
	"errors"

	"gorm.io/gorm"

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
	db *gorm.DB
}

// NewService 创建坐席服务。
func NewService(db *gorm.DB, events ports.AgentEventPublisher) *Service {
	_ = events
	return &Service{db: db}
}

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

// Me 当前登录坐席。
func (s *Service) Me(ctx context.Context, agentID string) (MeDTO, error) {
	info, err := s.ByID(ctx, agentID)
	if err != nil {
		return MeDTO{}, err
	}
	sess := SessionDTO{AgentID: agentID, State: StateOffline, QueueIDs: []string{}}
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
	return ports.AgentInfo{
		TerminalType: ag.TerminalType, SIPUsername: ag.SIPUsername,
		AgentID:      ag.ID,
		UserID:       ag.UserID,
		Extension:    ag.Extension,
		VideoCapable: ag.VideoCapable,
		DisplayName:  u.DisplayName,
		State:        StateOffline,
	}, nil
}
