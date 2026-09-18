// Package config 负责从单一 YAML 文件加载运行时配置并校验，禁止使用环境变量覆盖。
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 表示 open-voip 进程的全部可配置项，与 deploy/config.example.yml 字段对齐。
type Config struct {
	// Server 监听与对外 URL。
	Server ServerConfig `yaml:"server"`
	// Database PostgreSQL 连接。
	Database DatabaseConfig `yaml:"database"`
	// Recordings 录音与 IVR 资产目录。
	Recordings RecordingsConfig `yaml:"recordings"`
	// JWT 访问令牌与刷新令牌参数。
	JWT JWTConfig `yaml:"jwt"`
	// Log 结构化日志级别与格式。
	Log LogConfig `yaml:"log"`
	// TLS 进程内 HTTPS；也可由前置反代终结。
	TLS TLSConfig `yaml:"tls"`
	// Static 前端构建产物目录。
	Static StaticConfig `yaml:"static"`
	// ICE WebRTC ICE 服务器列表。
	ICE ICEConfig `yaml:"ice"`
	// TURN 可选 coturn 凭证配置。
	TURN TURNConfig `yaml:"turn"`
	// StaticServe 为 true 时由 HTTP 服务静态资源。
	StaticServe bool `yaml:"static_serve"`
	// CORS 前端跨域访问 API 时的 Origin 白名单。
	CORS CORSConfig `yaml:"cors"`
}

// ServerConfig 定义 HTTP(S) 监听地址与对外 base URL。
type ServerConfig struct {
	// Listen 绑定地址，例如 0.0.0.0:8080。
	Listen string `yaml:"listen"`
	// PublicURL 坐席/访客浏览器使用的 HTTPS 根 URL（无尾斜杠）。
	PublicURL string `yaml:"public_url"`
}

// DatabaseConfig 定义 PostgreSQL DSN。
type DatabaseConfig struct {
	// DSN GORM postgres 驱动连接串。
	DSN string `yaml:"dsn"`
}

// RecordingsConfig 定义录音文件根目录。
type RecordingsConfig struct {
	// Dir 本地绝对或相对路径，需可写。
	Dir string `yaml:"dir"`
}

// JWTConfig 定义 JWT 签发参数。
type JWTConfig struct {
	// AccessTTLSec 访问令牌有效期（秒）。
	AccessTTLSec int `yaml:"access_ttl_sec"`
	// RefreshTTLSec 刷新令牌有效期（秒）。
	RefreshTTLSec int `yaml:"refresh_ttl_sec"`
	// SigningKey HS256 对称密钥，生产环境须足够长且保密。
	SigningKey string `yaml:"signing_key"`
}

// LogConfig 定义 slog 输出。
type LogConfig struct {
	// Level 日志级别：debug / info / warn / error。
	Level string `yaml:"level"`
	// Format json 或 text。
	Format string `yaml:"format"`
}

// TLSConfig 定义进程内 TLS 证书路径。
type TLSConfig struct {
	// Enabled 是否启用 TLS 监听。
	Enabled bool `yaml:"enabled"`
	// CertFile 服务端证书 PEM。
	CertFile string `yaml:"cert_file"`
	// KeyFile 服务端私钥 PEM。
	KeyFile string `yaml:"key_file"`
}

// StaticConfig 定义前端静态资源目录。
type StaticConfig struct {
	// Dir 例如 ./static，内含 agent/guest/admin 子目录。
	Dir string `yaml:"dir"`
}

// ICEConfig 定义 STUN 等 ICE URL。
type ICEConfig struct {
	// STUNURLs STUN 服务器列表，内网同网段可留空或指向本机。
	STUNURLs []string `yaml:"stun_urls"`
}

// TURNConfig 定义 TURN 中继（可选）。
type TURNConfig struct {
	// Enabled 是否向客户端签发 TURN 凭证。
	Enabled bool `yaml:"enabled"`
	// URLs turn: 或 turns: 地址列表。
	URLs []string `yaml:"urls"`
	// AuthSecret coturn static-auth-secret 或 HMAC 密钥。
	AuthSecret string `yaml:"auth_secret"`
}

// Load 从 path 读取并解析 YAML，随后 Validate。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置 %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置 YAML: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate 校验必填项与取值范围，启动前必须成功。
func (c *Config) Validate() error {
	var errs []string

	if strings.TrimSpace(c.Server.Listen) == "" {
		errs = append(errs, "server.listen 不能为空")
	}
	if strings.TrimSpace(c.Server.PublicURL) == "" {
		errs = append(errs, "server.public_url 不能为空")
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		errs = append(errs, "database.dsn 不能为空")
	}
	if strings.TrimSpace(c.Recordings.Dir) == "" {
		errs = append(errs, "recordings.dir 不能为空")
	}
	if c.JWT.AccessTTLSec <= 0 {
		errs = append(errs, "jwt.access_ttl_sec 必须大于 0")
	}
	if c.JWT.RefreshTTLSec <= 0 {
		errs = append(errs, "jwt.refresh_ttl_sec 必须大于 0")
	}
	if len(strings.TrimSpace(c.JWT.SigningKey)) < 16 {
		errs = append(errs, "jwt.signing_key 长度至少 16 字符")
	}

	level := strings.ToLower(strings.TrimSpace(c.Log.Level))
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		errs = append(errs, "log.level 必须为 debug/info/warn/error 之一")
	}
	format := strings.ToLower(strings.TrimSpace(c.Log.Format))
	if format != "json" && format != "text" {
		errs = append(errs, "log.format 必须为 json 或 text")
	}

	if c.TLS.Enabled {
		if strings.TrimSpace(c.TLS.CertFile) == "" || strings.TrimSpace(c.TLS.KeyFile) == "" {
			errs = append(errs, "tls.enabled 为 true 时必须设置 cert_file 与 key_file")
		}
	}

	if c.TURN.Enabled {
		if len(c.TURN.URLs) == 0 {
			errs = append(errs, "turn.enabled 为 true 时 turn.urls 不能为空")
		}
		if strings.TrimSpace(c.TURN.AuthSecret) == "" {
			errs = append(errs, "turn.enabled 为 true 时 turn.auth_secret 不能为空")
		}
	}

	if !c.StaticServe && !c.CORSActive() {
		errs = append(errs, "static_serve 为 false（前端独立部署）时必须设置 cors.allowed_origins")
	}

	if len(errs) > 0 {
		return fmt.Errorf("配置校验失败:\n- %s", strings.Join(errs, "\n- "))
	}
	return nil
}
