// 本文件负责外部身份登录流程与本地账号绑定。
package oidcauth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"open-call/internal/config"
	"open-call/internal/errs"
	"open-call/internal/layers/biz/authz"
	"open-call/internal/store/models"
)

// flow 保存一次授权跳转的校验参数；状态值仅以哈希形式落库。
type flow struct {
	StateHash  string    `gorm:"column:state_hash"`
	Nonce      string    `gorm:"column:nonce"`
	Verifier   string    `gorm:"column:verifier"`
	ReturnPath string    `gorm:"column:return_path"`
	ExpiresAt  time.Time `gorm:"column:expires_at"`
}

func (flow) TableName() string { return "oc_oidc_flows" }

// ticket 是登录回调完成后供前端一次性兑换的短期凭据。
type ticket struct {
	TicketHash           string    `gorm:"column:ticket_hash"`
	UserID               string    `gorm:"column:user_id"`
	ProviderRefreshToken string    `gorm:"column:provider_refresh_token"`
	ProviderSubject      string    `gorm:"column:provider_subject"`
	ExpiresAt            time.Time `gorm:"column:expires_at"`
}

func (ticket) TableName() string { return "oc_oidc_tickets" }

type Service struct {
	cfg      config.OIDCConfig
	db       *gorm.DB
	roles    *authz.Service
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
	key      [32]byte
	mu       sync.Mutex
}

func NewService(ctx context.Context, cfg config.OIDCConfig, db *gorm.DB, roles *authz.Service) (*Service, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	s := &Service{cfg: cfg, db: db, roles: roles, key: sha256.Sum256([]byte(cfg.EncryptionKey))}
	return s, nil
}

func (s *Service) ensureProvider(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provider != nil {
		return nil
	}
	provider, err := oidc.NewProvider(ctx, s.cfg.Issuer)
	if err != nil {
		return fmt.Errorf("OIDC discovery: %w", err)
	}
	scopes := s.cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email", "offline_access"}
	}
	s.oauth = oauth2.Config{ClientID: s.cfg.ClientID, ClientSecret: s.cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: s.cfg.RedirectURL, Scopes: scopes}
	s.verifier = provider.Verifier(&oidc.Config{ClientID: s.cfg.ClientID})
	s.provider = provider
	return nil
}

// Start 只允许跳转到受信任的前端路径，并保存 state、nonce 与 PKCE 验证值。
func (s *Service) Start(ctx context.Context, returnPath string) (string, error) {
	if err := s.ensureProvider(ctx); err != nil {
		return "", err
	}
	if returnPath != "/admin/" && returnPath != "/agent/" {
		return "", errs.InvalidRequest("登录回跳路径无效")
	}
	state, err := random(32)
	if err != nil {
		return "", err
	}
	nonce, err := random(32)
	if err != nil {
		return "", err
	}
	verifier := oauth2.GenerateVerifier()
	row := flow{StateHash: digest(state), Nonce: nonce, Verifier: verifier, ReturnPath: returnPath, ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", err
	}
	return s.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier), oauth2.AccessTypeOffline), nil
}

// Callback 原子消费登录状态，校验外部身份与角色后签发短期兑换凭据。
func (s *Service) Callback(ctx context.Context, state, code string) (string, string, error) {
	if err := s.ensureProvider(ctx); err != nil {
		return "", "", err
	}
	var f flow
	if err := s.db.WithContext(ctx).Raw("DELETE FROM oc_oidc_flows WHERE state_hash = ? RETURNING *", digest(state)).Scan(&f).Error; err != nil {
		return "", "", err
	}
	if f.StateHash == "" || time.Now().UTC().After(f.ExpiresAt) || code == "" {
		return "", "", errs.Unauthorized("单点登录状态已失效")
	}
	tok, err := s.oauth.Exchange(ctx, code, oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return "", "", errs.Unauthorized("身份平台授权失败")
	}
	claims, err := s.claims(ctx, tok, f.Nonce)
	if err != nil {
		return "", "", err
	}
	roles, err := s.roles.RolesForGroups(ctx, claims.Groups)
	if err != nil {
		return "", "", err
	}
	if len(roles) == 0 {
		return "", "", errs.Forbidden("外部账号未映射到本项目角色")
	}
	username := claims.Username
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		username = claims.Subject
	}
	if len(username) > 64 {
		return "", "", errs.InvalidRequest("外部用户名超过长度限制")
	}
	u, err := s.roles.NewOIDCUser(ctx, s.cfg.Issuer, claims.Subject, username, claims.Email, claims.Name, roles)
	if err != nil {
		return "", "", err
	}
	if u.Username == s.cfg.EmergencyAdmin {
		return "", "", errs.Forbidden("应急管理员不可通过外部身份登录")
	}
	if u.Disabled {
		return "", "", errs.Unauthorized("账号已禁用")
	}
	current, err := s.roles.UserRoles(ctx, u.ID)
	if err != nil {
		return "", "", err
	}
	if !same(current, roles) {
		if err := s.roles.SetUserRoles(ctx, u.ID, roles); err != nil {
			return "", "", err
		}
	}
	encrypted := ""
	if tok.RefreshToken != "" {
		encrypted, err = s.encrypt(tok.RefreshToken)
		if err != nil {
			return "", "", err
		}
	}
	value, err := random(32)
	if err != nil {
		return "", "", err
	}
	row := ticket{TicketHash: digest(value), UserID: u.ID, ProviderRefreshToken: encrypted, ProviderSubject: claims.Subject, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", "", err
	}
	return value, f.ReturnPath, nil
}

// Exchange 消费一次性凭据，并返回本地用户及后续刷新所需的信息。
func (s *Service) Exchange(ctx context.Context, value string) (models.User, string, string, error) {
	var row ticket
	if err := s.db.WithContext(ctx).Raw("DELETE FROM oc_oidc_tickets WHERE ticket_hash = ? RETURNING *", digest(value)).Scan(&row).Error; err != nil {
		return models.User{}, "", "", err
	}
	if row.TicketHash == "" || time.Now().UTC().After(row.ExpiresAt) {
		return models.User{}, "", "", errs.Unauthorized("单点登录票据无效")
	}
	var u models.User
	if err := s.db.WithContext(ctx).First(&u, "id = ?", row.UserID).Error; err != nil {
		return models.User{}, "", "", err
	}
	return u, row.ProviderRefreshToken, row.ProviderSubject, nil
}

// Refresh 使用加密保存的提供方令牌刷新登录，并重新读取当前角色映射。
func (s *Service) Refresh(ctx context.Context, encrypted, subject string) (string, []string, error) {
	if err := s.ensureProvider(ctx); err != nil {
		return "", nil, err
	}
	plain, err := s.decrypt(encrypted)
	if err != nil {
		return "", nil, err
	}
	tok, err := s.oauth.TokenSource(ctx, &oauth2.Token{RefreshToken: plain}).Token()
	if err != nil {
		return "", nil, err
	}
	claims, err := s.claims(ctx, tok, "")
	if err != nil {
		return "", nil, err
	}
	if claims.Subject != subject {
		return "", nil, errors.New("OIDC subject changed")
	}
	roles, err := s.roles.RolesForGroups(ctx, claims.Groups)
	if err != nil {
		return "", nil, err
	}
	refresh := tok.RefreshToken
	if refresh == "" {
		refresh = plain
	}
	enc, err := s.encrypt(refresh)
	return enc, roles, err
}

type identityClaims struct {
	Subject  string
	Username string
	Email    string
	Name     string
	Groups   []string
}

func (s *Service) claims(ctx context.Context, tok *oauth2.Token, nonce string) (identityClaims, error) {
	var raw map[string]json.RawMessage
	if encoded, ok := tok.Extra("id_token").(string); ok && encoded != "" {
		id, err := s.verifier.Verify(ctx, encoded)
		if err != nil {
			return identityClaims{}, errs.Unauthorized("身份令牌无效")
		}
		if nonce != "" && id.Nonce != nonce {
			return identityClaims{}, errs.Unauthorized("身份令牌 nonce 无效")
		}
		if err := id.Claims(&raw); err != nil {
			return identityClaims{}, err
		}
	} else {
		return identityClaims{}, errs.Unauthorized("身份平台未返回身份令牌")
	}
	var tokenSubject string
	_ = json.Unmarshal(raw["sub"], &tokenSubject)
	if tokenSubject == "" {
		return identityClaims{}, errs.Unauthorized("身份令牌缺少 sub")
	}
	if len(raw[s.cfg.GroupsClaim]) == 0 {
		info, err := s.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok))
		if err == nil {
			if info.Subject != tokenSubject {
				return identityClaims{}, errs.Unauthorized("用户信息与身份令牌不一致")
			}
			var extra map[string]json.RawMessage
			if info.Claims(&extra) == nil {
				raw[s.cfg.GroupsClaim] = extra[s.cfg.GroupsClaim]
			}
		}
	}
	var out identityClaims
	_ = json.Unmarshal(raw["sub"], &out.Subject)
	_ = json.Unmarshal(raw["preferred_username"], &out.Username)
	_ = json.Unmarshal(raw["email"], &out.Email)
	_ = json.Unmarshal(raw["name"], &out.Name)
	if err := json.Unmarshal(raw[s.cfg.GroupsClaim], &out.Groups); err != nil || len(out.Groups) == 0 {
		return identityClaims{}, errs.Forbidden("身份平台未提供授权组")
	}
	return out, nil
}

// encrypt 在持久化提供方刷新令牌前加密，避免明文进入业务库。
func (s *Service) encrypt(plain string) (string, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(plain), nil)), nil
}

// decrypt 校验并解密此前保存的提供方刷新令牌。
func (s *Service) decrypt(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < a.NonceSize() {
		return "", errors.New("invalid encrypted token")
	}
	plain, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], nil)
	return string(plain), err
}
func random(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), err
}
func digest(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func same(a, b []string) bool {
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

func (s *Service) Redirect(ticket, returnPath string) string {
	u, _ := url.Parse(strings.TrimRight(s.cfg.FrontendURL, "/"))
	u.Path = returnPath
	u.Fragment = "sso_ticket=" + url.QueryEscape(ticket)
	return u.String()
}
