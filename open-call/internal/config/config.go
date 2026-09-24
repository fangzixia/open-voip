// Package config loads and validates the open-call control-plane configuration.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Database    DatabaseConfig    `yaml:"database"`
	Recordings  RecordingsConfig  `yaml:"recordings"`
	JWT         JWTConfig         `yaml:"jwt"`
	Log         LogConfig         `yaml:"log"`
	TLS         TLSConfig         `yaml:"tls"`
	Webhook     WebhookConfig     `yaml:"webhook"`
	Public      PublicConfig      `yaml:"public"`
	Bootstrap   BootstrapConfig   `yaml:"bootstrap"`
	Integration IntegrationConfig `yaml:"integration"`
	Security    SecurityConfig    `yaml:"security"`
}

type IntegrationConfig struct {
	Secret        string `yaml:"secret"`
	SwitchBaseURL string `yaml:"switch_base_url"`
}

type ServerConfig struct {
	Listen string `yaml:"listen"`
}
type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

// RecordingsConfig contains business retention policy only. Files belong to open-switch.
type RecordingsConfig struct {
	RetainDays    int    `yaml:"retain_days"`
	NotifyMessage string `yaml:"notify_message"`
}

type WebhookConfig struct {
	MaxRetries int `yaml:"max_retries"`
	TimeoutSec int `yaml:"timeout_sec"`
}

type PublicConfig struct {
	GuestBaseURL    string `yaml:"guest_base_url"`
	AllowDirectJoin bool   `yaml:"allow_direct_join"`
}

type JWTConfig struct {
	AccessTTLSec  int    `yaml:"access_ttl_sec"`
	RefreshTTLSec int    `yaml:"refresh_ttl_sec"`
	SigningKey    string `yaml:"signing_key"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// Bootstrap is deliberately opt-in. Production does not silently create known users.
type BootstrapConfig struct {
	Enabled       bool   `yaml:"enabled"`
	AdminPassword string `yaml:"admin_password"`
	AgentPassword string `yaml:"agent_password"`
}

type SecurityConfig struct {
	AllowedOrigins      []string `yaml:"allowed_origins"`
	LoginRequestsPerMin int      `yaml:"login_requests_per_min"`
	GuestRequestsPerMin int      `yaml:"guest_requests_per_min"`
	AdminRequestsPerMin int      `yaml:"admin_requests_per_min"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置 %q: %w", path, err)
	}
	var cfg Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置 YAML: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	var problems []string
	if strings.TrimSpace(c.Server.Listen) == "" {
		problems = append(problems, "server.listen 不能为空")
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		problems = append(problems, "database.dsn 不能为空")
	}
	if c.JWT.AccessTTLSec <= 0 || c.JWT.RefreshTTLSec <= 0 || c.JWT.RefreshTTLSec <= c.JWT.AccessTTLSec {
		problems = append(problems, "JWT TTL 必须为正且 refresh_ttl_sec 大于 access_ttl_sec")
	}
	key := strings.TrimSpace(c.JWT.SigningKey)
	if len(key) < 32 || strings.EqualFold(key, "changeme") || strings.Contains(strings.ToLower(key), "replace-with") {
		problems = append(problems, "jwt.signing_key 至少 32 字符且不能使用示例值")
	}
	if strings.TrimSpace(c.Integration.SwitchBaseURL) == "" {
		problems = append(problems, "integration.switch_base_url 不能为空")
	} else if u, err := url.Parse(c.Integration.SwitchBaseURL); err != nil || u.Scheme == "" || u.Host == "" {
		problems = append(problems, "integration.switch_base_url 必须是完整 URL")
	}
	secret := strings.TrimSpace(c.Integration.Secret)
	if len(secret) < 16 || strings.EqualFold(secret, "changeme") {
		problems = append(problems, "integration.secret 长度至少 16 字符且不能使用默认值")
	}
	level := strings.ToLower(strings.TrimSpace(c.Log.Level))
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		problems = append(problems, "log.level 必须为 debug/info/warn/error 之一")
	}
	format := strings.ToLower(strings.TrimSpace(c.Log.Format))
	if format != "json" && format != "text" {
		problems = append(problems, "log.format 必须为 json 或 text")
	}
	if c.TLS.Enabled && (strings.TrimSpace(c.TLS.CertFile) == "" || strings.TrimSpace(c.TLS.KeyFile) == "") {
		problems = append(problems, "tls.enabled 为 true 时必须设置 cert_file 与 key_file")
	}
	for _, origin := range c.Security.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" {
			problems = append(problems, "security.allowed_origins 包含无效来源: "+origin)
		}
	}
	if c.Bootstrap.Enabled {
		for name, password := range map[string]string{"admin_password": c.Bootstrap.AdminPassword, "agent_password": c.Bootstrap.AgentPassword} {
			if len(password) < 12 || strings.EqualFold(password, "changeme") {
				problems = append(problems, "bootstrap."+name+" 至少 12 字符且不能为 changeme")
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("配置校验失败:\n- %s", strings.Join(problems, "\n- "))
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.Recordings.RetainDays <= 0 {
		c.Recordings.RetainDays = 90
	}
	if strings.TrimSpace(c.Recordings.NotifyMessage) == "" {
		c.Recordings.NotifyMessage = "本通话可能会被录音或录像，继续即表示您已知悉。"
	}
	if c.Webhook.MaxRetries <= 0 {
		c.Webhook.MaxRetries = 8
	}
	if c.Webhook.TimeoutSec <= 0 {
		c.Webhook.TimeoutSec = 5
	}
	if c.Security.LoginRequestsPerMin <= 0 {
		c.Security.LoginRequestsPerMin = 10
	}
	if c.Security.GuestRequestsPerMin <= 0 {
		c.Security.GuestRequestsPerMin = 30
	}
	if c.Security.AdminRequestsPerMin <= 0 {
		c.Security.AdminRequestsPerMin = 120
	}
}
