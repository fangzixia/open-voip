package guest

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-call/internal/errs"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
	"open-call/internal/store/models"
)

// SessionDTO 管理员签发的入会会话。
type SessionDTO struct {
	Token     string    `json:"token"`
	GuestURL  string    `json:"guest_url"` // 相对路径，根地址由调用方决定
	ExpiresAt time.Time `json:"expires_at"`
}

// JoinResult 是访客入队后的通话结果。
type JoinResult struct {
	CallID    string    `json:"call_id"`
	Token     string    `json:"token"`
	LegID     string    `json:"leg_id"`
	State     string    `json:"state,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
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
func (s *Service) CreateSession(ctx context.Context, queueID string, ttlSec int, allowedMedia string, priority int) (SessionDTO, error) {
	if ttlSec <= 0 {
		ttlSec = 3600
	}
	if ttlSec > 86400 {
		return SessionDTO{}, errs.InvalidRequest("ttl_sec 不能超过 86400")
	}
	if allowedMedia == "" {
		allowedMedia = "audio"
	}
	if allowedMedia != "audio" && allowedMedia != "video" {
		return SessionDTO{}, errs.InvalidRequest("allowed_media 必须为 audio 或 video")
	}
	var q models.Queue
	if err := s.db.WithContext(ctx).First(&q, "id = ?", queueID).Error; err != nil {
		return SessionDTO{}, errs.NotFound("队列不存在")
	}
	if allowedMedia == "video" && !q.VideoEnabled {
		return SessionDTO{}, errs.Unprocessable("该队列不支持视频", errs.CodeAgentNotVideoCapable)
	}
	if !q.PriorityEnabled {
		priority = 0
	}
	if priority < 0 || priority > 10 {
		return SessionDTO{}, errs.InvalidRequest("priority 必须在 0-10 之间")
	}
	token, err := auth.RandomToken(24)
	if err != nil {
		return SessionDTO{}, err
	}
	exp := time.Now().UTC().Add(time.Duration(ttlSec) * time.Second)
	row := models.GuestSession{
		ID:           uuid.New().String(),
		QueueID:      queueID,
		Token:        auth.TokenDigest(token),
		AllowedMedia: allowedMedia,
		Priority:     priority,
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

// Join 仅供显式启用的演示直连模式创建会话并入队，优先级固定为零。
func (s *Service) Join(ctx context.Context, queueID string, sessionType dto.SessionType, _ int) (JoinResult, error) {
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
	priority := 0
	token, err := auth.RandomToken(24)
	if err != nil {
		return JoinResult{}, err
	}
	exp := time.Now().UTC().Add(2 * time.Hour)
	gs := models.GuestSession{
		ID:           uuid.New().String(),
		QueueID:      queueID,
		Token:        auth.TokenDigest(token),
		AllowedMedia: string(sessionType),
		Priority:     0,
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
	var callID string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token IN ?", []string{auth.TokenDigest(token), token}).First(&gs).Error; err != nil {
			return errs.Unauthorized("访客令牌无效")
		}
		if time.Now().UTC().After(gs.ExpiresAt) {
			e := errs.Unauthorized("访客令牌已过期")
			e.Code = errs.CodeGuestTokenExpired
			return e
		}
		if gs.CallID != nil && *gs.CallID != "" {
			callID = *gs.CallID
			return nil
		}
		if sessionType == "" {
			sessionType = dto.SessionType(gs.AllowedMedia)
		}
		if string(sessionType) != gs.AllowedMedia {
			return errs.Forbidden("请求的媒体类型不在令牌授权范围内")
		}
		id, err := s.calls.StartInbound(ctx, dto.InboundRequest{QueueID: gs.QueueID, GuestSessionID: gs.ID, SessionType: sessionType, Priority: gs.Priority})
		if err != nil {
			return err
		}
		callID = id
		now := time.Now().UTC()
		gs.CallID = &callID
		gs.ConsumedAt = &now
		return tx.Model(&models.GuestSession{}).Where("id = ?", gs.ID).Updates(map[string]any{"call_id": callID, "consumed_at": now, "token": auth.TokenDigest(token)}).Error
	})
	if err != nil {
		return JoinResult{}, err
	}
	legID, state := s.customerLeg(ctx, callID)
	return JoinResult{CallID: callID, Token: token, LegID: legID, State: state, ExpiresAt: gs.ExpiresAt}, nil
}

// Cleanup 删除已过期一天以上的访客会话。
func (s *Service) Cleanup(ctx context.Context) error {
	return s.db.WithContext(ctx).Where("expires_at < ?", time.Now().UTC().Add(-24*time.Hour)).Delete(&models.GuestSession{}).Error
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
