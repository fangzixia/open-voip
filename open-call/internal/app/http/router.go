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
	"open-call/internal/layers/biz/authz"
	"open-call/internal/layers/biz/cdr"
	"open-call/internal/layers/biz/configio"
	"open-call/internal/layers/biz/configpub"
	"open-call/internal/layers/biz/guest"
	"open-call/internal/layers/biz/ivr"
	"open-call/internal/layers/biz/oidcauth"
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
	Config        config.Config
	Status        StatusProvider
	Auth          *auth.Service
	Authorization *authz.Service
	OIDC          *oidcauth.Service
	Users         *user.Service
	Agents        *agent.Service
	Queues        *queue.Service
	Guests        *guest.Service
	CDR           *cdr.RecorderService
	IVR           *ivr.Service
	Skills        *skill.Service
	Recordings    *recmeta.Service
	Reports       *report.Service
	Webhooks      *webhook.Service
	Audit         *audit.Service
	ConfigIO      *configio.Service
	Snapshots     *configpub.SnapshotService
	Hub           interface {
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
		api.Get("/auth/options", deps.handleAuthOptions)
		api.With(middleware.RateLimit(deps.Config.Security.LoginRequestsPerMin)).Post("/auth/refresh", deps.handleRefresh)
		if deps.OIDC != nil {
			api.Get("/auth/oidc/start", deps.handleOIDCStart)
			api.Get("/auth/oidc/callback", deps.handleOIDCCallback)
			api.With(middleware.RateLimit(deps.Config.Security.LoginRequestsPerMin)).Post("/auth/oidc/exchange", deps.handleOIDCExchange)
		}

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
			priv.Get("/auth/me", deps.handleMe)
			priv.Post("/auth/change-password", deps.handleChangePassword)
			priv.Get("/auth/sessions", deps.handleSessionList)
			priv.Delete("/auth/sessions/{sessionId}", deps.handleSessionRevoke)

			priv.Group(func(admin chi.Router) {
				admin.Use(middleware.RateLimit(deps.Config.Security.AdminRequestsPerMin))
				admin.With(middleware.RequirePermission("users.read")).Get("/users", deps.handleUserList)
				admin.With(middleware.RequirePermission("users.create")).Post("/users", deps.handleUserCreate)
				admin.With(middleware.RequirePermission("users.read")).Get("/users/{userId}", deps.handleUserGet)
				admin.With(middleware.RequirePermission("users.update")).Patch("/users/{userId}", deps.handleUserPatch)
				admin.With(middleware.RequirePermission("users.delete")).Delete("/users/{userId}", deps.handleUserDelete)
				admin.With(middleware.RequirePermission("users.credentials")).Post("/users/{userId}/reset-password", deps.handleUserResetPassword)
				admin.With(middleware.RequirePermission("users.sessions")).Post("/users/{userId}/revoke-sessions", deps.handleUserRevokeSessions)
				admin.With(middleware.RequirePermission("users.read")).Get("/users/{userId}/roles", deps.handleUserRoles)
				admin.With(middleware.RequirePermission("users.update")).Put("/users/{userId}/roles", deps.handleUserRolesPut)
				admin.With(middleware.RequirePermission("identity.read")).Get("/users/{userId}/identities", deps.handleIdentities)
				admin.With(middleware.RequirePermission("identity.write")).Post("/users/{userId}/identities", deps.handleIdentityBind)
				admin.With(middleware.RequirePermission("identity.write")).Delete("/users/{userId}/identities", deps.handleIdentityDelete)
				admin.With(middleware.RequirePermission("roles.read")).Get("/roles", deps.handleRoleList)
				admin.With(middleware.RequirePermission("roles.read")).Get("/permissions", deps.handlePermissionList)
				admin.With(middleware.RequirePermission("roles.write")).Put("/roles/{roleId}", deps.handleRoleSave)
				admin.With(middleware.RequirePermission("roles.write")).Delete("/roles/{roleId}", deps.handleRoleDelete)
				admin.With(middleware.RequirePermission("identity.read")).Get("/identity/group-mappings", deps.handleGroupMappings)
				admin.With(middleware.RequirePermission("identity.write")).Put("/identity/group-mappings", deps.handleGroupMappingSave)
				admin.With(middleware.RequirePermission("identity.write")).Put("/identity/group-mappings/{group}", deps.handleGroupMappingSave)
				admin.With(middleware.RequirePermission("queues.write")).Post("/queues", deps.handleQueueCreate)
				admin.With(middleware.RequirePermission("queues.write")).Patch("/queues/{queueId}", deps.handleQueuePatch)
				admin.With(middleware.RequirePermission("queues.write")).Delete("/queues/{queueId}", deps.handleQueueDelete)
				admin.With(middleware.RequirePermission("queues.write")).Put("/queues/{queueId}/agents", deps.handleQueueBind)
				admin.With(middleware.RequirePermission("skills.read")).Get("/skills", deps.handleSkillList)
				admin.With(middleware.RequirePermission("skills.write")).Post("/skills", deps.handleSkillCreate)
				admin.With(middleware.RequirePermission("skills.write")).Patch("/skills/{skillId}", deps.handleSkillUpdate)
				admin.With(middleware.RequirePermission("skills.write")).Delete("/skills/{skillId}", deps.handleSkillDelete)
				admin.With(middleware.RequirePermission("skills.write")).Put("/agents/{agentId}/skills", deps.handleAgentSkills)
				admin.With(middleware.RequirePermission("skills.read")).Get("/agents/{agentId}/skills", deps.handleAgentSkillsGet)
				admin.With(middleware.RequirePermission("ivr.read")).Get("/ivr/flows", deps.handleIVRList)
				admin.With(middleware.RequirePermission("ivr.write")).Post("/ivr/flows", deps.handleIVRCreate)
				admin.With(middleware.RequirePermission("ivr.read")).Get("/ivr/flows/{flowId}", deps.handleIVRGet)
				admin.With(middleware.RequirePermission("ivr.write")).Patch("/ivr/flows/{flowId}", deps.handleIVRUpdate)
				admin.With(middleware.RequirePermission("ivr.write")).Delete("/ivr/flows/{flowId}", deps.handleIVRDelete)
				admin.With(middleware.RequirePermission("ivr.read")).Get("/ivr/flows/{flowId}/versions", deps.handleIVRVersions)
				admin.With(middleware.RequirePermission("ivr.write")).Post("/ivr/flows/{flowId}/rollback", deps.handleIVRRollback)
				admin.With(middleware.RequirePermission("ivr.write")).Post("/ivr/flows/{flowId}/publish", deps.handleIVRPublish)
				admin.With(middleware.RequirePermission("cdr.export")).Get("/cdr/export.csv", deps.handleCDRExport)
				admin.With(middleware.RequirePermission("webhooks.read")).Get("/webhooks/subscriptions", deps.handleWebhookList)
				admin.With(middleware.RequirePermission("webhooks.write")).Post("/webhooks/subscriptions", deps.handleWebhookCreate)
				admin.With(middleware.RequirePermission("webhooks.write")).Patch("/webhooks/subscriptions/{subscriptionId}", deps.handleWebhookUpdate)
				admin.With(middleware.RequirePermission("webhooks.write")).Delete("/webhooks/subscriptions/{subscriptionId}", deps.handleWebhookDelete)
				admin.With(middleware.RequirePermission("webhooks.read")).Get("/webhooks/deliveries", deps.handleWebhookDeliveryList)
				admin.With(middleware.RequirePermission("webhooks.write")).Post("/webhooks/deliveries/{deliveryId}/retry", deps.handleWebhookRetry)
				admin.With(middleware.RequirePermission("config.read")).Get("/config/export", deps.handleConfigExport)
				admin.With(middleware.RequirePermission("config.write")).Post("/config/import", deps.handleConfigImport)
				admin.With(middleware.RequirePermission("audit.read")).Get("/audit/logs", deps.handleAuditList)
				admin.With(middleware.RequirePermission("recordings.purge")).Post("/admin/recordings/purge-expired", deps.handlePurgeRecordings)
				admin.With(middleware.RequirePermission("dids.read")).Get("/dids", deps.handleDIDList)
				admin.With(middleware.RequirePermission("dids.write")).Post("/dids", deps.handleDIDUpsert)
				admin.With(middleware.RequirePermission("dids.write")).Delete("/dids/{didId}", deps.handleDIDDelete)
			})

			priv.Group(func(ops chi.Router) {
				ops.With(middleware.RequirePermission("cdr.read")).Get("/cdr", deps.handleCDRList)
				ops.With(middleware.RequirePermission("recordings.read")).Get("/recordings", deps.handleRecordingList)
				ops.With(middleware.RequirePermission("recordings.download")).Get("/recordings/{recordingId}/download", deps.handleRecordingDownload)
				ops.With(middleware.RequirePermission("reports.read")).Get("/reports/live", deps.handleLiveReport)
				ops.With(middleware.RequirePermission("reports.read")).Get("/reports/historical", deps.handleHistoricalReport)
				ops.With(middleware.RequirePermission("reports.read")).Get("/reports/agents", deps.handleAgentUtil)
				ops.With(middleware.RequirePermission("recordings.read")).Get("/calls/{callId}/qa-marks", deps.handleQAList)
				ops.With(middleware.RequirePermission("recordings.qa")).Post("/calls/{callId}/qa-marks", deps.handleQACreate)
				ops.With(middleware.RequirePermission("recordings.qa")).Patch("/calls/{callId}/qa-marks/{markId}", deps.handleQAUpdate)
				ops.With(middleware.RequirePermission("recordings.qa")).Delete("/calls/{callId}/qa-marks/{markId}", deps.handleQADelete)
				ops.With(middleware.RequirePermission("cdr.read")).Get("/wrap-ups", deps.handleWrapUpList)
				ops.With(middleware.RequirePermission("status.read")).Get("/status", deps.Status.handleStatus)
			})

			priv.Group(func(staff chi.Router) {
				staff.With(middleware.RequirePermission("queues.read")).Get("/queues", deps.handleQueueList)
				staff.With(middleware.RequirePermission("queues.read")).Get("/queues/{queueId}", deps.handleQueueGet)
				staff.With(middleware.RequirePermission("agents.self")).Get("/agents/me", deps.handleAgentMe)
				staff.With(middleware.RequirePermission("agents.self")).Get("/agents/me/calls", deps.handleMyCalls)
				staff.With(middleware.RequirePermission("agents.read")).Get("/agents", deps.handleAgentList)
				staff.With(middleware.RequirePermission("agents.self")).Post("/agents/{agentId}/check-in", deps.handleCheckIn)
				staff.With(middleware.RequirePermission("agents.self")).Post("/agents/{agentId}/check-out", deps.handleCheckOut)
				staff.With(middleware.RequirePermission("agents.self")).Put("/agents/{agentId}/state", deps.handleAgentState)
				staff.With(middleware.RequirePermission("guest.issue")).Post("/guest/sessions", deps.handleGuestSessions)
			})

			priv.With(middleware.RequirePermission("calls.wrap_up")).Post("/calls/{callId}/wrap-up", deps.handleWrapUp)
		})
	})

	return r
}
