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
	Commands        *store.Commands
	Routing         *store.RoutingSessions
	Direct          ports.DirectControlPort
	Admin           ports.CallCenterAdminPort
	BusinessActions interface {
		CompleteBusinessAction(context.Context, string, string, string) error
	}
	RouterDeps
}

// NewSwitchRouter 构建 /switch/v1 路由（无鉴权，依赖内网隔离）。
func NewSwitchRouter(deps SwitchRouterDeps) http.Handler {
	r := chi.NewRouter()
	r.NotFound(httpapi.NotFound)
	r.MethodNotAllowed(httpapi.MethodNotAllowed)
	r.Use(chimw.RealIP)
	r.Use(httpapi.Recover)
	r.Use(middleware.RequestID)
	r.Use(chimw.Logger)

	r.Get("/health", handleHealth)

	r.Route("/switch/v1", func(sw chi.Router) {
		sw.Group(func(api chi.Router) {
			api.Use(deps.authorizeCallResource)
			deps.registerSwitchAPI(api)
		})
	})

	return r
}

func (d SwitchRouterDeps) registerSwitchAPI(api chi.Router) {
	api.Post("/configuration/versions", d.handleConfigStore)
	api.Get("/configuration/versions/{version}", d.handleConfigGet)
	api.Post("/configuration/versions/{version}/activate", d.handleConfigActivate)
	api.Get("/configuration/active", d.handleConfigActive)
	api.Get("/configuration/active/summary", d.handleConfigActiveSummary)
	api.Post("/config-versions", d.handleConfigStore)
	api.Get("/config-versions/{version}", d.handleConfigGet)
	api.Post("/config-versions/{version}/activate", d.handleConfigActivate)
	api.Get("/config-versions/active", d.handleConfigActive)
	api.Get("/config-versions/active/summary", d.handleConfigActiveSummary)

	api.Get("/queues", d.handleQueueConfigList)
	api.Post("/queues", d.handleQueueConfigCreate)
	api.Get("/queues/{queueId}/config", d.handleQueueConfigGet)
	api.Patch("/queues/{queueId}/config", d.handleQueueConfigPatch)
	api.Delete("/queues/{queueId}/config", d.handleQueueConfigDelete)
	api.Put("/queues/{queueId}/agents", d.handleQueueConfigAgents)
	api.Put("/queues/{queueId}/skills", d.handleQueueConfigSkills)

	api.Get("/skills", d.handleSkillConfigList)
	api.Post("/skills", d.handleSkillConfigCreate)
	api.Patch("/skills/{skillId}", d.handleSkillConfigPatch)
	api.Delete("/skills/{skillId}", d.handleSkillConfigDelete)

	api.Get("/agents/config", d.handleAgentConfigList)
	api.Put("/agents/{agentId}/config", d.handleAgentConfigUpsert)
	api.Delete("/agents/{agentId}/config", d.handleAgentConfigDelete)
	api.Put("/agents/{agentId}/skills", d.handleAgentConfigSkills)

	api.Get("/did-routes", d.handleDIDConfigList)
	api.Post("/did-routes", d.handleDIDConfigUpsert)
	api.Patch("/did-routes/{didId}", d.handleDIDConfigPatch)
	api.Delete("/did-routes/{didId}", d.handleDIDConfigDelete)

	api.Get("/ivr/flows", d.handleIVRFlowList)
	api.Get("/ivr/flows/{flowId}", d.handleIVRFlowGet)
	api.Put("/ivr/flows/{flowId}", d.handleIVRFlowUpsert)
	api.Post("/ivr/flows", d.handleIVRFlowCreate)
	api.Delete("/ivr/flows/{flowId}", d.handleIVRFlowDelete)
	api.Post("/agents/{agentId}/check-in", d.handleAgentCheckIn)
	api.Post("/agents/{agentId}/check-out", d.handleAgentCheckOut)
	api.Put("/agents/{agentId}/presence", d.handleAgentPresence)
	api.Get("/agents/{agentId}/session", d.handleAgentSession)
	api.Get("/queues/{queueId}/status", d.handleQueueStatus)
	api.Get("/internal/calls/{callId}", d.handleInternalCall)
	if d.Events != nil {
		api.Get("/events", d.handleEvents)
	}
	if d.Commands != nil {
		api.Get("/commands/{commandId}", d.handleCommandGet)
	}
	if d.Routing != nil {
		api.Get("/routing-sessions/{callId}", d.handleRoutingSession)
	}
	api.Get("/ivr-assets", d.handleIVRAssets)
	api.Post("/ivr-assets", d.handleIVRAssetUpload)
	api.Get("/ivr-assets/{assetId}", d.handleIVRAssetFile)
	api.Get("/internal/recordings/{callId}/{recordingId}", d.handleRecordingFile)
	api.Delete("/internal/recordings/{callId}/{recordingId}", d.handleRecordingFile)
	if d.Runtime != nil {
		api.Get("/internal/calls", d.handleInternalCalls)
	}

	api.Post("/calls", d.handleStubCall)
	api.Post("/calls/direct", d.handleDirectCreate)
	api.Post("/calls/{callId}/legs", d.handleDirectLeg)
	api.Post("/calls/{callId}/legs/webrtc", d.handleDirectLeg)
	api.Post("/calls/{callId}/legs/sip", d.handleDirectSIP)
	api.Delete("/calls/{callId}/legs/{legId}", d.handleDirectLeave)
	api.Post("/calls/{callId}/bridge", d.handleDirectBridge)
	api.Post("/calls/{callId}/bridges", d.handleDirectBridgeAliases)
	api.Put("/calls/{callId}/bridges/{bridgeId}", d.handlePutBridge)
	api.Delete("/calls/{callId}/bridges/{bridgeId}", d.handleDeleteBridge)
	api.Post("/calls/{callId}/legs/{legId}/hold", d.handleLegHold)
	api.Post("/calls/{callId}/legs/{legId}/reject", d.handleLegReject)
	api.Post("/calls/{callId}/legs/{legId}/playbacks", d.handleLegPlaybackStart)
	api.Delete("/calls/{callId}/legs/{legId}/playbacks/{playbackId}", d.handleLegPlaybackStop)
	api.Post("/calls/{callId}/recording/start", d.handleDirectRecordingStart)
	api.Post("/calls/{callId}/recording/stop", d.handleDirectRecordingStop)
	api.Post("/calls/{callId}/recordings", d.handleDirectRecordingStart)
	api.Post("/calls/{callId}/recordings/{recordingId}/stop", d.handleDirectRecordingStop)
	api.Post("/calls/{callId}/business-actions/{actionId}/complete", d.handleBusinessActionComplete)
	api.Post("/routing-sessions/{callId}/business-actions/{actionId}/complete", d.handleBusinessActionComplete)
	if d.Runtime != nil {
		api.Get("/calls", d.handleListOpenCalls)
	}
	api.Get("/calls/{callId}", d.handleCallGet)
	api.Post("/calls/{callId}/hangup", d.handleCallHangup)
	api.Post("/calls/inbound", d.handleSwitchInbound)
	api.Post("/calls/outbound", d.handleOutbound)
	api.Post("/calls/{callId}/answer", d.handleCallAnswer)
	api.Post("/calls/{callId}/decline", d.handleDecline)
	api.Post("/calls/{callId}/hold", d.handleHold)
	api.Post("/calls/{callId}/transfer", d.handleTransfer)
	api.Post("/calls/{callId}/transfer/complete", d.handleCompleteTransfer)
	api.Post("/calls/{callId}/conference", d.handleConference)
	api.Post("/supervisor/calls/{callId}/listen", d.handleListen)
	api.Post("/supervisor/agents/{agentId}/force-check-out", d.handleForceCheckout)
	api.Post("/calls/{callId}/video/request", d.handleVideoRequest)
	api.Post("/calls/{callId}/video/respond", d.handleVideoRespond)
	api.Post("/calls/{callId}/video/downgrade", d.handleVideoDowngrade)
	api.Post("/calls/{callId}/screen-share", d.handleScreenShare)
	api.Post("/calls/{callId}/dtmf", d.handleDTMF)

	api.Post("/calls/{callId}/legs/{legId}/offer", d.handleOffer)
	api.Post("/calls/{callId}/legs/{legId}/answer", d.handleAnswerSDP)
	api.Post("/calls/{callId}/legs/{legId}/ice", d.handleICE)
	api.Post("/calls/{callId}/legs/{legId}/mute", d.handleMute)
	api.Get("/calls/{callId}/turn-credentials", d.handleTURN)
}

func (d SwitchRouterDeps) handleSwitchInbound(w http.ResponseWriter, r *http.Request) {
	var req dto.InboundRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	// config_version / 作用域由 Switch 从激活配置或队列解析，拒绝客户端伪造。
	req.ConfigVersion = 0
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

// authorizeCallResource 在变更活跃通话前校验资源存在（无鉴权设计下仅作存在性检查）。
func (d SwitchRouterDeps) authorizeCallResource(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const base = "/switch/v1"
		path := r.URL.Path
		for _, suffix := range []string{"/calls/", "/routing-sessions/", "/internal/calls/", "/internal/recordings/", "/supervisor/calls/"} {
			prefix := base + suffix
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			id := strings.Split(strings.TrimPrefix(path, prefix), "/")[0]
			if suffix == "/calls/" && (id == "direct" || id == "inbound" || id == "outbound" || (r.Method == http.MethodPost && strings.TrimSuffix(path, "/") == base+"/calls")) {
				next.ServeHTTP(w, r)
				return
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
