package http

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"open-call/internal/app/http/middleware"
	"open-call/internal/config"
)

// WrapSwitchBFF 将 /api/v1/calls 与相关 supervisor 通话路径代理到 open-switch。
func WrapSwitchBFF(cfg config.IntegrationConfig, auth middleware.Authenticator, next http.Handler, allowedOrigins ...[]string) http.Handler {
	target, err := url.Parse(strings.TrimRight(cfg.SwitchBaseURL, "/"))
	if err != nil {
		panic(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	proxy.Transport = transport
	origDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		origDirector(r)
		// 身份头只能由已验证的用户主体生成，禁止透传浏览器提供的身份。
		r.Header.Del("X-Principal")
		r.Header.Set("Authorization", "Bearer "+cfg.Secret)
		if p, ok := middleware.PrincipalFromContext(r.Context()); ok {
			b, _ := json.Marshal(p)
			r.Header.Set("X-Principal", string(b))
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "switch_unavailable", "message": "交换服务暂不可用"})
	}
	protected := middleware.Auth(auth)(proxy)

	origins := []string{}
	if len(allowedOrigins) > 0 {
		origins = allowedOrigins[0]
	}
	return middleware.CORS(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldProxyToSwitch(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		r = r.Clone(r.Context())
		r.URL.Path = mapSwitchPath(r.URL.Path)
		protected.ServeHTTP(w, r)
	}))
}

func mapSwitchPath(path string) string {
	switch {
	case path == "/api/v1/calls/outbound":
		return "/switch/v1/calls/outbound"
	case strings.HasPrefix(path, "/api/v1/calls/"):
		return strings.Replace(path, "/api/v1/calls/", "/switch/v1/calls/", 1)
	case strings.HasPrefix(path, "/api/v1/supervisor/calls/"):
		return strings.Replace(path, "/api/v1/supervisor/calls/", "/switch/v1/supervisor/calls/", 1)
	case strings.HasPrefix(path, "/api/v1/supervisor/agents/"):
		return strings.Replace(path, "/api/v1/supervisor/agents/", "/switch/v1/supervisor/agents/", 1)
	default:
		return path
	}
}

func shouldProxyToSwitch(path string) bool {
	if path == "/api/v1/calls/outbound" {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/calls/") {
		parts := strings.Split(strings.TrimPrefix(path, "/api/v1/calls/"), "/")
		if len(parts) == 1 {
			return parts[0] != "inbound"
		}
		suffix := strings.Join(parts[1:], "/")
		switch suffix {
		case "answer", "decline", "hangup", "hold", "transfer", "transfer/complete", "video/request", "video/respond", "video/downgrade", "screen-share", "conference", "dtmf", "turn-credentials":
			return true
		}
		if len(parts) == 4 && parts[1] == "legs" {
			switch parts[3] {
			case "offer", "answer", "ice", "mute":
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(path, "/api/v1/supervisor/calls/") {
		parts := strings.Split(strings.TrimPrefix(path, "/api/v1/supervisor/calls/"), "/")
		return len(parts) == 2 && parts[1] == "listen"
	}
	if strings.HasPrefix(path, "/api/v1/supervisor/agents/") {
		parts := strings.Split(strings.TrimPrefix(path, "/api/v1/supervisor/agents/"), "/")
		return len(parts) == 2 && parts[1] == "force-check-out"
	}
	return false
}
