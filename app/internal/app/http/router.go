package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"open-voip/internal/app/http/middleware"
	"open-voip/internal/config"
	"open-voip/internal/ports"
)

// RouterDeps HTTP 路由依赖，仅包含 Port/Service 接口。
type RouterDeps struct {
	// Config 运行时配置。
	Config config.Config
	// Status 状态探针数据源。
	Status StatusProvider
	// CallControl 呼叫控制 Port（Phase 1 handler 使用）。
	CallControl ports.CallControlPort
	// Hub WebSocket 网关（Phase 1 注册 WS 路由）。
	Hub interface {
		ServeHTTP(w http.ResponseWriter, r *http.Request)
	}
}

// NewRouter 构建 chi 路由：平台探针、API 占位、静态资源。
func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(chimw.Logger)

	if deps.Config.CORSActive() {
		r.Use(middleware.CORS(deps.Config.CORS.NormalizedOrigins()))
	}

	r.Get("/health", handleHealth)

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/status", deps.Status.handleStatus)

		// WebSocket 业务通道，见 docs/events.md
		if deps.Hub != nil {
			api.Get("/ws", deps.Hub.ServeHTTP)
		}

		// Phase 1 起按 OpenAPI 拆分 admin/agent/guest/media 子路由。
		api.Route("/auth", func(auth chi.Router) {
			auth.Post("/login", notImplemented("登录"))
		})
	})

	if deps.Config.StaticServe {
		mountStaticApps(r, deps.Config.Static.Dir)
	}

	return r
}

func notImplemented(feature string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, feature+"：尚未实现", http.StatusNotImplemented)
	}
}
