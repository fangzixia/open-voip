package http

import (
	"context"
	"net/http"
	"open-switch/internal/httpapi"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"open-switch/internal/app/http/middleware"
	"open-switch/internal/config"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/store"
)

// SwitchRouterDeps Switch API 依赖。
type SwitchRouterDeps struct {
	Config          config.Config
	Applications    *store.ApplicationRegistry
	Runtime         RuntimeReader
	Events          *store.CallEvents
	Commands        *store.Commands
	Routing         *store.RoutingSessions
	Direct          ports.DirectControlPort
	Admin           ports.CallCenterAdminPort
	BusinessActions interface {
		CompleteBusinessAction(context.Context, string, string, string) error
	}
	RouterDeps
}

// NewSwitchRouter 构建应用隔离的 /switch/v2 路由。
func NewSwitchRouter(deps SwitchRouterDeps) http.Handler {
	r := chi.NewRouter()
	r.NotFound(httpapi.NotFound)
	r.MethodNotAllowed(httpapi.MethodNotAllowed)
	r.Use(chimw.RealIP)
	r.Use(httpapi.Recover)
	r.Use(middleware.RequestID)
	r.Use(chimw.Logger)

	r.Get("/health", handleHealth)

	r.Route("/switch/v2", func(sw chi.Router) {
		sw.With(middleware.RegisterAuth(deps.Config.Integration, deps.Applications)).Post("/integrations/register", deps.handleIntegrationRegister)
		sw.With(middleware.RegisterAuth(deps.Config.Integration, deps.Applications)).Post("/integrations/{applicationId}/rotate-secret", deps.handleIntegrationRotateSecret)

		sw.Group(func(api chi.Router) {
			api.Use(middleware.ApplicationAuth(deps.Applications))
			api.Use(deps.authorizeCallResource)
			api.Post("/configuration/versions", deps.handleConfigStore)
			api.Get("/configuration/versions/{version}", deps.handleConfigGet)
			api.Post("/configuration/versions/{version}/activate", deps.handleConfigActivate)
			api.Post("/config-versions", deps.handleConfigStore)
			api.Get("/config-versions/{version}", deps.handleConfigGet)
			api.Post("/config-versions/{version}/activate", deps.handleConfigActivate)
			api.Post("/agents/{agentId}/check-in", deps.handleAgentCheckIn)
			api.Post("/agents/{agentId}/check-out", deps.handleAgentCheckOut)
			api.Put("/agents/{agentId}/presence", deps.handleAgentPresence)
			api.Get("/agents/{agentId}/session", deps.handleAgentSession)
			api.Get("/queues/{queueId}/status", deps.handleQueueStatus)
			api.Get("/internal/calls/{callId}", deps.handleInternalCall)
			if deps.Events != nil {
				api.Get("/events", deps.handleEvents)
			}
			if deps.Commands != nil {
				api.Get("/commands/{commandId}", deps.handleCommandGet)
			}
			if deps.Routing != nil {
				api.Get("/routing-sessions/{callId}", deps.handleRoutingSession)
			}
			api.Get("/ivr-assets", deps.handleIVRAssets)
			api.Post("/ivr-assets", deps.handleIVRAssetUpload)
			api.Get("/ivr-assets/{assetId}", deps.handleIVRAssetFile)
			api.Get("/internal/recordings/{callId}/{recordingId}", deps.handleRecordingFile)
			api.Delete("/internal/recordings/{callId}/{recordingId}", deps.handleRecordingFile)
			if deps.Runtime != nil {
				api.Get("/internal/calls", deps.handleInternalCalls)
			}

			api.Post("/calls", deps.handleStubCall)
			api.Post("/calls/direct", deps.handleDirectCreate)
			api.Post("/calls/{callId}/legs", deps.handleDirectLeg)
			api.Post("/calls/{callId}/legs/webrtc", deps.handleDirectLeg)
			api.Post("/calls/{callId}/legs/sip", deps.handleDirectSIP)
			api.Delete("/calls/{callId}/legs/{legId}", deps.handleDirectLeave)
			api.Post("/calls/{callId}/bridge", deps.handleDirectBridge)
			api.Post("/calls/{callId}/bridges", deps.handleDirectBridgeAliases)
			api.Put("/calls/{callId}/bridges/{bridgeId}", deps.handlePutBridge)
			api.Delete("/calls/{callId}/bridges/{bridgeId}", deps.handleDeleteBridge)
			api.Post("/calls/{callId}/legs/{legId}/hold", deps.handleLegHold)
			api.Post("/calls/{callId}/legs/{legId}/reject", deps.handleLegReject)
			api.Post("/calls/{callId}/legs/{legId}/playbacks", deps.handleLegPlaybackStart)
			api.Delete("/calls/{callId}/legs/{legId}/playbacks/{playbackId}", deps.handleLegPlaybackStop)
			api.Post("/calls/{callId}/recording/start", deps.handleDirectRecordingStart)
			api.Post("/calls/{callId}/recording/stop", deps.handleDirectRecordingStop)
			api.Post("/calls/{callId}/recordings", deps.handleDirectRecordingStart)
			api.Post("/calls/{callId}/recordings/{recordingId}/stop", deps.handleDirectRecordingStop)
			api.Post("/calls/{callId}/business-actions/{actionId}/complete", deps.handleBusinessActionComplete)
			api.Post("/routing-sessions/{callId}/business-actions/{actionId}/complete", deps.handleBusinessActionComplete)
			if deps.Runtime != nil {
				api.Get("/calls", deps.handleListOpenCalls)
			}
			api.Get("/calls/{callId}", deps.handleCallGet)
			api.Post("/calls/{callId}/hangup", deps.handleCallHangup)
			api.Post("/calls/inbound", deps.handleSwitchInbound)
			api.Post("/calls/outbound", deps.handleOutbound)
			api.Post("/calls/{callId}/answer", deps.handleCallAnswer)
			api.Post("/calls/{callId}/decline", deps.handleDecline)
			api.Post("/calls/{callId}/hold", deps.handleHold)
			api.Post("/calls/{callId}/transfer", deps.handleTransfer)
			api.Post("/calls/{callId}/transfer/complete", deps.handleCompleteTransfer)
			api.Post("/calls/{callId}/conference", deps.handleConference)
			api.Post("/supervisor/calls/{callId}/listen", deps.handleListen)
			api.Post("/supervisor/agents/{agentId}/force-check-out", deps.handleForceCheckout)
			api.Post("/calls/{callId}/video/request", deps.handleVideoRequest)
			api.Post("/calls/{callId}/video/respond", deps.handleVideoRespond)
			api.Post("/calls/{callId}/video/downgrade", deps.handleVideoDowngrade)
			api.Post("/calls/{callId}/screen-share", deps.handleScreenShare)
			api.Post("/calls/{callId}/dtmf", deps.handleDTMF)

			api.Post("/calls/{callId}/legs/{legId}/offer", deps.handleOffer)
			api.Post("/calls/{callId}/legs/{legId}/answer", deps.handleAnswerSDP)
			api.Post("/calls/{callId}/legs/{legId}/ice", deps.handleICE)
			api.Post("/calls/{callId}/legs/{legId}/mute", deps.handleMute)
			api.Get("/calls/{callId}/turn-credentials", deps.handleTURN)
		})
	})

	return r
}

func (d SwitchRouterDeps) handleSwitchInbound(w http.ResponseWriter, r *http.Request) {
	var req dto.InboundRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	id, err := d.CallControl.StartInbound(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	view, err := d.CallControl.GetCall(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusCreated, map[string]string{"call_id": id})
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

// authorizeCallResource 在变更活跃通话前校验资源属于当前应用。
func (d SwitchRouterDeps) authorizeCallResource(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, prefix := range []string{"/switch/v2/calls/", "/switch/v2/routing-sessions/", "/switch/v2/internal/calls/", "/switch/v2/internal/recordings/", "/switch/v2/supervisor/calls/"} {
			if !strings.HasPrefix(r.URL.Path, prefix) {
				continue
			}
			id := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")[0]
			if prefix == "/switch/v2/calls/" && (id == "direct" || id == "inbound" || id == "outbound" || r.Method == http.MethodPost && strings.TrimSuffix(r.URL.Path, "/") == "/switch/v2/calls") {
				break
			}
			if _, err := d.CallControl.GetCall(r.Context(), id); err != nil {
				writeErr(w, err)
				return
			}
			break
		}
		next.ServeHTTP(w, r)
	})
}

// handleBusinessActionComplete 业务系统提交 IVR 业务判断节点的结果分支。
func (d SwitchRouterDeps) handleBusinessActionComplete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	if err := d.BusinessActions.CompleteBusinessAction(r.Context(), chi.URLParam(r, "callId"), chi.URLParam(r, "actionId"), body.Outcome); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
