package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"open-voip/internal/app/http/middleware"
	"open-voip/internal/config"
	"open-voip/internal/layers/biz/agent"
	"open-voip/internal/layers/biz/audit"
	"open-voip/internal/layers/biz/auth"
	"open-voip/internal/layers/biz/cdr"
	"open-voip/internal/layers/biz/configio"
	"open-voip/internal/layers/biz/configpub"
	"open-voip/internal/layers/biz/guest"
	"open-voip/internal/layers/biz/ivr"
	"open-voip/internal/layers/biz/queue"
	"open-voip/internal/layers/biz/recmeta"
	"open-voip/internal/layers/biz/report"
	"open-voip/internal/layers/biz/skill"
	"open-voip/internal/layers/biz/user"
	"open-voip/internal/layers/biz/webhook"
	"open-voip/internal/ports"
)

// RouterDeps HTTP 路由依赖。
type RouterDeps struct {
	Config      config.Config
	Status      StatusProvider
	Auth        *auth.Service
	Users       *user.Service
	Agents      *agent.Service
	Queues      *queue.Service
	Guests      *guest.Service
	CDR         *cdr.RecorderService
	CallControl ports.CallControlPort
	Signaling   ports.SignalingPort
	IVR         *ivr.Service
	Skills      *skill.Service
	Recordings  *recmeta.Service
	Reports     *report.Service
	Webhooks    *webhook.Service
	Audit       *audit.Service
	ConfigIO    *configio.Service
	Snapshots   *configpub.SnapshotService
	Hub         interface {
		ServeHTTP(w http.ResponseWriter, r *http.Request)
	}
}

// NewRouter 构建 chi 路由。
func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(chimw.Logger)
	r.Use(middleware.CORS)

	r.Get("/health", handleHealth)

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/status", deps.Status.handleStatus)

		api.Post("/auth/login", deps.handleLogin)
		api.Post("/auth/refresh", deps.handleRefresh)

		api.Get("/guest/queues", deps.handleGuestQueues)
		api.Post("/guest/join", deps.handleGuestJoin)

		if deps.Hub != nil {
			api.Get("/ws", deps.Hub.ServeHTTP)
		}

		api.Group(func(priv chi.Router) {
			if deps.Auth != nil {
				priv.Use(middleware.Auth(deps.Auth))
			}
			priv.Post("/auth/logout", deps.handleLogout)

			priv.Group(func(admin chi.Router) {
				admin.Use(middleware.RequireRoles("admin"))
				admin.Get("/users", deps.handleUserList)
				admin.Post("/users", deps.handleUserCreate)
				admin.Get("/users/{userId}", deps.handleUserGet)
				admin.Patch("/users/{userId}", deps.handleUserPatch)
				admin.Delete("/users/{userId}", deps.handleUserDelete)
				admin.Post("/users/{userId}/reset-password", deps.handleUserResetPassword)
				admin.Post("/queues", deps.handleQueueCreate)
				admin.Patch("/queues/{queueId}", deps.handleQueuePatch)
				admin.Delete("/queues/{queueId}", deps.handleQueueDelete)
				admin.Put("/queues/{queueId}/agents", deps.handleQueueBind)
				admin.Get("/skills", deps.handleSkillList)
				admin.Post("/skills", deps.handleSkillCreate)
				admin.Put("/agents/{agentId}/skills", deps.handleAgentSkills)
				admin.Get("/agents/{agentId}/skills", deps.handleAgentSkillsGet)
				admin.Get("/ivr/flows", deps.handleIVRList)
				admin.Post("/ivr/flows", deps.handleIVRCreate)
				admin.Post("/ivr/flows/{flowId}/publish", deps.handleIVRPublish)
				admin.Get("/cdr/export.csv", deps.handleCDRExport)
				admin.Get("/webhooks/subscriptions", deps.handleWebhookList)
				admin.Post("/webhooks/subscriptions", deps.handleWebhookCreate)
				admin.Post("/webhooks/deliveries/{deliveryId}/retry", deps.handleWebhookRetry)
				admin.Get("/config/export", deps.handleConfigExport)
				admin.Post("/config/import", deps.handleConfigImport)
				admin.Get("/audit/logs", deps.handleAuditList)
				admin.Post("/admin/recordings/purge-expired", deps.handlePurgeRecordings)
				admin.Get("/dids", deps.handleDIDList)
				admin.Post("/dids", deps.handleDIDUpsert)
			})

			priv.Group(func(ops chi.Router) {
				ops.Use(middleware.RequireRoles("admin", "supervisor"))
				ops.Get("/cdr", deps.handleCDRList)
				ops.Get("/recordings", deps.handleRecordingList)
				ops.Get("/recordings/{recordingId}/download", deps.handleRecordingDownload)
				ops.Get("/reports/live", deps.handleLiveReport)
				ops.Get("/reports/historical", deps.handleHistoricalReport)
				ops.Get("/reports/agents", deps.handleAgentUtil)
				ops.Post("/supervisor/agents/{agentId}/force-check-out", deps.handleForceCheckout)
				ops.Post("/supervisor/calls/{callId}/listen", deps.handleListen)
				ops.Get("/calls/{callId}/qa-marks", deps.handleQAList)
				ops.Post("/calls/{callId}/qa-marks", deps.handleQACreate)
				ops.Get("/wrap-ups", deps.handleWrapUpList)
			})

			priv.Group(func(staff chi.Router) {
				staff.Use(middleware.RequireRoles("admin", "supervisor", "agent"))
				staff.Get("/queues", deps.handleQueueList)
				staff.Get("/queues/{queueId}", deps.handleQueueGet)
				staff.Get("/agents/me", deps.handleAgentMe)
				staff.Get("/agents", deps.handleAgentList)
				staff.Post("/agents/{agentId}/check-in", deps.handleCheckIn)
				staff.Post("/agents/{agentId}/check-out", deps.handleCheckOut)
				staff.Put("/agents/{agentId}/state", deps.handleAgentState)
				staff.Post("/guest/sessions", deps.handleGuestSessions)
				staff.Post("/calls/{callId}/answer", deps.handleCallAnswer)
				staff.Post("/calls/{callId}/decline", deps.handleDecline)
			})

			priv.Get("/calls/{callId}", deps.handleCallGet)
			priv.Post("/calls/{callId}/hangup", deps.handleCallHangup)
			priv.Post("/calls/{callId}/legs/{legId}/offer", deps.handleOffer)
			priv.Post("/calls/{callId}/legs/{legId}/answer", deps.handleAnswerSDP)
			priv.Post("/calls/{callId}/legs/{legId}/ice", deps.handleICE)
			priv.Post("/calls/{callId}/legs/{legId}/mute", deps.handleMute)
			priv.Get("/calls/{callId}/turn-credentials", deps.handleTURN)
			priv.Post("/calls/outbound", deps.handleOutbound)
			priv.Post("/calls/{callId}/hold", deps.handleHold)
			priv.Post("/calls/{callId}/transfer", deps.handleTransfer)
			priv.Post("/calls/{callId}/transfer/complete", deps.handleCompleteTransfer)
			priv.Post("/calls/{callId}/video/request", deps.handleVideoRequest)
			priv.Post("/calls/{callId}/video/respond", deps.handleVideoRespond)
			priv.Post("/calls/{callId}/video/downgrade", deps.handleVideoDowngrade)
			priv.Post("/calls/{callId}/screen-share", deps.handleScreenShare)
			priv.Post("/calls/{callId}/conference", deps.handleConference)
			priv.Post("/calls/{callId}/dtmf", deps.handleDTMF)
			priv.Post("/calls/{callId}/wrap-up", deps.handleWrapUp)
		})
	})

	return r
}
