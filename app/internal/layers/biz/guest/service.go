package guest

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-voip/internal/errs"
	"open-voip/internal/layers/biz/auth"
	"open-voip/internal/ports"
	"open-voip/internal/ports/dto"
	"open-voip/internal/store/models"
)

// SessionDTO 管理员签发的入会会话。
type SessionDTO struct {
	Token     string    `json:"token"`
	GuestURL  string    `json:"guest_url"` // 相对路径，根地址由调用方决定
	ExpiresAt time.Time `json:"expires_at"`
}

// JoinResult Demo 入队结果。
type JoinResult struct {
	CallID     string    `json:"call_id"`
	Token      string    `json:"token"`
	LegID      string    `json:"leg_id"`
	State      string    `json:"state,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// Service 访客会话与入队。
type Service struct {
	db        *gorm.DB
	calls     ports.CallControlPort
	guestBase string
}

// NewService 创建访客服务。
func NewService(db *gorm.DB, calls ports.CallControlPort, guestBase string) *Service {
	return &Service{db: db, calls: calls, guestBase: guestBase}
}

// CreateSession 管理员/坐席签发入会 token。
func (s *Service) CreateSession(ctx context.Context, queueID string, ttlSec int, allowedMedia string) (SessionDTO, error) {
	if ttlSec <= 0 {
		ttlSec = 3600
	}
	if allowedMedia == "" {
		allowedMedia = "audio"
	}
	var q models.Queue
	if err := s.db.WithContext(ctx).First(&q, "id = ?", queueID).Error; err != nil {
		return SessionDTO{}, errs.NotFound("队列不存在")
	}
	token, err := auth.RandomToken(24)
	if err != nil {
		return SessionDTO{}, err
	}
	exp := time.Now().UTC().Add(time.Duration(ttlSec) * time.Second)
	row := models.GuestSession{
		ID:           uuid.New().String(),
		QueueID:      queueID,
		Token:        token,
		AllowedMedia: allowedMedia,
		ExpiresAt:    exp,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return SessionDTO{}, err
	}
	url := "/guest/?token=" + token
	if s.guestBase != "" {
		url = strings.TrimRight(s.guestBase, "/") + "/?token=" + token
	}
	return SessionDTO{
		Token:     token,
		GuestURL:  url,
		ExpiresAt: exp,
	}, nil
}

// Join Demo 直链：创建会话并 StartInbound。
func (s *Service) Join(ctx context.Context, queueID string, sessionType dto.SessionType, priority int) (JoinResult, error) {
	if sessionType == "" {
		sessionType = dto.SessionTypeAudio
	}
	var q models.Queue
	if err := s.db.WithContext(ctx).First(&q, "id = ?", queueID).Error; err != nil {
		return JoinResult{}, errs.NotFound("队列不存在")
	}
	if sessionType == dto.SessionTypeVideo && !q.VideoEnabled {
		return JoinResult{}, errs.Unprocessable("该队列不支持视频", errs.CodeAgentNotVideoCapable)
	}
	if !q.PriorityEnabled || priority < 1 {
		priority = 0
	} else if priority > 10 {
		priority = 10
	}
	token, err := auth.RandomToken(24)
	if err != nil {
		return JoinResult{}, err
	}
	exp := time.Now().UTC().Add(2 * time.Hour)
	gs := models.GuestSession{
		ID:           uuid.New().String(),
		QueueID:      queueID,
		Token:        token,
		AllowedMedia: string(sessionType),
		ExpiresAt:    exp,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&gs).Error; err != nil {
		return JoinResult{}, err
	}
	callID, err := s.calls.StartInbound(ctx, dto.InboundRequest{
		QueueID:        queueID,
		GuestSessionID: gs.ID,
		SessionType:    sessionType,
		Priority:       priority,
	})
	if err != nil {
		return JoinResult{}, err
	}
	if err := s.db.WithContext(ctx).Model(&models.GuestSession{}).Where("id = ?", gs.ID).Update("call_id", callID).Error; err != nil {
		return JoinResult{}, err
	}
	legID, state := s.customerLeg(ctx, callID)
	return JoinResult{CallID: callID, Token: token, LegID: legID, State: state, ExpiresAt: exp}, nil
}

// JoinByToken 使用已签发入会 token 入队。
func (s *Service) JoinByToken(ctx context.Context, token string, sessionType dto.SessionType) (JoinResult, error) {
	var gs models.GuestSession
	if err := s.db.WithContext(ctx).Where("token = ?", token).First(&gs).Error; err != nil {
		return JoinResult{}, errs.Unauthorized("访客令牌无效")
	}
	if time.Now().UTC().After(gs.ExpiresAt) {
		e := errs.Unauthorized("访客令牌已过期")
		e.Code = errs.CodeGuestTokenExpired
		return JoinResult{}, e
	}
	if gs.CallID != nil && *gs.CallID != "" {
		legID, state := s.customerLeg(ctx, *gs.CallID)
		return JoinResult{CallID: *gs.CallID, Token: token, LegID: legID, State: state, ExpiresAt: gs.ExpiresAt}, nil
	}
	if sessionType == "" {
		sessionType = dto.SessionType(gs.AllowedMedia)
	}
	prio := 0
	if gs.AllowedMedia == "vip" {
		prio = 10
	}
	callID, err := s.calls.StartInbound(ctx, dto.InboundRequest{
		QueueID:        gs.QueueID,
		GuestSessionID: gs.ID,
		SessionType:    sessionType,
		Priority:       prio,
	})
	if err != nil {
		return JoinResult{}, err
	}
	if err := s.BindCall(ctx, gs.ID, callID); err != nil {
		return JoinResult{}, err
	}
	legID, state := s.customerLeg(ctx, callID)
	return JoinResult{CallID: callID, Token: token, LegID: legID, State: state, ExpiresAt: gs.ExpiresAt}, nil
}

func (s *Service) customerLeg(ctx context.Context, callID string) (legID, state string) {
	if s.calls == nil {
		return "", ""
	}
	view, err := s.calls.GetCall(ctx, callID)
	if err != nil {
		return "", ""
	}
	for _, leg := range view.Legs {
		if leg.Role == dto.LegRoleCustomer {
			return leg.ID, view.State
		}
	}
	return "", view.State
}

// BindCall 将已有 guest session 绑定通话（token 入会后入队）。
func (s *Service) BindCall(ctx context.Context, sessionID, callID string) error {
	return s.db.WithContext(ctx).Model(&models.GuestSession{}).Where("id = ?", sessionID).Update("call_id", callID).Error
}
