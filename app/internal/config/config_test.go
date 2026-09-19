package config

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExampleConfig(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", "config.example.yml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Listen == "" {
		t.Fatal("expected server.listen")
	}
}

func TestValidateRejectsShortJWTKey(t *testing.T) {
	cfg := &Config{
		Server:     ServerConfig{Listen: ":8080"},
		Database:   DatabaseConfig{DSN: "host=localhost"},
		Recordings: RecordingsConfig{Dir: "./data"},
		JWT:        JWTConfig{AccessTTLSec: 3600, RefreshTTLSec: 86400, SigningKey: "short"},
		Log:        LogConfig{Level: "info", Format: "json"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSIPEnabledRequiresExternalIPAndTrunk(t *testing.T) {
	cfg := validBase()
	cfg.SIP.Enabled = true
	cfg.applyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected sip.external_ip error")
	}
	cfg.SIP.ExternalIP = "127.0.0.1"
	cfg.SIP.Trunks = []SIPTrunkConfig{{ID: "t1", Host: "sip.example.com", Register: true}}
	cfg.applyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected register username/password error")
	}
	cfg.SIP.Trunks[0].Username = "u"
	cfg.SIP.Trunks[0].Password = "p"
	cfg.SIP.Trunks[0].AllowedCIDRs = []string{"10.0.0.1/32"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLocalRegistrarDefaultsPassword(t *testing.T) {
	cfg := validBase()
	cfg.SIP.LocalRegistrar = true
	cfg.applyDefaults()
	if cfg.SIP.RegistrarPassword != "changeme" {
		t.Fatalf("got %q", cfg.SIP.RegistrarPassword)
	}
}

func TestSessionExpiresBelowMinSE(t *testing.T) {
	cfg := validBase()
	cfg.SIP.Enabled = true
	cfg.SIP.ExternalIP = "127.0.0.1"
	cfg.SIP.SessionExpiresSec = 30
	cfg.SIP.Trunks = []SIPTrunkConfig{{ID: "t1", Host: "sip.example.com"}}
	cfg.applyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected session_expires_sec < 90 error")
	}
}

func TestSIPRejectsUnknownCodecAndTLSWithoutCert(t *testing.T) {
	cfg := validBase()
	cfg.SIP.Enabled = true
	cfg.SIP.ExternalIP = "1.2.3.4"
	cfg.SIP.Transport = "tls"
	cfg.SIP.Trunks = []SIPTrunkConfig{{ID: "t1", Host: "sip.example.com", Codecs: []string{"G729"}}}
	cfg.applyDefaults()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func validBase() *Config {
	return &Config{
		Server:     ServerConfig{Listen: ":8080"},
		Database:   DatabaseConfig{DSN: "host=localhost"},
		Recordings: RecordingsConfig{Dir: "./data"},
		JWT:        JWTConfig{AccessTTLSec: 3600, RefreshTTLSec: 86400, SigningKey: "0123456789abcdef"},
		Log:        LogConfig{Level: "info", Format: "json"},
		ICE:        ICEConfig{UDPPortMin: 10000, UDPPortMax: 20000},
	}
}

func TestNormalizeDial(t *testing.T) {
	tr := SIPTrunkConfig{StripPrefix: "0", Prefix: "86"}
	if got := tr.NormalizeDial("+8613800138000"); got != "8613800138000" {
		t.Fatalf("got %s", got)
	}
	if got := tr.NormalizeDial("013800138000"); got != "8613800138000" {
		t.Fatalf("strip+prefix got %s", got)
	}
	if got := tr.NormalizeDial("008613800138000"); got != "8613800138000" {
		t.Fatalf("00 prefix got %s", got)
	}
}

func TestParseIPNet(t *testing.T) {
	n, err := ParseIPNet("10.0.0.1")
	if err != nil || !n.Contains(net.ParseIP("10.0.0.1")) {
		t.Fatalf("single ip %v %v", n, err)
	}
	n, err = ParseIPNet("192.168.1.0/24")
	if err != nil || !n.Contains(net.ParseIP("192.168.1.20")) {
		t.Fatalf("cidr %v %v", n, err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(os.TempDir(), "nonexistent-open-voip-config.yml"))
	if err == nil {
		t.Fatal("expected error")
	}
}
