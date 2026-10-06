// 本文件负责服务配置读取与校验。
// Package config 读取并校验业务服务配置。
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
	OIDC        OIDCConfig        `yaml:"oidc"`
	Log         LogConfig         `yaml:"log"`
	TLS         TLSConfig         `yaml:"tls"`
	Webhook     WebhookConfig     `yaml:"webhook"`
	Public      PublicConfig      `yaml:"public"`
	Bootstrap   BootstrapConfig   `yaml:"bootstrap"`
	Integration IntegrationConfig `yaml:"integration"`
	Security    SecurityConfig    `yaml:"security"`
	// FFmpegPath FFmpeg 可执行文件；为空时启动时在 PATH 中查找 ffmpeg。
	FFmpegPath string    `yaml:"ffmpeg_path"`
	TTS        TTSConfig  `yaml:"tts"`
	Aibot      AibotConfig `yaml:"aibot"`
}

// AibotConfig 虚拟坐席 Worker（AI 呼入与语音通知 WebRTC 媒体）。
type AibotConfig struct {
	Enabled      bool                  `yaml:"enabled"`
	Username     string                `yaml:"username"`
	Password     string                `yaml:"password"`
	AgentID      string                `yaml:"agent_id"`
	QueueIDs     []string              `yaml:"queue_ids"`
	SystemPrompt string                `yaml:"system_prompt"`
	OpenAI       AibotOpenAIConfig     `yaml:"openai_realtime"`
	// WidebandWebRTC 为 true 时 Bot 与 Switch 优先协商 Opus 宽带（SIP 侧仍为 8 kHz G.711）。
	WidebandWebRTC bool `yaml:"wideband_webrtc"`
}

type AibotOpenAIConfig struct {
	// Provider 可选：qwen（通义/百炼 Realtime）、openai（官方 Realtime）；空则按 base_url 推断。
	Provider string `yaml:"provider"`
	BaseURL  string `yaml:"base_url"`
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
	Voice    string `yaml:"voice"`
}

// TTSConfig 控制 IVR 素材文本转语音（业务侧合成后上传至 Switch）。
type TTSConfig struct {
	Enabled      bool                      `yaml:"enabled"`
	Provider     string                    `yaml:"provider"`
	SampleRate   int                       `yaml:"sample_rate"`
	MaxTextChars int                       `yaml:"max_text_chars"`
	Aliyun       TTSAliyunConfig           `yaml:"aliyun"`
	Xunfei       TTSXunfeiConfig           `yaml:"xunfei"`
	OpenAI       TTSOpenAICompatibleConfig `yaml:"openai_compatible"`
}

type TTSAliyunConfig struct {
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
	AppKey          string `yaml:"app_key"`
	Voice           string `yaml:"voice"`
	Format          string `yaml:"format"`
	GatewayURL      string `yaml:"gateway_url"`
	MetaURL         string `yaml:"meta_url"`
}

type TTSXunfeiConfig struct {
	AppID     string `yaml:"app_id"`
	APIKey    string `yaml:"api_key"`
	APISecret string `yaml:"api_secret"`
	Voice     string `yaml:"voice"`
	Aue       string `yaml:"aue"`
	Host      string `yaml:"host"`
}

type TTSOpenAICompatibleConfig struct {
	BaseURL        string            `yaml:"base_url"`
	APIKey         string            `yaml:"api_key"`
	Model          string            `yaml:"model"`
	Voice          string            `yaml:"voice"`
	ResponseFormat string            `yaml:"response_format"`
	ExtraHeaders   map[string]string `yaml:"extra_headers"`
}

type IntegrationConfig struct {
	SwitchBaseURL string `yaml:"switch_base_url"`
}

type ServerConfig struct {
	Listen string `yaml:"listen"`
}
type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

// RecordingsConfig 仅包含业务保留策略，录音文件由 open-switch 管理。
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

type OIDCConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Issuer         string   `yaml:"issuer"`
	ClientID       string   `yaml:"client_id"`
	ClientSecret   string   `yaml:"client_secret"`
	RedirectURL    string   `yaml:"redirect_url"`
	FrontendURL    string   `yaml:"frontend_url"`
	GroupsClaim    string   `yaml:"groups_claim"`
	Scopes         []string `yaml:"scopes"`
	EmergencyAdmin string   `yaml:"emergency_admin"`
	EncryptionKey  string   `yaml:"encryption_key"`
}

type LogConfig struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	Dir        string `yaml:"dir"`
	MaxSizeMB  int64  `yaml:"max_size_mb"`
	MaxAgeDays int    `yaml:"max_age_days"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// Bootstrap 必须显式启用，生产环境不会自动创建预设用户。
type BootstrapConfig struct {
	Enabled       bool   `yaml:"enabled"`
	AdminPassword string `yaml:"admin_password"`
	AgentPassword string `yaml:"agent_password"`
}

type SecurityConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
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
	level := strings.ToLower(strings.TrimSpace(c.Log.Level))
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		problems = append(problems, "log.level 必须为 debug/info/warn/error 之一")
	}
	format := strings.ToLower(strings.TrimSpace(c.Log.Format))
	if format != "json" && format != "text" {
		problems = append(problems, "log.format 必须为 json 或 text")
	}
	if c.Log.MaxSizeMB < 0 {
		problems = append(problems, "log.max_size_mb 不能小于 0")
	}
	if c.Log.MaxAgeDays < 0 {
		problems = append(problems, "log.max_age_days 不能小于 0")
	}
	if c.TLS.Enabled && (strings.TrimSpace(c.TLS.CertFile) == "" || strings.TrimSpace(c.TLS.KeyFile) == "") {
		problems = append(problems, "tls.enabled 为 true 时必须设置 cert_file 与 key_file")
	}
	for _, origin := range c.Security.AllowedOrigins {
		if strings.TrimSpace(origin) == "*" {
			continue
		}
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
	if c.OIDC.Enabled {
		if c.OIDC.Issuer == "" || c.OIDC.ClientID == "" || c.OIDC.ClientSecret == "" || c.OIDC.RedirectURL == "" || c.OIDC.FrontendURL == "" || c.OIDC.GroupsClaim == "" || c.OIDC.EmergencyAdmin == "" || len(c.OIDC.EncryptionKey) < 32 {
			problems = append(problems, "oidc 配置缺少 issuer、客户端、回调、前端地址、组声明、应急管理员或加密密钥")
		}
		for _, raw := range []string{c.OIDC.Issuer, c.OIDC.RedirectURL, c.OIDC.FrontendURL} {
			u, err := url.Parse(raw)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				problems = append(problems, "OIDC 地址必须是完整 HTTP 或 HTTPS URL")
				break
			}
		}
	}
	if c.Aibot.Enabled {
		if strings.TrimSpace(c.Aibot.Username) == "" || strings.TrimSpace(c.Aibot.Password) == "" {
			problems = append(problems, "aibot.enabled 时必须配置 username 与 password")
		}
		if strings.TrimSpace(c.Aibot.AgentID) == "" {
			problems = append(problems, "aibot.enabled 时必须配置 agent_id")
		}
		if len(c.Aibot.QueueIDs) > 0 {
			if strings.TrimSpace(c.Aibot.OpenAI.BaseURL) == "" {
				problems = append(problems, "aibot 配置了 queue_ids 时必须配置 openai_realtime.base_url（兼容 Realtime 的网关地址，无默认值）")
			}
			if strings.TrimSpace(c.Aibot.OpenAI.APIKey) == "" || strings.TrimSpace(c.Aibot.OpenAI.Model) == "" {
				problems = append(problems, "aibot 配置了 queue_ids 时必须配置 openai_realtime.api_key 与 model")
			}
		}
	}
	if c.TTS.Enabled {
		provider := strings.ToLower(strings.TrimSpace(c.TTS.Provider))
		switch provider {
		case "aliyun":
			if strings.TrimSpace(c.TTS.Aliyun.AccessKeyID) == "" || strings.TrimSpace(c.TTS.Aliyun.AccessKeySecret) == "" || strings.TrimSpace(c.TTS.Aliyun.AppKey) == "" {
				problems = append(problems, "tts.enabled 且 provider=aliyun 时必须配置 aliyun.access_key_id、access_key_secret、app_key")
			}
		case "xunfei":
			if strings.TrimSpace(c.TTS.Xunfei.AppID) == "" || strings.TrimSpace(c.TTS.Xunfei.APIKey) == "" || strings.TrimSpace(c.TTS.Xunfei.APISecret) == "" {
				problems = append(problems, "tts.enabled 且 provider=xunfei 时必须配置 xunfei.app_id、api_key、api_secret")
			}
		case "openai_compatible":
			if strings.TrimSpace(c.TTS.OpenAI.BaseURL) == "" || strings.TrimSpace(c.TTS.OpenAI.APIKey) == "" || strings.TrimSpace(c.TTS.OpenAI.Model) == "" {
				problems = append(problems, "tts.enabled 且 provider=openai_compatible 时必须配置 openai_compatible.base_url、api_key、model")
			}
		default:
			problems = append(problems, "tts.provider 必须为 aliyun、xunfei 或 openai_compatible")
		}
		if c.TTS.SampleRate != 8000 && c.TTS.SampleRate != 16000 {
			problems = append(problems, "tts.sample_rate 必须为 8000 或 16000")
		}
		if c.TTS.MaxTextChars <= 0 {
			problems = append(problems, "tts.max_text_chars 必须大于 0")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("配置校验失败:\n- %s", strings.Join(problems, "\n- "))
	}
	return nil
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Log.Level) == "" {
		c.Log.Level = "info"
	}
	if strings.TrimSpace(c.Log.Format) == "" {
		c.Log.Format = "json"
	}
	if strings.TrimSpace(c.Log.Dir) == "" {
		c.Log.Dir = "logs/open-call"
	}
	if c.Log.MaxSizeMB <= 0 {
		c.Log.MaxSizeMB = 100
	}
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
	if len(c.Security.AllowedOrigins) == 0 {
		c.Security.AllowedOrigins = []string{"*"}
	}
	if c.TTS.SampleRate == 0 {
		c.TTS.SampleRate = 16000
	}
	if c.TTS.MaxTextChars == 0 {
		c.TTS.MaxTextChars = 2000
	}
	if strings.TrimSpace(c.TTS.Aliyun.Format) == "" {
		c.TTS.Aliyun.Format = "wav"
	}
	if strings.TrimSpace(c.TTS.Aliyun.Voice) == "" {
		c.TTS.Aliyun.Voice = "xiaoyun"
	}
	if strings.TrimSpace(c.TTS.Aliyun.GatewayURL) == "" {
		c.TTS.Aliyun.GatewayURL = "https://nls-gateway-cn-shanghai.aliyuncs.com"
	}
	if strings.TrimSpace(c.TTS.Aliyun.MetaURL) == "" {
		c.TTS.Aliyun.MetaURL = "https://nls-meta.cn-shanghai.aliyuncs.com"
	}
	if strings.TrimSpace(c.TTS.Xunfei.Aue) == "" {
		c.TTS.Xunfei.Aue = "lame"
	}
	if strings.TrimSpace(c.TTS.Xunfei.Voice) == "" {
		c.TTS.Xunfei.Voice = "xiaoyan"
	}
	if strings.TrimSpace(c.TTS.Xunfei.Host) == "" {
		c.TTS.Xunfei.Host = "tts-api.xfyun.cn"
	}
	if strings.TrimSpace(c.TTS.OpenAI.ResponseFormat) == "" {
		c.TTS.OpenAI.ResponseFormat = "mp3"
	}
	if strings.TrimSpace(c.Aibot.OpenAI.Model) == "" {
		c.Aibot.OpenAI.Model = "gpt-4o-realtime-preview"
	}
	if strings.TrimSpace(c.Aibot.OpenAI.Voice) == "" {
		c.Aibot.OpenAI.Voice = "alloy"
	}
}
