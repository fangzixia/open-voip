package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-call/internal/config"
	"open-call/internal/errs"
	"open-call/internal/layers/biz/authz"
	"open-call/internal/store/models"
)

// Principal 是完成数据库复核后的认证主体，供 HTTP 与 WebSocket 使用。
type Principal struct {
	UserID             string    `json:"user_id,omitempty"`
	Role               string    `json:"role,omitempty"`
	Permissions        []string  `json:"permissions,omitempty"`
	AgentID            string    `json:"agent_id,omitempty"`
	GuestID            string    `json:"guest_id,omitempty"`
	GuestCallID        string    `json:"guest_call_id,omitempty"`
	JTI                string    `json:"jti,omitempty"`
	SessionID          string    `json:"session_id,omitempty"`
	MustChangePassword bool      `json:"must_change_password,omitempty"`
	ExpiresAt          time.Time `json:"expires_at,omitempty"`
}

// IsGuest 返回当前主体是否为访客。
func (p Principal) IsGuest() bool { return p.Role == "guest" }
func (p Principal) Has(code string) bool {
	for _, v := range p.Permissions {
		if v == code {
			return true
		}
	}
	return false
}

// TokenPair 是登录或刷新成功后的令牌响应。
type TokenPair struct {
	AccessToken        string `json:"access_token"`
	RefreshToken       string `json:"refresh_token"`
	ExpiresIn          int    `json:"expires_in"`
	TokenType          string `json:"token_type"`
	MustChangePassword bool   `json:"must_change_password,omitempty"`
}

// SessionDTO 是可供用户查看和撤销的登录会话摘要。
type SessionDTO struct {
	ID         string     `json:"id"`
	UserAgent  string     `json:"user_agent,omitempty"`
	RemoteIP   string     `json:"remote_ip,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// SessionMeta 记录登录客户端信息，不参与授权判断。
type SessionMeta struct {
	UserAgent string
	RemoteIP  string
}

type jwtClaims struct {
	jwt.RegisteredClaims
	Role        string `json:"role"`
	AgentID     string `json:"agent_id,omitempty"`
	Typ         string `json:"typ"`
	SessionID   string `json:"sid"`
	AuthVersion int64  `json:"ver"`
}

// Service 负责账号登录、令牌轮换、会话撤销与访客令牌认证。
type Service struct {
	db             *gorm.DB
	cfg            config.JWTConfig
	authorization  *authz.Service
	emergencyAdmin string
	oidcEnabled    bool
	oidcRefresh    func(context.Context, string, string) (string, []string, error)
}

// NewService 创建认证服务。
func NewService(db *gorm.DB, cfg config.JWTConfig) *Service {
	return &Service{db: db, cfg: cfg, authorization: authz.NewService(db)}
}
func (s *Service) ConfigureOIDC(enabled bool, emergencyAdmin string) {
	s.oidcEnabled = enabled
	s.emergencyAdmin = emergencyAdmin
}
func (s *Service) ValidateEmergencyAdmin(ctx context.Context) error {
	if !s.oidcEnabled {
		return nil
	}
	var u models.User
	if err := s.db.WithContext(ctx).Where("username = ? AND disabled = false", s.emergencyAdmin).First(&u).Error; err != nil {
		return fmt.Errorf("OIDC 应急管理员不存在或已禁用: %w", err)
	}
	var n int64
	if err := s.db.WithContext(ctx).Table("oc_user_roles").Where("user_id = ? AND role_id = 'admin'", u.ID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("OIDC 应急管理员必须拥有内置 admin 角色")
	}
	return nil
}
func (s *Service) SetOIDCRefresher(fn func(context.Context, string, string) (string, []string, error)) {
	s.oidcRefresh = fn
}

func (s *Service) IssueOIDC(ctx context.Context, user models.User, encryptedRefresh, subject string, meta SessionMeta) (TokenPair, error) {
	if user.Disabled {
		return TokenPair{}, errs.Unauthorized("账号已禁用")
	}
	agentID, err := s.agentID(ctx, user)
	if err != nil {
		return TokenPair{}, err
	}
	now := time.Now().UTC()
	sid := uuid.NewString()
	pair, jti, exp, err := s.buildPair(user, agentID, sid, now)
	if err != nil {
		return TokenPair{}, err
	}
	row := models.AuthSession{ID: sid, UserID: user.ID, RefreshJTI: jti, UserAgent: truncate(meta.UserAgent, 256), RemoteIP: truncate(meta.RemoteIP, 64), CreatedAt: now, LastSeenAt: now, ExpiresAt: exp, Provider: "oidc", ProviderRefreshToken: encryptedRefresh, ProviderSubject: subject}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return TokenPair{}, err
	}
	return pair, nil
}

// Login 校验凭证并创建一条可独立撤销的登录会话。
func (s *Service) Login(ctx context.Context, username, password string, meta ...SessionMeta) (TokenPair, error) {
	if s.oidcEnabled && strings.TrimSpace(username) != s.emergencyAdmin {
		return TokenPair{}, errs.Unauthorized("请使用单点登录")
	}
	var user models.User
	if err := s.db.WithContext(ctx).Where("username = ?", strings.TrimSpace(username)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return TokenPair{}, errs.Unauthorized("账号或密码错误")
		}
		return TokenPair{}, err
	}
	if user.Disabled {
		return TokenPair{}, errs.Unauthorized("账号已禁用")
	}
	if s.oidcEnabled {
		var n int64
		if err := s.db.WithContext(ctx).Table("oc_user_roles ur").Where("ur.user_id = ? AND ur.role_id = ?", user.ID, "admin").Count(&n).Error; err != nil {
			return TokenPair{}, err
		}
		if n == 0 {
			return TokenPair{}, errs.Unauthorized("应急账号必须具有管理员角色")
		}
	}
	if err := VerifyPassword(password, user.PasswordHash); err != nil {
		return TokenPair{}, err
	}
	agentID, err := s.agentID(ctx, user)
	if err != nil {
		return TokenPair{}, err
	}
	var m SessionMeta
	if len(meta) > 0 {
		m = meta[0]
	}
	now := time.Now().UTC()
	sid := uuid.New().String()
	pair, refreshJTI, refreshExp, err := s.buildPair(user, agentID, sid, now)
	if err != nil {
		return TokenPair{}, err
	}
	row := models.AuthSession{ID: sid, UserID: user.ID, RefreshJTI: refreshJTI,
		UserAgent: truncate(m.UserAgent, 256), RemoteIP: truncate(m.RemoteIP, 64),
		CreatedAt: now, LastSeenAt: now, ExpiresAt: refreshExp}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return TokenPair{}, err
	}
	return pair, nil
}

// Refresh 在同一登录会话内原子轮换刷新令牌。
func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	claims, err := s.parse(refreshToken, "refresh")
	if err != nil || claims.SessionID == "" {
		return TokenPair{}, errs.Unauthorized("令牌无效")
	}
	var pair TokenPair
	var rejection error
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 锁定会话后再校验旧 JTI，确保同一刷新令牌只能成功轮换一次。
		if revoked(tx, claims.ID) {
			return errs.Unauthorized("令牌已撤销")
		}
		var session models.AuthSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&session, "id = ?", claims.SessionID).Error; err != nil {
			return errs.Unauthorized("登录会话已失效")
		}
		now := time.Now().UTC()
		if session.RevokedAt != nil || now.After(session.ExpiresAt) || session.RefreshJTI != claims.ID {
			return errs.Unauthorized("登录会话已失效")
		}
		var user models.User
		if err := tx.First(&user, "id = ?", claims.Subject).Error; err != nil || user.Disabled || user.AuthVersion != claims.AuthVersion {
			return errs.Unauthorized("账号状态已变化，请重新登录")
		}
		if s.oidcEnabled && session.Provider != "oidc" && user.Username != s.emergencyAdmin {
			return errs.Unauthorized("请使用单点登录")
		}
		var refreshedProviderToken string
		if session.Provider == "oidc" {
			var roleIDs []string
			var refreshErr error
			if s.oidcRefresh == nil || session.ProviderRefreshToken == "" {
				refreshErr = errs.Unauthorized("请重新完成单点登录")
			} else {
				refreshedProviderToken, roleIDs, refreshErr = s.oidcRefresh(ctx, session.ProviderRefreshToken, session.ProviderSubject)
			}
			if refreshErr == nil && len(roleIDs) > 0 {
				current, err := authz.NewService(tx).UserRoles(ctx, user.ID)
				if err != nil {
					return err
				}
				if !sameStrings(current, roleIDs) {
					if err := tx.Where("user_id = ?", user.ID).Delete(&authz.UserRole{}).Error; err != nil {
						return err
					}
					for _, roleID := range roleIDs {
						if err := tx.Create(&authz.UserRole{UserID: user.ID, RoleID: roleID}).Error; err != nil {
							return err
						}
					}
					user.AuthVersion++
					if err := tx.Model(&user).Update("auth_version", user.AuthVersion).Error; err != nil {
						return err
					}
					if err := tx.Model(&models.AuthSession{}).Where("user_id = ? AND id <> ? AND revoked_at IS NULL", user.ID, session.ID).Update("revoked_at", now).Error; err != nil {
						return err
					}
				}
			}
			if refreshErr != nil || len(roleIDs) == 0 {
				if err := tx.Model(&models.User{}).Where("id = ?", user.ID).Update("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
					return err
				}
				if err := revokeUserSessions(tx, user.ID); err != nil {
					return err
				}
				rejection = errs.Unauthorized("外部授权已失效，请重新登录")
				return nil
			}
		}
		agentID, err := agentIDWithDB(tx, user)
		if err != nil {
			return err
		}
		if err := revokeWithDB(tx, claims.ID, claims.ExpiresAt.Time); err != nil {
			return err
		}
		var newJTI string
		var refreshExp time.Time
		pair, newJTI, refreshExp, err = s.buildPair(user, agentID, session.ID, now)
		if err != nil {
			return err
		}
		updates := map[string]any{"refresh_jti": newJTI, "last_seen_at": now, "expires_at": refreshExp}
		if session.Provider == "oidc" {
			updates["provider_refresh_token"] = refreshedProviderToken
		}
		return tx.Model(&session).Updates(updates).Error
	})
	if rejection != nil {
		return TokenPair{}, rejection
	}
	return pair, err
}
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, v := range a {
		m[v]++
	}
	for _, v := range b {
		m[v]--
	}
	for _, n := range m {
		if n != 0 {
			return false
		}
	}
	return true
}

// Logout 撤销当前访问令牌及其登录会话。
func (s *Service) Logout(ctx context.Context, accessJTI string, accessExp time.Time, sessionID, refreshToken string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := revokeWithDB(tx, accessJTI, accessExp); err != nil {
			return err
		}
		if refreshToken != "" {
			if claims, err := s.parse(refreshToken, "refresh"); err == nil {
				if err := revokeWithDB(tx, claims.ID, claims.ExpiresAt.Time); err != nil {
					return err
				}
			}
		}
		if sessionID != "" {
			now := time.Now().UTC()
			return tx.Model(&models.AuthSession{}).Where("id = ?", sessionID).Update("revoked_at", now).Error
		}
		return nil
	})
}

// Authenticate 解析令牌，并用数据库中的账号版本及会话状态进行复核。
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, errs.Unauthorized("缺少令牌")
	}
	claims, err := s.parse(token, "access")
	if err == nil {
		if revoked(s.db.WithContext(ctx), claims.ID) {
			return Principal{}, errs.Unauthorized("令牌已撤销")
		}
		var user models.User
		if err := s.db.WithContext(ctx).First(&user, "id = ?", claims.Subject).Error; err != nil || user.Disabled || user.AuthVersion != claims.AuthVersion {
			return Principal{}, errs.Unauthorized("账号状态已变化，请重新登录")
		}
		var session models.AuthSession
		if claims.SessionID == "" || s.db.WithContext(ctx).Where("id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", claims.SessionID, user.ID, time.Now().UTC()).First(&session).Error != nil {
			return Principal{}, errs.Unauthorized("登录会话已失效")
		}
		if s.oidcEnabled && session.Provider != "oidc" && user.Username != s.emergencyAdmin {
			return Principal{}, errs.Unauthorized("请使用单点登录")
		}
		agentID, err := s.agentID(ctx, user)
		if err != nil {
			return Principal{}, err
		}
		permissions, err := s.authorization.Permissions(ctx, user.ID)
		if err != nil {
			return Principal{}, err
		}
		return Principal{UserID: user.ID, Role: user.Role, Permissions: permissions, AgentID: agentID, JTI: claims.ID,
			SessionID: session.ID, MustChangePassword: user.MustChangePassword, ExpiresAt: claims.ExpiresAt.Time}, nil
	}
	return s.authenticateGuest(ctx, token)
}

// ChangePassword 校验原密码、写入新密码并撤销该账号全部现有会话。
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			return errs.NotFound("用户不存在")
		}
		if err := VerifyPassword(currentPassword, user.PasswordHash); err != nil {
			return err
		}
		hash, err := HashPassword(newPassword)
		if err != nil {
			return err
		}
		if err := tx.Model(&user).Updates(map[string]any{"password_hash": hash, "must_change_password": false,
			"auth_version": gorm.Expr("auth_version + 1"), "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return revokeUserSessions(tx, userID)
	})
}

// ListSessions 列出用户尚未过期的登录会话。
func (s *Service) ListSessions(ctx context.Context, userID string) ([]SessionDTO, error) {
	var rows []models.AuthSession
	if err := s.db.WithContext(ctx).Where("user_id = ? AND expires_at > ?", userID, time.Now().UTC()).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SessionDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, SessionDTO{ID: row.ID, UserAgent: row.UserAgent, RemoteIP: row.RemoteIP,
			CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, LastSeenAt: row.LastSeenAt, RevokedAt: row.RevokedAt})
	}
	return out, nil
}

// RevokeSession 撤销用户指定的登录会话。
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	now := time.Now().UTC()
	res := s.db.WithContext(ctx).Model(&models.AuthSession{}).Where("id = ? AND user_id = ?", sessionID, userID).Update("revoked_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("登录会话不存在")
	}
	return nil
}

// RevokeAllSessions 递增认证版本并撤销用户的全部登录会话。
func (s *Service) RevokeAllSessions(ctx context.Context, userID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.User{}).Where("id = ?", userID).Update("auth_version", gorm.Expr("auth_version + 1"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.NotFound("用户不存在")
		}
		return revokeUserSessions(tx, userID)
	})
}

// CleanupExpired 删除已过期的撤销记录和登录会话。
func (s *Service) CleanupExpired(ctx context.Context) error {
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Exec("DELETE FROM oc_oidc_flows WHERE expires_at < ?", now).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Exec("DELETE FROM oc_oidc_tickets WHERE expires_at < ?", now).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&models.JWTRevocation{}).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&models.AuthSession{}).Error
}

// buildPair 为同一登录会话生成访问令牌和可轮换的刷新令牌。
func (s *Service) buildPair(user models.User, agentID, sessionID string, now time.Time) (TokenPair, string, time.Time, error) {
	access, _, _, err := s.sign(user, agentID, sessionID, "access", now, time.Duration(s.cfg.AccessTTLSec)*time.Second)
	if err != nil {
		return TokenPair{}, "", time.Time{}, err
	}
	refresh, refreshJTI, refreshExp, err := s.sign(user, agentID, sessionID, "refresh", now, time.Duration(s.cfg.RefreshTTLSec)*time.Second)
	if err != nil {
		return TokenPair{}, "", time.Time{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: s.cfg.AccessTTLSec,
		TokenType: "Bearer", MustChangePassword: user.MustChangePassword}, refreshJTI, refreshExp, nil
}

func (s *Service) sign(user models.User, agentID, sessionID, typ string, now time.Time, ttl time.Duration) (string, string, time.Time, error) {
	jti := uuid.New().String()
	exp := now.Add(ttl)
	claims := jwtClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: user.ID, ID: jti,
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp)}, Role: user.Role,
		AgentID: agentID, Typ: typ, SessionID: sessionID, AuthVersion: user.AuthVersion}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.SigningKey))
	return token, jti, exp, err
}

// parse 校验 JWT 的签名算法和令牌类型，阻止访问令牌与刷新令牌混用。
func (s *Service) parse(token, wantTyp string) (*jwtClaims, error) {
	parsed, err := jwt.ParseWithClaims(token, &jwtClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errs.Unauthorized("令牌无效")
		}
		return []byte(s.cfg.SigningKey), nil
	}, jwt.WithExpirationRequired())
	if err != nil || !parsed.Valid {
		return nil, errs.Unauthorized("令牌无效")
	}
	claims, ok := parsed.Claims.(*jwtClaims)
	if !ok || claims.Typ != wantTyp || claims.Subject == "" || claims.ID == "" {
		return nil, errs.Unauthorized("令牌无效")
	}
	return claims, nil
}

// authenticateGuest 从访客会话令牌构造受限身份。
func (s *Service) authenticateGuest(ctx context.Context, token string) (Principal, error) {
	hash := sha256.Sum256([]byte(token))
	encoded := hex.EncodeToString(hash[:])
	var gs models.GuestSession
	if err := s.db.WithContext(ctx).Where("token IN ?", []string{encoded, token}).First(&gs).Error; err != nil {
		return Principal{}, errs.Unauthorized("未认证或令牌失效")
	}
	if time.Now().UTC().After(gs.ExpiresAt) {
		return Principal{}, guestExpired()
	}
	callID := ""
	if gs.CallID != nil {
		callID = *gs.CallID
	}
	return Principal{Role: "guest", GuestID: gs.ID, GuestCallID: callID, ExpiresAt: gs.ExpiresAt}, nil
}

func guestExpired() *errs.APIError {
	e := errs.Unauthorized("访客令牌已过期")
	e.Code = errs.CodeGuestTokenExpired
	return e
}

func (s *Service) agentID(ctx context.Context, user models.User) (string, error) {
	return agentIDWithDB(s.db.WithContext(ctx), user)
}

func agentIDWithDB(db *gorm.DB, user models.User) (string, error) {
	var ag models.Agent
	err := db.Where("user_id = ?", user.ID).First(&ag).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return ag.ID, nil
}

func revoked(db *gorm.DB, jti string) bool {
	if jti == "" {
		return false
	}
	var n int64
	return db.Model(&models.JWTRevocation{}).Where("jti = ?", jti).Count(&n).Error == nil && n > 0
}

func revokeWithDB(db *gorm.DB, jti string, exp time.Time) error {
	if jti == "" {
		return nil
	}
	row := models.JWTRevocation{JTI: jti, ExpiresAt: exp, RevokedAt: time.Now().UTC()}
	return db.Where("jti = ?", jti).FirstOrCreate(&row).Error
}

func revokeUserSessions(db *gorm.DB, userID string) error {
	now := time.Now().UTC()
	return db.Model(&models.AuthSession{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", now).Error
}

func truncate(value string, n int) string {
	value = strings.TrimSpace(value)
	if len(value) > n {
		return value[:n]
	}
	return value
}
