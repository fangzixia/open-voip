// Package config 负责从单一 YAML 文件加载运行时配置并校验，禁止使用环境变量覆盖。
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 表示 open-voip 进程的全部可配置项，与 deploy/config.example.yml 字段对齐。
type Config struct {
	// Server HTTP 监听。
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
	// ICE WebRTC ICE 服务器列表。
	ICE ICEConfig `yaml:"ice"`
	// TURN 可选 coturn 凭证配置。
	TURN TURNConfig `yaml:"turn"`
	// SIP 可选 PSTN 中继；未启用则不监听 5060。
	SIP SIPConfig `yaml:"sip"`
	// Webhook 同步投递重试参数。
	Webhook WebhookConfig `yaml:"webhook"`
	// Public 入会链接等对外 URL。
	Public PublicConfig `yaml:"public"`
	// Bootstrap 空库种子账号密码（仅 users 表为空时使用）。
	Bootstrap BootstrapConfig `yaml:"bootstrap"`
}

// ServerConfig 定义 HTTP(S) 监听地址。
type ServerConfig struct {
	// Listen 绑定地址，例如 0.0.0.0:8080。
	Listen string `yaml:"listen"`
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
	// RetainDays 保留天数，purge API 按此删除；0 表示不自动过期。
	RetainDays int `yaml:"retain_days"`
	// NotifyMessage 录音告知文案（REC-03）。
	NotifyMessage string `yaml:"notify_message"`
}

// WebhookConfig Webhook 投递。
type WebhookConfig struct {
	// MaxRetries 同步重试次数（含首次）。
	MaxRetries int `yaml:"max_retries"`
	// TimeoutSec 单次 HTTP 超时秒。
	TimeoutSec int `yaml:"timeout_sec"`
}

// PublicConfig 入会链接与 TURN 展示用的内网根 URL。
type PublicConfig struct {
	// GuestBaseURL 访客页根地址，例如 https://cc.internal/guest。
	GuestBaseURL string `yaml:"guest_base_url"`
}

// SIPConfig PSTN/SIP 中继。
type SIPConfig struct {
	// Enabled 是否启动 SIP UA。
	Enabled bool `yaml:"enabled"`
	// Listen 本机 SIP UDP 监听，例如 0.0.0.0:5060。
	Listen string `yaml:"listen"`
	// UserAgent User-Agent 头与默认 URI 用户名。
	UserAgent string `yaml:"user_agent"`
	// LocalDomain From/PAI 的 host；空则用 external_ip。
	LocalDomain string `yaml:"local_domain"`
	// ExternalIP NAT 通告地址（Via/Contact/SDP c=）。本机演示可用 127.0.0.1 或 0.0.0.0。
	ExternalIP string `yaml:"external_ip"`
	// RTPPortMin SIP RTP 端口下界。
	RTPPortMin uint16 `yaml:"rtp_port_min"`
	// RTPPortMax SIP RTP 端口上界。
	RTPPortMax uint16 `yaml:"rtp_port_max"`
	// Transport sipgo 传输：udp（默认）或 tls（预留）。
	Transport string `yaml:"transport"`
	// SessionExpiresSec 出局 Session-Expires 秒数（RFC 4028）；0 表示不发送。非 0 时须 ≥ 90。
	SessionExpiresSec int `yaml:"session_expires_sec"`
	// LocalRegistrar 是否接受白名单 IP 的软电话 REGISTER（本机演示）。公网中继应关闭。
	LocalRegistrar bool `yaml:"local_registrar"`
	// RegistrarPassword 本地 registrar Digest 密码（RFC 3261 §22）；local_registrar 时空则默认为 changeme。
	RegistrarPassword string `yaml:"registrar_password"`
	// TLSCertFile transport=tls 时的服务端证书。
	TLSCertFile string `yaml:"tls_cert_file"`
	// TLSKeyFile transport=tls 时的服务端私钥。
	TLSKeyFile string `yaml:"tls_key_file"`
	// Trunks 中继列表。
	Trunks []SIPTrunkConfig `yaml:"trunks"`
}

// SIPTrunkConfig 单条中继。
type SIPTrunkConfig struct {
	// ID 中继标识。
	ID string `yaml:"id"`
	// Host 对端主机。
	Host string `yaml:"host"`
	// Port 对端端口，默认 5060。
	Port int `yaml:"port"`
	// Username Digest / REGISTER 用户。
	Username string `yaml:"username"`
	// Password Digest / REGISTER 密码。
	Password string `yaml:"password"`
	// Realm Digest realm，空则使用质询中的值。
	Realm string `yaml:"realm"`
	// Register 是否向中继发起 REGISTER 并刷新。
	Register bool `yaml:"register"`
	// ExpireSec REGISTER Expires，默认 300。
	ExpireSec int `yaml:"expire_sec"`
	// FromUser 出局 CLI / P-Asserted-Identity 用户部分。
	FromUser string `yaml:"from_user"`
	// Codecs 出局 SDP 编解码，默认 PCMU+PCMA。
	Codecs []string `yaml:"codecs"`
	// AllowedCIDRs 入站 INVITE/REGISTER 源 IP 白名单。
	AllowedCIDRs []string `yaml:"allowed_cidrs"`
	// StripPrefix 出局拨号去掉的前缀。
	StripPrefix string `yaml:"strip_prefix"`
	// Prefix 出局拨号追加的前缀。
	Prefix string `yaml:"prefix"`
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

// ICEConfig 定义 STUN 等 ICE URL。
type ICEConfig struct {
	// STUNURLs STUN 服务器列表，内网同网段可留空或指向本机。
	STUNURLs []string `yaml:"stun_urls"`
	// UDPPortMin 媒体 RTP/ICE UDP 端口下界。
	UDPPortMin uint16 `yaml:"udp_port_min"`
	// UDPPortMax 媒体 RTP/ICE UDP 端口上界。
	UDPPortMax uint16 `yaml:"udp_port_max"`
}

// BootstrapConfig 空库演示账号。
type BootstrapConfig struct {
	// AdminPassword 管理员初始密码。
	AdminPassword string `yaml:"admin_password"`
	// AgentPassword 演示坐席初始密码。
	AgentPassword string `yaml:"agent_password"`
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

	cfg.applyDefaults()

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

	if c.ICE.UDPPortMin == 0 || c.ICE.UDPPortMax == 0 || c.ICE.UDPPortMin >= c.ICE.UDPPortMax {
		errs = append(errs, "ice.udp_port_min 必须小于 ice.udp_port_max 且均大于 0")
	}

	if c.SIP.Enabled {
		if strings.TrimSpace(c.SIP.Listen) == "" {
			errs = append(errs, "sip.enabled 为 true 时 sip.listen 不能为空")
		}
		if strings.TrimSpace(c.SIP.ExternalIP) == "" {
			errs = append(errs, "sip.enabled 为 true 时必须设置 sip.external_ip（本机演示可用 127.0.0.1）")
		}
		if c.SIP.RTPPortMin == 0 || c.SIP.RTPPortMax == 0 || c.SIP.RTPPortMin >= c.SIP.RTPPortMax {
			errs = append(errs, "sip.enabled 为 true 时 sip.rtp_port_min 必须小于 sip.rtp_port_max 且均大于 0")
		}
		transport := strings.ToLower(strings.TrimSpace(c.SIP.Transport))
		if transport != "udp" && transport != "tls" {
			errs = append(errs, "sip.transport 必须为 udp 或 tls")
		}
		if transport == "tls" {
			if strings.TrimSpace(c.SIP.TLSCertFile) == "" || strings.TrimSpace(c.SIP.TLSKeyFile) == "" {
				errs = append(errs, "sip.transport 为 tls 时必须设置 tls_cert_file 与 tls_key_file")
			}
		}
		if c.SIP.SessionExpiresSec < 0 {
			errs = append(errs, "sip.session_expires_sec 不能为负数")
		}
		if c.SIP.SessionExpiresSec > 0 && c.SIP.SessionExpiresSec < 90 {
			errs = append(errs, "sip.session_expires_sec 非 0 时必须 ≥ 90（RFC 4028 Min-SE）")
		}
		if len(c.SIP.Trunks) == 0 {
			errs = append(errs, "sip.enabled 为 true 时至少配置一条 sip.trunks")
		}
		for i, t := range c.SIP.Trunks {
			if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Host) == "" {
				errs = append(errs, fmt.Sprintf("sip.trunks[%d] 必须设置 id 与 host", i))
			}
			if t.Register && (strings.TrimSpace(t.Username) == "" || t.Password == "") {
				errs = append(errs, fmt.Sprintf("sip.trunks[%d] register 为 true 时必须设置 username 与 password", i))
			}
			for _, cidr := range t.AllowedCIDRs {
				if err := validateCIDR(cidr); err != nil {
					errs = append(errs, fmt.Sprintf("sip.trunks[%d].allowed_cidrs 含无效 CIDR %q", i, cidr))
				}
			}
			for _, codec := range t.Codecs {
				c := strings.ToUpper(strings.TrimSpace(codec))
				if c != "" && c != "PCMU" && c != "PCMA" {
					errs = append(errs, fmt.Sprintf("sip.trunks[%d].codecs 仅支持 PCMU/PCMA，收到 %q", i, codec))
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("配置校验失败:\n- %s", strings.Join(errs, "\n- "))
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.ICE.UDPPortMin == 0 && c.ICE.UDPPortMax == 0 {
		c.ICE.UDPPortMin = 10000
		c.ICE.UDPPortMax = 20000
	}
	if strings.TrimSpace(c.Bootstrap.AdminPassword) == "" {
		c.Bootstrap.AdminPassword = "changeme"
	}
	if strings.TrimSpace(c.Bootstrap.AgentPassword) == "" {
		c.Bootstrap.AgentPassword = "changeme"
	}
	if c.Recordings.RetainDays == 0 {
		c.Recordings.RetainDays = 90
	}
	if strings.TrimSpace(c.Recordings.NotifyMessage) == "" {
		c.Recordings.NotifyMessage = "本通话可能会被录音或录像，继续即表示您已知悉。"
	}
	if c.Webhook.MaxRetries <= 0 {
		c.Webhook.MaxRetries = 3
	}
	if c.Webhook.TimeoutSec <= 0 {
		c.Webhook.TimeoutSec = 5
	}
	if strings.TrimSpace(c.SIP.UserAgent) == "" {
		c.SIP.UserAgent = "open-voip"
	}
	if strings.TrimSpace(c.SIP.Listen) == "" {
		c.SIP.Listen = "0.0.0.0:5060"
	}
	if strings.TrimSpace(c.SIP.Transport) == "" {
		c.SIP.Transport = "udp"
	} else {
		c.SIP.Transport = strings.ToLower(strings.TrimSpace(c.SIP.Transport))
	}
	if c.SIP.LocalRegistrar && strings.TrimSpace(c.SIP.RegistrarPassword) == "" {
		c.SIP.RegistrarPassword = "changeme"
	}
	if c.SIP.RTPPortMin == 0 && c.SIP.RTPPortMax == 0 {
		c.SIP.RTPPortMin = 20000
		c.SIP.RTPPortMax = 20100
	}
	for i := range c.SIP.Trunks {
		if c.SIP.Trunks[i].Port <= 0 {
			c.SIP.Trunks[i].Port = 5060
		}
		if c.SIP.Trunks[i].ExpireSec <= 0 {
			c.SIP.Trunks[i].ExpireSec = 300
		}
		if len(c.SIP.Trunks[i].Codecs) == 0 {
			c.SIP.Trunks[i].Codecs = []string{"PCMU", "PCMA"}
		}
	}
}

func validateCIDR(s string) error {
	_, err := ParseIPNet(s)
	return err
}

// ParseIPNet 解析 CIDR 或单 IP（默认 /32 或 /128）。
func ParseIPNet(s string) (*net.IPNet, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty")
	}
	if !strings.Contains(s, "/") {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("invalid ip")
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		s = fmt.Sprintf("%s/%d", ip.String(), bits)
	}
	_, n, err := net.ParseCIDR(s)
	return n, err
}

// AdvertiseHost 返回 SIP Via/Contact/SDP 通告地址。
func (c SIPConfig) AdvertiseHost() string {
	ip := strings.TrimSpace(c.ExternalIP)
	if ip == "" || ip == "0.0.0.0" {
		return "127.0.0.1"
	}
	return ip
}

// Domain 返回 From/PAI 的 host。
func (c SIPConfig) Domain() string {
	if d := strings.TrimSpace(c.LocalDomain); d != "" {
		return d
	}
	return c.AdvertiseHost()
}

// ListenPort 解析 sip.listen 端口。
func (c SIPConfig) ListenPort() int {
	_, p, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return 5060
	}
	n, err := strconv.Atoi(p)
	if err != nil || n <= 0 {
		return 5060
	}
	return n
}

// CLIUser 出局主叫用户名：from_user > username。
func (t SIPTrunkConfig) CLIUser(fallback string) string {
	if u := strings.TrimSpace(t.FromUser); u != "" {
		return u
	}
	if u := strings.TrimSpace(t.Username); u != "" {
		return u
	}
	return fallback
}

// NormalizeDial 去掉 +/00 并按中继前后缀变换出局号码。
func (t SIPTrunkConfig) NormalizeDial(dest string) string {
	d := strings.TrimSpace(dest)
	d = strings.TrimPrefix(d, "+")
	d = strings.TrimPrefix(d, "00")
	strip := strings.TrimSpace(t.StripPrefix)
	if strip != "" && strings.HasPrefix(d, strip) {
		d = d[len(strip):]
	}
	if p := strings.TrimSpace(t.Prefix); p != "" && !strings.HasPrefix(d, p) {
		d = p + d
	}
	return d
}
