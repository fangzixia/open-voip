package http

import (
	"context"
	"net/http"
	"open-call/internal/httpapi"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"open-call/internal/app/http/middleware"
	"open-call/internal/config"
	"open-call/internal/layers/biz/agent"
	"open-call/internal/layers/biz/audit"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/layers/biz/cdr"
	"open-call/internal/layers/biz/configio"
	"open-call/internal/layers/biz/configpub"
	"open-call/internal/layers/biz/guest"
	"open-call/internal/layers/biz/ivr"
	"open-call/internal/layers/biz/queue"
	"open-call/internal/layers/biz/recmeta"
	"open-call/internal/layers/biz/report"
	"open-call/internal/layers/biz/skill"
	"open-call/internal/layers/biz/user"
	"open-call/internal/layers/biz/webhook"
	"open-call/internal/ports"
)

// RouterDeps HTTP 路由依赖。
type RouterDeps struct {
	Config     config.Config
	Status     StatusProvider
	Auth       *auth.Service
	Users      *user.Service
	Agents     *agent.Service
	Queues     *queue.Service
	Guests     *guest.Service
	CDR        *cdr.RecorderService
	IVR        *ivr.Service
	Skills     *skill.Service
	Recordings *recmeta.Service
	Reports    *report.Service
	Webhooks   *webhook.Service
	Audit      *audit.Service
	ConfigIO   *configio.Service
	Snapshots  *configpub.SnapshotService
	Hub        interface {
		ServeHTTP(w http.ResponseWriter, r *http.Request)
	}
	Calls interface {
		GetCall(context.Context, string) (ports.CallView, error)
	}
}

// NewRouter 构建 chi 路由。
func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.NotFound(httpapi.NotFound)
	r.MethodNotAllowed(httpapi.MethodNotAllowed)
	r.Use(chimw.RealIP)
	r.Use(httpapi.Recover)
	r.Use(middleware.RequestID)
	r.Use(chimw.Logger)
	r.Use(middleware.CORS(deps.Config.Security.AllowedOrigins))

	r.Get("/health", handleHealth)
	r.Get("/health/live", handleHealth)
	r.Get("/health/ready", deps.Status.handleReady)

	r.Route("/api/v1", func(api chi.Router) {
		api.With(middleware.RateLimit(deps.Config.Security.LoginRequestsPerMin)).Post("/auth/login", deps.handleLogin)
		api.With(middleware.RateLimit(deps.Config.Security.LoginRequestsPerMin)).Post("/auth/refresh", deps.handleRefresh)

		api.With(middleware.RateLimit(deps.Config.Security.GuestRequestsPerMin)).Get("/guest/queues", deps.handleGuestQueues)
		api.With(middleware.RateLimit(deps.Config.Security.GuestRequestsPerMin)).Post("/guest/join", deps.handleGuestJoin)

		if deps.Hub != nil {
			api.Get("/ws", deps.Hub.ServeHTTP)
		}
		if deps.Auth != nil {
			api.With(
				middleware.RateLimit(deps.Config.Security.ClientEventRequestsPerMin),
				middleware.Auth(deps.Auth),
			).Post("/client-events", deps.handleClientEvents)
		}

		api.Group(func(priv chi.Router) {
			if deps.Auth != nil {
				priv.Use(middleware.Auth(deps.Auth))
			}
			priv.Use(middleware.AuditMutations(deps.Audit))
			priv.Post("/auth/logout", deps.handleLogout)
			priv.Post("/auth/change-password", deps.handleChangePassword)
			priv.Get("/auth/sessions", deps.handleSessionList)
			priv.Delete("/auth/sessions/{sessionId}", deps.handleSessionRevoke)

			priv.Group(func(admin chi.Router) {
				admin.Use(middleware.RequireRoles("admin"))
				admin.Use(middleware.RateLimit(deps.Config.Security.AdminRequestsPerMin))
				admin.Get("/users", deps.handleUserList)
				admin.Post("/users", deps.handleUserCreate)
				admin.Get("/users/{userId}", deps.handleUserGet)
				admin.Patch("/users/{userId}", deps.handleUserPatch)
				admin.Delete("/users/{userId}", deps.handleUserDelete)
				admin.Post("/users/{userId}/reset-password", deps.handleUserResetPassword)
				admin.Post("/users/{userId}/revoke-sessions", deps.handleUserRevokeSessions)
				admin.Post("/queues", deps.handleQueueCreate)
				admin.Patch("/queues/{queueId}", deps.handleQueuePatch)
				admin.Delete("/queues/{queueId}", deps.handleQueueDelete)
				admin.Put("/queues/{queueId}/agents", deps.handleQueueBind)
				admin.Get("/skills", deps.handleSkillList)
				admin.Post("/skills", deps.handleSkillCreate)
				admin.Patch("/skills/{skillId}", deps.handleSkillUpdate)
				admin.Delete("/skills/{skillId}", deps.handleSkillDelete)
				admin.Put("/agents/{agentId}/skills", deps.handleAgentSkills)
				admin.Get("/agents/{agentId}/skills", deps.handleAgentSkillsGet)
				admin.Get("/ivr/flows", deps.handleIVRList)
				admin.Post("/ivr/flows", deps.handleIVRCreate)
				admin.Get("/ivr/flows/{flowId}", deps.handleIVRGet)
				admin.Patch("/ivr/flows/{flowId}", deps.handleIVRUpdate)
				admin.Delete("/ivr/flows/{flowId}", deps.handleIVRDelete)
				admin.Get("/ivr/flows/{flowId}/versions", deps.handleIVRVersions)
				admin.Post("/ivr/flows/{flowId}/rollback", deps.handleIVRRollback)
				admin.Post("/ivr/flows/{flowId}/publish", deps.handleIVRPublish)
				admin.Get("/cdr/export.csv", deps.handleCDRExport)
				admin.Get("/webhooks/subscriptions", deps.handleWebhookList)
				admin.Post("/webhooks/subscriptions", deps.handleWebhookCreate)
				admin.Patch("/webhooks/subscriptions/{subscriptionId}", deps.handleWebhookUpdate)
				admin.Delete("/webhooks/subscriptions/{subscriptionId}", deps.handleWebhookDelete)
				admin.Get("/webhooks/deliveries", deps.handleWebhookDeliveryList)
				admin.Post("/webhooks/deliveries/{deliveryId}/retry", deps.handleWebhookRetry)
				admin.Get("/config/export", deps.handleConfigExport)
				admin.Post("/config/import", deps.handleConfigImport)
				admin.Get("/audit/logs", deps.handleAuditList)
				admin.Post("/admin/recordings/purge-expired", deps.handlePurgeRecordings)
				admin.Get("/dids", deps.handleDIDList)
				admin.Post("/dids", deps.handleDIDUpsert)
				admin.Delete("/dids/{didId}", deps.handleDIDDelete)
			})

			priv.Group(func(ops chi.Router) {
				ops.Use(middleware.RequireRoles("admin", "supervisor"))
				ops.Get("/cdr", deps.handleCDRList)
				ops.Get("/recordings", deps.handleRecordingList)
				ops.Get("/recordings/{recordingId}/download", deps.handleRecordingDownload)
				ops.Get("/reports/live", deps.handleLiveReport)
				ops.Get("/reports/historical", deps.handleHistoricalReport)
				ops.Get("/reports/agents", deps.handleAgentUtil)
				ops.Get("/calls/{callId}/qa-marks", deps.handleQAList)
				ops.Post("/calls/{callId}/qa-marks", deps.handleQACreate)
				ops.Patch("/calls/{callId}/qa-marks/{markId}", deps.handleQAUpdate)
				ops.Delete("/calls/{callId}/qa-marks/{markId}", deps.handleQADelete)
				ops.Get("/wrap-ups", deps.handleWrapUpList)
				ops.Get("/status", deps.Status.handleStatus)
			})

			priv.Group(func(staff chi.Router) {
				staff.Use(middleware.RequireRoles("admin", "supervisor", "agent"))
				staff.Get("/queues", deps.handleQueueList)
				staff.Get("/queues/{queueId}", deps.handleQueueGet)
				staff.Get("/agents/me", deps.handleAgentMe)
				staff.Get("/agents/me/calls", deps.handleMyCalls)
				staff.Get("/agents", deps.handleAgentList)
				staff.Post("/agents/{agentId}/check-in", deps.handleCheckIn)
				staff.Post("/agents/{agentId}/check-out", deps.handleCheckOut)
				staff.Put("/agents/{agentId}/state", deps.handleAgentState)
				staff.Post("/guest/sessions", deps.handleGuestSessions)
			})

			priv.Post("/calls/{callId}/wrap-up", deps.handleWrapUp)
		})
	})

	return r
}
