package config

import (
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

func TestLoadSplitDeployConfig(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", "config.example.split.yml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StaticServe {
		t.Fatal("split config must disable static_serve")
	}
	if !cfg.CORSActive() {
		t.Fatal("split config must define cors.allowed_origins")
	}
}

func TestValidateRejectsShortJWTKey(t *testing.T) {
	cfg := &Config{
		Server:     ServerConfig{Listen: ":8080", PublicURL: "https://x"},
		Database:   DatabaseConfig{DSN: "host=localhost"},
		Recordings: RecordingsConfig{Dir: "./data"},
		JWT:        JWTConfig{AccessTTLSec: 3600, RefreshTTLSec: 86400, SigningKey: "short"},
		Log:        LogConfig{Level: "info", Format: "json"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(os.TempDir(), "nonexistent-open-voip-config.yml"))
	if err == nil {
		t.Fatal("expected error")
	}
}
