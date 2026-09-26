// 本文件验证service integration的关键行为。
package oidcauth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"open-call/internal/config"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/layers/biz/authz"
	"open-call/internal/layers/biz/oidcauth"
	"open-call/internal/store"
	"open-call/internal/store/migrate"
	"open-call/internal/store/models"
)

func TestOIDCLoginTicketAndGroupRefresh(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("OPEN_VOIP_TEST_DSN not set")
	}
	db, err := store.Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	groups := []string{"test-agents"}
	nonce := ""
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks", "userinfo_endpoint": issuer + "/userinfo", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/jwks":
			e := big.NewInt(int64(key.PublicKey.E)).Bytes()
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(e)}}})
		case "/token":
			_ = r.ParseForm()
			claims := jwt.MapClaims{"iss": issuer, "aud": "open-voip-test", "sub": "user-sub-1", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "preferred_username": "oidc-user", "name": "OIDC User", "groups": groups}
			if r.Form.Get("grant_type") == "authorization_code" {
				claims["nonce"] = nonce
			}
			signed := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			signed.Header["kid"] = "test-key"
			raw, err := signed.SignedString(key)
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "provider-access", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "provider-refresh", "id_token": raw})
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": "user-sub-1", "groups": groups})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	rollback := errors.New("rollback")
	err = db.Transaction(func(tx *gorm.DB) error {
		ctx := context.Background()
		roles := authz.NewService(tx)
		if err := roles.SetMapping(ctx, "test-agents", []string{"agent"}); err != nil {
			return err
		}
		if err := roles.SetMapping(ctx, "test-supervisors", []string{"supervisor"}); err != nil {
			return err
		}
		cfg := config.OIDCConfig{Enabled: true, Issuer: issuer, ClientID: "open-voip-test", ClientSecret: "secret", RedirectURL: issuer + "/callback", FrontendURL: "https://ui.test", GroupsClaim: "groups", EncryptionKey: "long-test-encryption-key-over-32-chars"}
		svc, err := oidcauth.NewService(ctx, cfg, tx, roles)
		if err != nil {
			return err
		}
		start, err := svc.Start(ctx, "/agent/")
		if err != nil {
			return err
		}
		u, err := url.Parse(start)
		if err != nil {
			return err
		}
		state := u.Query().Get("state")
		nonce = u.Query().Get("nonce")
		if state == "" || nonce == "" || u.Query().Get("code_challenge") == "" {
			t.Fatal("OIDC state, nonce or PKCE challenge missing")
		}
		ticket, path, err := svc.Callback(ctx, state, "test-code")
		if err != nil {
			return err
		}
		if path != "/agent/" || ticket == "" {
			t.Fatal("callback did not return one-time ticket")
		}
		if _, _, err := svc.Callback(ctx, state, "test-code"); err == nil {
			t.Fatal("state replay succeeded")
		}
		user, encrypted, subject, err := svc.Exchange(ctx, ticket)
		if err != nil {
			return err
		}
		if user.Username != "oidc-user" || encrypted == "" || subject != "user-sub-1" {
			t.Fatalf("ticket payload invalid: %s %s", user.Username, subject)
		}
		if _, _, _, err := svc.Exchange(ctx, ticket); err == nil {
			t.Fatal("ticket replay succeeded")
		}
		jwtCfg := config.JWTConfig{AccessTTLSec: 3600, RefreshTTLSec: 86400, SigningKey: "test-jwt-signing-key-at-least-32-chars"}
		authSvc := auth.NewService(tx, jwtCfg)
		authSvc.SetOIDCRefresher(svc.Refresh)
		passwordHash, err := auth.HashPassword("StrongRescuePassword123!")
		if err != nil {
			return err
		}
		rescue := models.User{ID: uuid.NewString(), Username: "rescue-" + uuid.NewString()[:8], PasswordHash: passwordHash, Role: "admin", AuthVersion: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := tx.Create(&rescue).Error; err != nil {
			return err
		}
		if err := tx.Create(&authz.UserRole{UserID: rescue.ID, RoleID: "admin"}).Error; err != nil {
			return err
		}
		other := models.User{ID: uuid.NewString(), Username: "local-" + uuid.NewString()[:8], PasswordHash: passwordHash, Role: "agent", AuthVersion: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := tx.Create(&other).Error; err != nil {
			return err
		}
		if err := tx.Create(&authz.UserRole{UserID: other.ID, RoleID: "agent"}).Error; err != nil {
			return err
		}
		oldLocalPair, err := authSvc.Login(ctx, other.Username, "StrongRescuePassword123!")
		if err != nil {
			return err
		}
		authSvc.ConfigureOIDC(true, rescue.Username)
		if err := authSvc.ValidateEmergencyAdmin(ctx); err != nil {
			return err
		}
		if _, err := authSvc.Login(ctx, rescue.Username, "StrongRescuePassword123!"); err != nil {
			return err
		}
		if _, err := authSvc.Login(ctx, user.Username, "anything"); err == nil {
			t.Fatal("OIDC user could use local password")
		}
		if _, err := authSvc.Authenticate(ctx, oldLocalPair.AccessToken); err == nil {
			t.Fatal("pre-existing local session bypassed OIDC policy")
		}
		if _, err := authSvc.Refresh(ctx, oldLocalPair.RefreshToken); err == nil {
			t.Fatal("pre-existing local refresh bypassed OIDC policy")
		}
		pair, err := authSvc.IssueOIDC(ctx, user, encrypted, subject, auth.SessionMeta{})
		if err != nil {
			return err
		}
		pair, err = authSvc.Refresh(ctx, pair.RefreshToken)
		if err != nil {
			return err
		}
		oldAccess := pair.AccessToken
		groups = []string{"test-supervisors"}
		pair, err = authSvc.Refresh(ctx, pair.RefreshToken)
		if err != nil {
			return err
		}
		if _, err := authSvc.Authenticate(ctx, oldAccess); err == nil {
			t.Fatal("old access token retained after group change")
		}
		principal, err := authSvc.Authenticate(ctx, pair.AccessToken)
		if err != nil || !principal.Has("calls.listen") {
			t.Fatalf("refreshed group permissions missing: %v", err)
		}
		groups = []string{"removed-group"}
		if _, err := authSvc.Refresh(ctx, pair.RefreshToken); err == nil {
			t.Fatal("removed external group retained access")
		}
		if _, err := authSvc.Authenticate(ctx, pair.AccessToken); err == nil {
			t.Fatal("revoked session still authenticates")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}

func TestProviderOutageDoesNotBlockServiceConstruction(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	issuer := provider.URL
	provider.Close()
	svc, err := oidcauth.NewService(context.Background(), config.OIDCConfig{Enabled: true, Issuer: issuer, ClientID: "app", EncryptionKey: "test-key"}, nil, nil)
	if err != nil || svc == nil {
		t.Fatalf("OIDC service construction failed during outage: %v", err)
	}
	if _, err := svc.Start(context.Background(), "/admin/"); err == nil {
		t.Fatal("login start should fail when provider is unavailable")
	}
}
