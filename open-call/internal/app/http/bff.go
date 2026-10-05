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
	"open-call/internal/layers/biz/audit"
)

// WrapSwitchBFF 将允许的 /api/v1 通话动作经 switchapi 服务端调用；IVR 资源仍走受限反向代理。
func WrapSwitchBFF(cfg config.IntegrationConfig, auth middleware.Authenticator, next http.Handler, allowedOrigins []string, auditSvc *audit.Service) http.Handler {
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

	protected := middleware.Auth(auth)(middleware.AuditMutations(auditSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		r = attachSwitchMutation(r)
		if err := serveSwitchBFF(w, r, switchClient); err != nil {
			httpapi.Error(w, err)
			return
		}
	})))

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
		r.URL.Path = strings.Replace(r.URL.Path, "/api/v1/ivr-assets", "/switch/v1/ivr-assets", 1)
		ivrProxy.ServeHTTP(w, r)
	}))

	return middleware.RequestID(httpapi.Recover(middleware.CORS(allowedOrigins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
