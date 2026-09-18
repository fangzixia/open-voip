package config

import "strings"

// CORSConfig 跨域 REST 配置；前端独立部署时必须填写 allowed_origins。
type CORSConfig struct {
	// AllowedOrigins 允许携带 Authorization 的浏览器 Origin 列表，例如 https://ui.cc.internal。
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// CORSActive 是否启用 CORS 中间件。
func (c *Config) CORSActive() bool {
	return len(c.CORS.NormalizedOrigins()) > 0
}

// NormalizedOrigins 去空白、去尾斜杠后的 Origin 列表。
func (c *CORSConfig) NormalizedOrigins() []string {
	var out []string
	for _, o := range c.AllowedOrigins {
		o = strings.TrimSpace(o)
		o = strings.TrimSuffix(o, "/")
		if o != "" {
			out = append(out, o)
		}
	}
	return out
}

// WebSocketOriginPatterns 返回 WebSocket Accept 使用的 Origin 模式列表。
func (c *Config) WebSocketOriginPatterns() []string {
	if c.CORSActive() {
		return c.CORS.NormalizedOrigins()
	}
	if c.StaticServe {
		return []string{"*"}
	}
	return nil
}
