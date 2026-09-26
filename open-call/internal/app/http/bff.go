package http

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"open-call/internal/httpapi"
	"open-call/internal/integration/switchapi"
	"open-call/internal/observability"
	"strings"
	"time"

	"open-call/internal/app/http/middleware"
	"open-call/internal/config"
	"open-call/internal/errs"
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
	switchClient := switchapi.NewClient(cfg)
	origDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		origDirector(r)
		// Switch 只接受服务命令；删除浏览器伪造的身份提示头。
		r.Header.Del("X-Principal")
		r.Header.Del("X-Agent-ID")
		r.Header.Set("Authorization", "Bearer "+cfg.Secret)
		if p, ok := middleware.PrincipalFromContext(r.Context()); ok && p.AgentID != "" {
			r.Header.Set("X-Agent-ID", p.AgentID)
		}
		if traceID := observability.From(r.Context()).TraceID; traceID != "" {
			r.Header.Set("X-Trace-ID", traceID)
		}
		if callID := switchCallID(r.URL.Path); callID != "" {
			r.Header.Set("X-Call-ID", callID)
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		httpapi.Failure(w, http.StatusBadGateway, "switch_unavailable", "交换服务暂不可用")
	}
	protected := middleware.Auth(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := middleware.PrincipalFromContext(r.Context())
		if !ok {
			httpapi.Error(w, errs.Unauthorized("未认证"))
			return
		}
		code := switchPermission(r.Method, r.URL.Path)
		if !p.IsGuest() && !p.Has(code) {
			httpapi.Error(w, errs.Forbidden("无权限"))
			return
		}
		if p.IsGuest() && (strings.Contains(r.URL.Path, "/supervisor/") || strings.HasSuffix(r.URL.Path, "/outbound") || strings.HasPrefix(r.URL.Path, "/switch/v1/ivr-assets")) {
			httpapi.Error(w, errs.Forbidden("无权限"))
			return
		}
		if err := prepareSwitchRequest(r, p, switchClient); err != nil {
			httpapi.Error(w, err)
			return
		}
		proxy.ServeHTTP(w, r)
	}))

	origins := []string{}
	if len(allowedOrigins) > 0 {
		origins = allowedOrigins[0]
	}
	return middleware.RequestID(httpapi.Recover(middleware.CORS(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldProxyToSwitch(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		r = r.Clone(r.Context())
		r.URL.Path = mapSwitchPath(r.URL.Path)
		protected.ServeHTTP(w, r)
	}))))
}

func switchPermission(method, path string) string {
	if strings.HasPrefix(path, "/switch/v1/ivr-assets") {
		if method == http.MethodGet {
			return "ivr.read"
		}
		return "ivr.write"
	}
	if strings.Contains(path, "/supervisor/agents/") {
		return "agents.force_checkout"
	}
	if strings.Contains(path, "/supervisor/calls/") {
		return "calls.listen"
	}
	if method == http.MethodGet {
		return "calls.read"
	}
	return "calls.operate"
}

func switchCallID(path string) string {
	for _, prefix := range []string{"/switch/v1/calls/", "/switch/v1/supervisor/calls/"} {
		if strings.HasPrefix(path, prefix) {
			id := strings.Split(strings.TrimPrefix(path, prefix), "/")[0]
			return observability.NormalizeID(id)
		}
	}
	return ""
}

func mapSwitchPath(path string) string {
	switch {
	case path == "/api/v1/calls/outbound":
		return "/switch/v1/calls/outbound"
	case strings.HasPrefix(path, "/api/v1/ivr-assets"):
		return strings.Replace(path, "/api/v1/ivr-assets", "/switch/v1/ivr-assets", 1)
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
	if path == "/api/v1/ivr-assets" || strings.HasPrefix(path, "/api/v1/ivr-assets/") {
		return true
	}
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
