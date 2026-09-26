// 本文件验证服务配置读取与校验的关键行为。
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExampleConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "deploy", "config.example.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Listen == "" || cfg.Security.LoginRequestsPerMin == 0 {
		t.Fatal("expected defaults")
	}
	if cfg.Log.Dir != "logs/open-call" || cfg.Log.MaxSizeMB <= 0 || cfg.Log.MaxAgeDays != 0 {
		t.Fatalf("unexpected log defaults: %+v", cfg.Log)
	}
}

func TestValidateRejectsWeakSecrets(t *testing.T) {
	cfg := validBase()
	cfg.JWT.SigningKey = "short"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected JWT validation error")
	}
	cfg = validBase()
	cfg.Bootstrap.Enabled = true
	cfg.Bootstrap.AdminPassword = "changeme"
	cfg.Bootstrap.AgentPassword = "StrongAgent123"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected bootstrap validation error")
	}
}

func TestRejectsUnknownControlPlaneFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yml")
	raw := "server:\n  listen: :8080\nsip:\n  enabled: true\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected removed sip field to fail")
	}
}

func validBase() *Config {
	return &Config{
		Server: ServerConfig{Listen: ":8080"}, Database: DatabaseConfig{DSN: "host=localhost"},
		JWT:         JWTConfig{AccessTTLSec: 3600, RefreshTTLSec: 86400, SigningKey: "0123456789abcdef0123456789abcdef"},
		Log:         LogConfig{Level: "info", Format: "json"},
		Integration: IntegrationConfig{Secret: "change_me_integration", SwitchBaseURL: "http://127.0.0.1:8082"},
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(os.TempDir(), "nonexistent-open-call-config.yml")); err == nil {
		t.Fatal("expected error")
	}
}

func TestOIDCAllowsHTTPAddresses(t *testing.T) {
	cfg := validBase()
	cfg.OIDC = OIDCConfig{
		Enabled: true, Issuer: "http://id.internal/realm", ClientID: "client",
		ClientSecret: "secret", RedirectURL: "http://cc.internal/api/v1/auth/oidc/callback",
		FrontendURL: "http://ui.internal", GroupsClaim: "groups",
		EmergencyAdmin: "admin", EncryptionKey: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("HTTP OIDC 地址应可配置: %v", err)
	}
}
