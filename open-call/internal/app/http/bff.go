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

// WrapSwitchBFF 将允许的 /api/v1 通话动作经 switchapi 服务端调用；IVR 资源仍走受限反向代理。
func WrapSwitchBFF(cfg config.IntegrationConfig, auth middleware.Authenticator, next http.Handler, allowedOrigins ...[]string) http.Handler {
	target, err := url.Parse(strings.TrimRight(cfg.SwitchBaseURL, "/"))
	if err != nil {
		panic(err)
	}
	ivrProxy := httputil.NewSingleHostReverseProxy(target)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	ivrProxy.Transport = transport
	switchClient := switchapi.NewClient(cfg)
	origDirector := ivrProxy.Director
	ivrProxy.Director = func(r *http.Request) {
		origDirector(r)
		r.Header.Del("X-Principal")
		r.Header.Del("X-Agent-ID")
		r.Header.Del("Authorization")
		if p, ok := middleware.PrincipalFromContext(r.Context()); ok && p.AgentID != "" {
			r.Header.Set("X-Agent-ID", p.AgentID)
		}
		if traceID := observability.From(r.Context()).TraceID; traceID != "" {
			r.Header.Set("X-Trace-ID", traceID)
		}
	}
	ivrProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
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
		if p.IsGuest() && (strings.Contains(r.URL.Path, "/supervisor/") || strings.HasSuffix(r.URL.Path, "/outbound") || strings.HasPrefix(r.URL.Path, "/switch/v2/ivr-assets")) {
			httpapi.Error(w, errs.Forbidden("无权限"))
			return
		}
		if err := prepareSwitchRequest(r, p, switchClient); err != nil {
			httpapi.Error(w, err)
			return
		}
		r = attachSwitchMutation(r)
		if err := serveSwitchBFF(w, r, switchClient); err != nil {
			httpapi.Error(w, err)
			return
		}
	}))

	ivrProtected := middleware.Auth(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		r = r.Clone(r.Context())
		r.URL.Path = strings.Replace(r.URL.Path, "/api/v1/ivr-assets", "/switch/v2/ivr-assets", 1)
		ivrProxy.ServeHTTP(w, r)
	}))

	origins := []string{}
	if len(allowedOrigins) > 0 {
		origins = allowedOrigins[0]
	}
	return middleware.RequestID(httpapi.Recover(middleware.CORS(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isIVRAssetPath(r.URL.Path) {
			ivrProtected.ServeHTTP(w, r)
			return
		}
		if !shouldProxyToSwitch(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		r = r.Clone(r.Context())
		r.URL.Path = mapSwitchPath(r.URL.Path)
		protected.ServeHTTP(w, r)
	}))))
}

func isIVRAssetPath(path string) bool {
	if path == "/api/v1/ivr-assets/tts-options" || path == "/api/v1/ivr-assets/synthesize" {
		return false
	}
	return path == "/api/v1/ivr-assets" || strings.HasPrefix(path, "/api/v1/ivr-assets/")
}

func switchPermission(method, path string) string {
	if strings.HasPrefix(path, "/switch/v2/ivr-assets") {
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
	for _, prefix := range []string{"/switch/v2/calls/", "/switch/v2/supervisor/calls/"} {
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
		return "/switch/v2/calls/outbound"
	case path == "/api/v1/calls":
		return "/switch/v2/calls"
	case strings.HasPrefix(path, "/api/v1/calls/"):
		return strings.Replace(path, "/api/v1/calls/", "/switch/v2/calls/", 1)
	case strings.HasPrefix(path, "/api/v1/supervisor/calls/"):
		return strings.Replace(path, "/api/v1/supervisor/calls/", "/switch/v2/supervisor/calls/", 1)
	case strings.HasPrefix(path, "/api/v1/supervisor/agents/"):
		return strings.Replace(path, "/api/v1/supervisor/agents/", "/switch/v2/supervisor/agents/", 1)
	default:
		return path
	}
}

func shouldProxyToSwitch(path string) bool {
	if isIVRAssetPath(path) {
		return false
	}
	if path == "/api/v1/calls/outbound" || path == "/api/v1/calls" {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/calls/") {
		parts := strings.Split(strings.TrimPrefix(path, "/api/v1/calls/"), "/")
		if len(parts) == 1 && parts[0] != "" && parts[0] != "inbound" && parts[0] != "outbound" {
			return true
		}
		suffix := strings.Join(parts[1:], "/")
		switch suffix {
		case "answer", "decline", "hangup", "hold", "transfer", "transfer/complete", "video/request", "video/respond", "video/downgrade", "screen-share", "conference", "dtmf", "turn-credentials", "bridges", "bridge":
			return true
		}
		if len(parts) >= 3 && parts[1] == "legs" {
			switch parts[3] {
			case "offer", "answer", "ice", "mute", "hold", "reject", "playbacks":
				return true
			}
			if len(parts) >= 5 && parts[3] == "playbacks" {
				return true
			}
		}
		if len(parts) >= 3 && parts[1] == "bridges" {
			return true
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
