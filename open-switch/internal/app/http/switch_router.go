package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"open-switch/internal/app/http/middleware"
	"open-switch/internal/config"
	"open-switch/internal/ports/dto"
)

// SwitchRouterDeps Switch API 依赖。
type SwitchRouterDeps struct {
	Config  config.Config
	Runtime RuntimeReader
	RouterDeps
}

// NewSwitchRouter 构建 /switch/v1 路由（见 docs/open-switch对接说明.md）。
func NewSwitchRouter(deps SwitchRouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(chimw.Logger)

	r.Get("/health", handleHealth)

	r.Route("/switch/v1", func(sw chi.Router) {
		sw.Use(middleware.IntegrationAuth(deps.Config.Integration.Secret))
		sw.Get("/internal/calls/{callId}", deps.handleInternalCall)
		sw.Get("/internal/recordings/{callId}/{recordingId}", deps.handleRecordingFile)
		sw.Delete("/internal/recordings/{callId}/{recordingId}", deps.handleRecordingFile)
		if deps.Runtime != nil {
			sw.Get("/internal/calls", deps.handleInternalCalls)
		}

		sw.Post("/calls/inbound", deps.handleSwitchInbound)
		sw.Get("/calls/{callId}", deps.handleCallGet)
		sw.Post("/calls/outbound", deps.handleOutbound)
		sw.Post("/calls/{callId}/answer", deps.handleCallAnswer)
		sw.Post("/calls/{callId}/decline", deps.handleDecline)
		sw.Post("/calls/{callId}/hangup", deps.handleCallHangup)
		sw.Post("/calls/{callId}/hold", deps.handleHold)
		sw.Post("/calls/{callId}/transfer", deps.handleTransfer)
		sw.Post("/calls/{callId}/transfer/complete", deps.handleCompleteTransfer)
		sw.Post("/calls/{callId}/video/request", deps.handleVideoRequest)
		sw.Post("/calls/{callId}/video/respond", deps.handleVideoRespond)
		sw.Post("/calls/{callId}/video/downgrade", deps.handleVideoDowngrade)
		sw.Post("/calls/{callId}/screen-share", deps.handleScreenShare)
		sw.Post("/calls/{callId}/conference", deps.handleConference)
		sw.Post("/calls/{callId}/dtmf", deps.handleDTMF)
		sw.Post("/supervisor/calls/{callId}/listen", deps.handleListen)
		sw.Post("/supervisor/agents/{agentId}/force-check-out", deps.handleForceCheckout)

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
