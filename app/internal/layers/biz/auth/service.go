package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
	"github.com/google/uuid"

	"open-voip/internal/config"
	"open-voip/internal/errs"
	"open-voip/internal/store/models"
)

// Principal 已认证主体，供 HTTP/WS 使用。
type Principal struct {
	// UserID 登录用户 ID，访客为空。
	UserID string
	// Role admin / supervisor / agent / guest。
	Role string
	// AgentID 坐席 ID，非坐席为空。
	AgentID string
	// GuestID 访客会话 ID。
	GuestID string
	// GuestCallID 访客当前通话。
	GuestCallID string
	// JTI access token jti。
	JTI string
	// ExpiresAt 令牌过期时间。
	ExpiresAt time.Time
}

// IsGuest 是否访客。
func (p Principal) IsGuest() bool {
	return p.Role == "guest"
}

// TokenPair 登录/刷新响应。
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type jwtClaims struct {
	jwt.RegisteredClaims
	Role    string `json:"role"`
	AgentID string `json:"agent_id,omitempty"`
	Typ     string `json:"typ"`
}

// Service JWT 登录、刷新、撤销与令牌解析。
type Service struct {
	db  *gorm.DB
	cfg config.JWTConfig
}

// NewService 创建认证服务。
func NewService(db *gorm.DB, cfg config.JWTConfig) *Service {
	return &Service{db: db, cfg: cfg}
}

// Login 校验用户名密码并签发令牌。
func (s *Service) Login(ctx context.Context, username, password string) (TokenPair, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return TokenPair{}, errs.Unauthorized("账号或密码错误")
		}
		return TokenPair{}, err
	}
	if user.Disabled {
		return TokenPair{}, errs.Unauthorized("账号已禁用")
	}
	if err := VerifyPassword(password, user.PasswordHash); err != nil {
		return TokenPair{}, err
	}
	agentID := ""
	if user.Role == "agent" || user.Role == "supervisor" {
		var ag models.Agent
		if err := s.db.WithContext(ctx).Where("user_id = ?", user.ID).First(&ag).Error; err == nil {
			agentID = ag.ID
		}
	}
	return s.issuePair(user.ID, user.Role, agentID)
}

// Refresh 使用 refresh token 换发新令牌并撤销旧 refresh。
func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	claims, err := s.parse(refreshToken, "refresh")
	if err != nil {
		return TokenPair{}, err
	}
	if s.revoked(ctx, claims.ID) {
		return TokenPair{}, errs.Unauthorized("令牌已撤销")
	}
	if err := s.revoke(ctx, claims.ID, claims.ExpiresAt.Time); err != nil {
		return TokenPair{}, err
	}
	return s.issuePair(claims.Subject, claims.Role, claims.AgentID)
}

// Logout 撤销当前 access jti，可选同时撤销 refresh。
func (s *Service) Logout(ctx context.Context, accessJTI string, accessExp time.Time, refreshToken string) error {
	if accessJTI != "" {
		if err := s.revoke(ctx, accessJTI, accessExp); err != nil {
			return err
		}
	}
	if refreshToken == "" {
		return nil
	}
	claims, err := s.parse(refreshToken, "refresh")
	if err != nil {
		return nil
	}
	return s.revoke(ctx, claims.ID, claims.ExpiresAt.Time)
}

// Authenticate 解析 JWT access 或访客 token。
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, errs.Unauthorized("缺少令牌")
	}
	claims, err := s.parse(token, "access")
	if err == nil {
		if s.revoked(ctx, claims.ID) {
			return Principal{}, errs.Unauthorized("令牌已撤销")
		}
		return Principal{
			UserID:    claims.Subject,
			Role:      claims.Role,
			AgentID:   claims.AgentID,
			JTI:       claims.ID,
			ExpiresAt: claims.ExpiresAt.Time,
		}, nil
	}
	var gs models.GuestSession
	if dbErr := s.db.WithContext(ctx).Where("token = ?", token).First(&gs).Error; dbErr != nil {
		return Principal{}, errs.Unauthorized("未认证或令牌失效")
	}
	if time.Now().UTC().After(gs.ExpiresAt) {
		return Principal{}, guestExpired()
	}
	callID := ""
	if gs.CallID != nil {
		callID = *gs.CallID
	}
	return Principal{
		Role:        "guest",
		GuestID:     gs.ID,
		GuestCallID: callID,
		ExpiresAt:   gs.ExpiresAt,
	}, nil
}

func guestExpired() *errs.APIError {
	e := errs.Unauthorized("访客令牌已过期")
	e.Code = errs.CodeGuestTokenExpired
	return e
}

func (s *Service) issuePair(userID, role, agentID string) (TokenPair, error) {
	now := time.Now().UTC()
	access, err := s.sign(userID, role, agentID, "access", now, time.Duration(s.cfg.AccessTTLSec)*time.Second)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.sign(userID, role, agentID, "refresh", now, time.Duration(s.cfg.RefreshTTLSec)*time.Second)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    s.cfg.AccessTTLSec,
		TokenType:    "Bearer",
	}, nil
}

func (s *Service) sign(userID, role, agentID, typ string, now time.Time, ttl time.Duration) (string, error) {
	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Role:    role,
		AgentID: agentID,
		Typ:     typ,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(s.cfg.SigningKey))
}

func (s *Service) parse(token, wantTyp string) (*jwtClaims, error) {
	parsed, err := jwt.ParseWithClaims(token, &jwtClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errs.Unauthorized("令牌无效")
		}
		return []byte(s.cfg.SigningKey), nil
	})
	if err != nil || !parsed.Valid {
		return nil, errs.Unauthorized("令牌无效")
	}
	claims, ok := parsed.Claims.(*jwtClaims)
	if !ok || claims.Typ != wantTyp {
		return nil, errs.Unauthorized("令牌无效")
	}
	return claims, nil
}

func (s *Service) revoked(ctx context.Context, jti string) bool {
	var n int64
	s.db.WithContext(ctx).Model(&models.JWTRevocation{}).Where("jti = ?", jti).Count(&n)
	return n > 0
}

func (s *Service) revoke(ctx context.Context, jti string, exp time.Time) error {
	if jti == "" {
		return nil
	}
	row := models.JWTRevocation{
		JTI:       jti,
		ExpiresAt: exp,
		RevokedAt: time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Where("jti = ?", jti).FirstOrCreate(&row).Error
}
