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
	Runtime         RuntimeReader
	Events          *store.CallEvents
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
		sw.Use(middleware.ApplicationAuth(deps.Config.Applications))
		sw.Use(deps.authorizeCallResource)
		sw.Post("/configuration/versions", deps.handleConfigStore)
		sw.Get("/configuration/versions/{version}", deps.handleConfigGet)
		sw.Post("/configuration/versions/{version}/activate", deps.handleConfigActivate)
		sw.Post("/agents/{agentId}/check-in", deps.handleAgentCheckIn)
		sw.Post("/agents/{agentId}/check-out", deps.handleAgentCheckOut)
		sw.Put("/agents/{agentId}/presence", deps.handleAgentPresence)
		sw.Get("/agents/{agentId}/session", deps.handleAgentSession)
		sw.Get("/queues/{queueId}/status", deps.handleQueueStatus)
		sw.Get("/internal/calls/{callId}", deps.handleInternalCall)
		if deps.Events != nil {
			sw.Get("/events", deps.handleEvents)
		}
		sw.Get("/ivr-assets", deps.handleIVRAssets)
		sw.Post("/ivr-assets", deps.handleIVRAssetUpload)
		sw.Get("/ivr-assets/{assetId}", deps.handleIVRAssetFile)
		sw.Get("/internal/recordings/{callId}/{recordingId}", deps.handleRecordingFile)
		sw.Delete("/internal/recordings/{callId}/{recordingId}", deps.handleRecordingFile)
		if deps.Runtime != nil {
			sw.Get("/internal/calls", deps.handleInternalCalls)
		}

		sw.Post("/calls/direct", deps.handleDirectCreate)
		sw.Post("/calls/{callId}/legs", deps.handleDirectLeg)
		sw.Post("/calls/{callId}/legs/sip", deps.handleDirectSIP)
		sw.Delete("/calls/{callId}/legs/{legId}", deps.handleDirectLeave)
		sw.Post("/calls/{callId}/bridge", deps.handleDirectBridge)
		sw.Post("/calls/{callId}/recording/start", deps.handleDirectRecordingStart)
		sw.Post("/calls/{callId}/recording/stop", deps.handleDirectRecordingStop)
		sw.Post("/calls/{callId}/business-actions/{actionId}/complete", deps.handleBusinessActionComplete)
		sw.Get("/calls/{callId}", deps.handleCallGet)
		sw.Post("/calls/{callId}/hangup", deps.handleCallHangup)
		sw.Post("/calls/inbound", deps.handleSwitchInbound)
		sw.Post("/calls/outbound", deps.handleOutbound)
		sw.Post("/calls/{callId}/answer", deps.handleCallAnswer)
		sw.Post("/calls/{callId}/decline", deps.handleDecline)
		sw.Post("/calls/{callId}/hold", deps.handleHold)
		sw.Post("/calls/{callId}/transfer", deps.handleTransfer)
		sw.Post("/calls/{callId}/transfer/complete", deps.handleCompleteTransfer)
		sw.Post("/calls/{callId}/conference", deps.handleConference)
		sw.Post("/supervisor/calls/{callId}/listen", deps.handleListen)
		sw.Post("/supervisor/agents/{agentId}/force-check-out", deps.handleForceCheckout)
		sw.Post("/calls/{callId}/video/request", deps.handleVideoRequest)
		sw.Post("/calls/{callId}/video/respond", deps.handleVideoRespond)
		sw.Post("/calls/{callId}/video/downgrade", deps.handleVideoDowngrade)
		sw.Post("/calls/{callId}/screen-share", deps.handleScreenShare)
		sw.Post("/calls/{callId}/dtmf", deps.handleDTMF)

		sw.Post("/calls/{callId}/legs/{legId}/offer", deps.handleOffer)
		sw.Post("/calls/{callId}/legs/{legId}/answer", deps.handleAnswerSDP)
		sw.Post("/calls/{callId}/legs/{legId}/ice", deps.handleICE)
		sw.Post("/calls/{callId}/legs/{legId}/mute", deps.handleMute)
		sw.Get("/calls/{callId}/turn-credentials", deps.handleTURN)
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

// authorizeCallResource checks ownership before any handler can mutate a live call.
func (d SwitchRouterDeps) authorizeCallResource(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, prefix := range []string{"/switch/v2/calls/", "/switch/v2/internal/calls/", "/switch/v2/internal/recordings/", "/switch/v2/supervisor/calls/"} {
			if !strings.HasPrefix(r.URL.Path, prefix) {
				continue
			}
			id := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")[0]
			if prefix == "/switch/v2/calls/" && (id == "direct" || id == "inbound" || id == "outbound") {
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
