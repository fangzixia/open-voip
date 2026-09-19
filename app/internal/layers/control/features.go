package control

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
	"open-voip/internal/ports/dto"
)

func (s *Service) Hold(ctx context.Context, callID string, on bool) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		return errs.NotFound("通话不存在")
	}
	if rt.rec.State != stateActive && rt.rec.State != stateHeld {
		return errs.Conflict("当前不能保持", "")
	}
	cust := customerLeg(rt)
	if err := s.deps.Media.SetHold(ctx, callID, cust, on); err != nil {
		return err
	}
	s.mu.Lock()
	rt.held = on
	s.mu.Unlock()
	st := stateActive
	ev := "call.unhold"
	if on {
		st = stateHeld
		ev = "call.hold"
	}
	if err := s.transition(ctx, callID, st); err != nil {
		return err
	}
	return s.publishCall(ctx, callID, ev, rt.offeredAgent, map[string]any{"call_id": callID, "leg_id": cust})
}

func (s *Service) RequestVideo(ctx context.Context, callID, fromLegID string) error {
	s.mu.Lock()
	rt := s.calls[callID]
	if rt != nil {
		rt.videoFromLeg = fromLegID
	}
	s.mu.Unlock()
	if rt == nil || (rt.rec.State != stateActive && rt.rec.State != stateHeld) {
		return errs.NotFound("通话不存在")
	}
	return s.publishCall(ctx, callID, "video.requested", "", map[string]any{"call_id": callID, "from_leg_id": fromLegID})
}

func (s *Service) RespondVideo(ctx context.Context, callID string, accept bool) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		return errs.NotFound("通话不存在")
	}
	if !accept {
		return s.publishCall(ctx, callID, "video.declined", "", map[string]any{"call_id": callID})
	}
	now := time.Now().UTC()
	s.mu.Lock()
	rt.rec.SessionType = dto.SessionTypeMixed
	rt.videoUpgradeOk = true
	rt.videoStartedAt = &now
	from := rt.videoFromLeg
	s.mu.Unlock()
	_ = s.deps.Calls.UpdateCall(ctx, rt.rec)
	_ = s.deps.Media.RequestRenegotiation(ctx, callID, from, true)
	_ = s.cdrUpsert(ctx, callID, "answered")
	return s.publishCall(ctx, callID, "video.accepted", "", map[string]any{"call_id": callID})
}

func (s *Service) DowngradeVideo(ctx context.Context, callID string) error {
	s.mu.Lock()
	rt := s.calls[callID]
	if rt != nil {
		rt.rec.SessionType = dto.SessionTypeAudio
	}
	s.mu.Unlock()
	if rt == nil {
		return errs.NotFound("通话不存在")
	}
	_ = s.deps.Calls.UpdateCall(ctx, rt.rec)
	_ = s.deps.Media.RequestRenegotiation(ctx, callID, "", false)
	_ = s.cdrUpsert(ctx, callID, "answered")
	return s.publishCall(ctx, callID, "video.downgraded", "", map[string]any{"call_id": callID, "session_type": "audio"})
}

func (s *Service) ScreenShare(ctx context.Context, callID, legID string, on bool) error {
	s.mu.Lock()
	rt := s.calls[callID]
	if rt != nil && on {
		rt.screenShares++
	}
	s.mu.Unlock()
	if rt == nil {
		return errs.NotFound("通话不存在")
	}
	ev := "screen_share.stopped"
	if on {
		ev = "screen_share.started"
	}
	_ = s.cdrUpsert(ctx, callID, "answered")
	if on {
		_ = s.deps.Media.RequestRenegotiation(ctx, callID, legID, true)
	}
	return s.publishCall(ctx, callID, ev, "", map[string]any{"call_id": callID, "leg_id": legID})
}

func (s *Service) ConferenceInvite(ctx context.Context, callID, targetAgentID string) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State != stateActive {
		return errs.Conflict("仅通话中可邀请", "")
	}
	info, err := s.deps.Agents.ByID(ctx, targetAgentID)
	if err != nil {
		return err
	}
	if rt.rec.SessionType != dto.SessionTypeAudio && !info.VideoCapable {
		return errs.Unprocessable("目标坐席不具备视频能力", errs.CodeAgentNotVideoCapable)
	}
	if err := s.deps.Agents.SetState(ctx, targetAgentID, "idle", "ringing", "conference:"+callID); err != nil {
		return err
	}
	s.mu.Lock()
	rt.offeredAgent = targetAgentID
	rt.offerAt = time.Now().UTC()
	s.mu.Unlock()
	return s.publishCall(ctx, callID, "call.ringing", targetAgentID, map[string]any{
		"call_id": callID, "session_type": string(rt.rec.SessionType), "caller": rt.caller, "conference": true,
	})
}

func (s *Service) SupervisorListen(ctx context.Context, callID, supervisorAgentID string) (string, error) {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State != stateActive && rt.rec.State != stateHeld {
		return "", errs.NotFound("通话不存在或未接通")
	}
	now := time.Now().UTC()
	leg := ports.CallLegRecord{ID: uuid.New().String(), CallID: callID, Role: dto.LegRoleSupervisor, AgentID: &supervisorAgentID, CreatedAt: now}
	if err := s.deps.Calls.InsertLeg(ctx, leg); err != nil {
		return "", err
	}
	s.mu.Lock()
	rt.legs = append(rt.legs, leg)
	s.mu.Unlock()
	announce := false
	if rt.rec.QueueID != nil {
		if q, err := s.deps.Config.GetQueue(ctx, *rt.rec.QueueID); err == nil {
			announce = q.ListenAnnounce
		}
	}
	if announce {
		_ = s.deps.Media.InjectAudio(ctx, callID, "", dto.AudioSource{FilePath: "", Loop: false})
		_ = s.publishCall(ctx, callID, "recording.notice", "", map[string]any{
			"call_id": callID, "message": "班长已加入监听",
		})
	}
	_ = s.publishCall(ctx, callID, "call.supervisor_listen", supervisorAgentID, map[string]any{
		"call_id": callID, "leg_id": leg.ID, "announced": announce,
	})
	return leg.ID, nil
}

func (s *Service) SendDTMF(ctx context.Context, callID, legID, digit string) error {
	if _, err := s.GetCall(ctx, callID); err != nil {
		return err
	}
	return s.deps.Media.SendDTMF(ctx, callID, legID, dto.DTMFDigit(digit))
}

func (s *Service) ForceReleaseAgent(ctx context.Context, agentID, policy string) error {
	if policy == "" {
		policy = "force_hangup"
	}
	s.mu.Lock()
	var ids []string
	for id, rt := range s.calls {
		if rt.offeredAgent == agentID {
			ids = append(ids, id)
			continue
		}
		for _, l := range rt.legs {
			if l.AgentID != nil && *l.AgentID == agentID {
				ids = append(ids, id)
			}
		}
	}
	s.mu.Unlock()
	if policy != "wait_until_idle" {
		for _, id := range ids {
			_ = s.Hangup(ctx, id, dto.HangupReasonNormal)
		}
	}
	_ = s.deps.Agents.SetState(ctx, agentID, "", "offline", "force-check-out")
	return nil
}

func (s *Service) doTransfer(ctx context.Context, callID string, req dto.TransferRequest) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || (rt.rec.State != stateActive && rt.rec.State != stateHeld) {
		return errs.Conflict("当前不能转接", errs.CodeTransferFailed)
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "blind"
	}
	if mode != "blind" && mode != "consult" {
		return errs.InvalidRequest("转接模式须为 blind 或 consult")
	}
	fromAgent := currentAgent(rt)
	needVideo := rt.rec.SessionType != dto.SessionTypeAudio
	if req.TargetAgentID != "" {
		info, err := s.deps.Agents.ByID(ctx, req.TargetAgentID)
		if err != nil {
			return err
		}
		if needVideo && !info.VideoCapable {
			return errs.Unprocessable("目标坐席不具备视频能力", errs.CodeAgentNotVideoCapable)
		}
		if err := s.deps.Agents.SetState(ctx, req.TargetAgentID, "idle", "ringing", "transfer:"+callID); err != nil {
			return err
		}
		cust := customerLeg(rt)
		if mode == "consult" {
			_ = s.deps.Media.SetHold(ctx, callID, cust, true)
			s.mu.Lock()
			rt.held = true
			rt.consultFrom = fromAgent
			rt.transferMode = "consult"
			s.mu.Unlock()
			_ = s.transition(ctx, callID, stateHeld)
		} else {
			if fromAgent != "" {
				_ = s.deps.Agents.SetState(ctx, fromAgent, "on_call", "acw", "transferred")
				for _, l := range rt.legs {
					if l.AgentID != nil && *l.AgentID == fromAgent {
						_ = s.deps.Media.LeaveRoom(ctx, callID, l.ID)
					}
				}
			}
			s.mu.Lock()
			rt.consultFrom = ""
			rt.transferMode = "blind"
			s.mu.Unlock()
		}
		_ = s.publishCall(ctx, callID, "call.transferring", req.TargetAgentID, map[string]any{
			"call_id": callID, "target_agent_id": req.TargetAgentID, "mode": mode,
		})
		s.mu.Lock()
		rt.offeredAgent = req.TargetAgentID
		rt.offerAt = time.Now().UTC()
		s.mu.Unlock()
		_ = s.transition(ctx, callID, stateRinging)
		slog.Info("转接振铃", "call_id", callID, "mode", mode, "target_agent_id", req.TargetAgentID)
		return s.publishCall(ctx, callID, "call.ringing", req.TargetAgentID, map[string]any{
			"call_id": callID, "queue_name": rt.queueName, "caller": rt.caller, "session_type": string(rt.rec.SessionType),
		})
	}
	if req.TargetQueueID == "" {
		return errs.InvalidRequest("转接目标必填")
	}
	if mode == "consult" {
		return errs.InvalidRequest("咨询转仅支持指定坐席")
	}
	q, err := s.deps.Config.GetQueue(ctx, req.TargetQueueID)
	if err != nil {
		return err
	}
	if needVideo && !q.VideoEnabled {
		return errs.Unprocessable("目标队列不支持视频", errs.CodeAgentNotVideoCapable)
	}
	if fromAgent != "" {
		_ = s.deps.Agents.SetState(ctx, fromAgent, "on_call", "acw", "transferred")
	}
	qid := req.TargetQueueID
	s.mu.Lock()
	rt.rec.QueueID = &qid
	rt.queueName = q.Name
	rt.offeredAgent = ""
	rt.consultFrom = ""
	rt.transferMode = "blind"
	s.mu.Unlock()
	return s.enterQueue(ctx, callID)
}

func (s *Service) completeConsult(ctx context.Context, callID string) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.consultFrom == "" {
		return errs.Conflict("当前不是咨询转", errs.CodeTransferFailed)
	}
	from := rt.consultFrom
	for _, l := range rt.legs {
		if l.AgentID != nil && *l.AgentID == from {
			_ = s.deps.Media.LeaveRoom(ctx, callID, l.ID)
		}
	}
	_ = s.deps.Agents.SetState(ctx, from, "on_call", "acw", "consult_complete")
	cust := customerLeg(rt)
	_ = s.deps.Media.SetHold(ctx, callID, cust, false)
	s.mu.Lock()
	rt.consultFrom = ""
	rt.transferMode = ""
	rt.held = false
	s.mu.Unlock()
	if err := s.transition(ctx, callID, stateActive); err != nil {
		return err
	}
	slog.Info("咨询转完成", "call_id", callID, "from_agent", from)
	return s.publishCall(ctx, callID, "call.transferred", "", map[string]any{"call_id": callID, "from_agent_id": from})
}

func (s *Service) doOutbound(ctx context.Context, req dto.OutboundRequest) (string, error) {
	if req.AgentID == "" || req.Destination == "" {
		return "", errs.InvalidRequest("agent_id 与 destination 必填")
	}
	now := time.Now().UTC()
	callID := uuid.New().String()
	dir := "internal"
	if looksPSTN(req.Destination) {
		dir = "outbound"
	}
	rec := ports.CallRecord{ID: callID, Direction: dir, SessionType: dto.SessionTypeAudio, State: stateCreated, CreatedAt: now, UpdatedAt: now}
	if err := s.deps.Calls.InsertCall(ctx, rec); err != nil {
		return "", err
	}
	from := req.AgentID
	fromLeg := ports.CallLegRecord{ID: uuid.New().String(), CallID: callID, Role: dto.LegRoleAgent, AgentID: &from, CreatedAt: now}
	if err := s.deps.Calls.InsertLeg(ctx, fromLeg); err != nil {
		return "", err
	}
	if err := s.deps.Agents.SetState(ctx, req.AgentID, "idle", "on_call", "outbound"); err != nil {
		return "", err
	}
	info, err := s.deps.Agents.ByID(ctx, req.AgentID)
	caller := req.AgentID
	if err == nil {
		caller = info.Extension
	}
	rt := &runtimeCall{rec: rec, legs: []ports.CallLegRecord{fromLeg}, caller: caller, callee: req.Destination, queuedAt: now, maxWait: 2 * time.Minute}
	t := now
	rt.answeredAt = &t
	s.mu.Lock()
	s.calls[callID] = rt
	s.mu.Unlock()

	if looksPSTN(req.Destination) {
		return s.originateSIPCall(ctx, callID, rt, req)
	}

	target, err := s.deps.Agents.ByExtension(ctx, req.Destination)
	if err != nil {
		rt.rec.Direction = "outbound"
		return s.originateSIPCall(ctx, callID, rt, req)
	}
	if err := s.deps.Agents.SetState(ctx, target.AgentID, "idle", "ringing", "inbound-internal"); err != nil {
		_ = s.Hangup(ctx, callID, dto.HangupReasonError)
		return "", err
	}
	s.mu.Lock()
	rt.offeredAgent = target.AgentID
	rt.offerAt = now
	s.mu.Unlock()
	if err := s.transition(ctx, callID, stateRinging); err != nil {
		return "", err
	}
	_ = s.cdrUpsert(ctx, callID, "queued")
	return callID, s.publishCall(ctx, callID, "call.ringing", target.AgentID, map[string]any{
		"call_id": callID, "caller": caller, "session_type": "audio", "queue_name": "分机互拨",
	})
}

func (s *Service) originateSIPCall(ctx context.Context, callID string, rt *runtimeCall, req dto.OutboundRequest) (string, error) {
	now := time.Now().UTC()
	pstnLeg := ports.CallLegRecord{ID: uuid.New().String(), CallID: callID, Role: dto.LegRolePSTN, CreatedAt: now}
	_ = s.deps.Calls.InsertLeg(ctx, pstnLeg)
	opts := dto.RoomOptions{SessionType: dto.SessionTypeAudio}
	if err := s.deps.Media.CreateRoom(ctx, callID, opts); err != nil {
		return "", err
	}
	if err := s.deps.Media.OriginateSIP(ctx, callID, pstnLeg.ID, req.Destination, req.TrunkID); err != nil {
		_ = s.Hangup(ctx, callID, dto.HangupReasonError)
		return "", err
	}
	s.mu.Lock()
	rt.legs = append(rt.legs, pstnLeg)
	s.mu.Unlock()
	if err := s.transition(ctx, callID, stateActive); err != nil {
		return "", err
	}
	_ = s.cdrUpsert(ctx, callID, "answered")
	return callID, s.publishCall(ctx, callID, "call.answered", req.AgentID, map[string]any{"call_id": callID, "agent_id": req.AgentID})
}

func (s *Service) overflow(ctx context.Context, callID string) bool {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.QueueID == nil {
		return false
	}
	q, err := s.deps.Config.GetQueue(ctx, *rt.rec.QueueID)
	if err != nil {
		return false
	}
	switch q.OverflowAction {
	case "queue":
		if q.OverflowQueueID == "" {
			return false
		}
		nq, err := s.deps.Config.GetQueue(ctx, q.OverflowQueueID)
		if err != nil {
			return false
		}
		oid := q.OverflowQueueID
		s.mu.Lock()
		rt.rec.QueueID = &oid
		rt.queueName = nq.Name
		rt.queuedAt = time.Now().UTC()
		s.mu.Unlock()
		_ = s.deps.Calls.UpdateCall(ctx, rt.rec)
		_ = s.publishCall(ctx, callID, "queue.overflow", "", map[string]any{"call_id": callID, "queue_id": oid})
		s.publishPosition(ctx, callID)
		_ = s.tryDispatch(ctx, callID)
		return true
	case "voicemail":
		s.startVoicemail(ctx, callID)
		return true
	default:
		return false
	}
}

func (s *Service) beginRecordingIfNeeded(ctx context.Context, callID string) {
	s.mu.Lock()
	rt := s.calls[callID]
	if rt == nil || rt.recordingID != "" {
		s.mu.Unlock()
		return
	}
	queueID := deref(rt.rec.QueueID)
	session := rt.rec.SessionType
	s.mu.Unlock()
	if s.deps.RecordingPolicy == nil {
		return
	}
	policy, err := s.deps.RecordingPolicy.ForQueue(ctx, queueID)
	if err != nil {
		return
	}
	if policy.Mode == "video_composite" && session == dto.SessionTypeAudio {
		policy.Mode = "audio"
	}
	s.startRecording(ctx, callID, policy)
	if policy.NotifyGuest && policy.NotifyMessage != "" {
		_ = s.publishCall(ctx, callID, "recording.notice", "", map[string]any{
			"call_id": callID, "message": policy.NotifyMessage,
		})
	}
}

func (s *Service) startRecording(ctx context.Context, callID string, policy dto.RecordingPolicy) {
	if policy.Mode == "" || policy.Mode == "off" {
		return
	}
	s.mu.Lock()
	if rt := s.calls[callID]; rt != nil && rt.recordingID != "" {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	id, err := s.deps.Media.StartRecording(ctx, callID, policy)
	if err != nil || id == "" {
		return
	}
	s.mu.Lock()
	if rt := s.calls[callID]; rt != nil {
		rt.recordingID = id
		rt.notify = policy.NotifyMessage
	}
	s.mu.Unlock()
	if s.deps.Recordings != nil {
		meta, err := s.deps.Media.RecordingInfo(ctx, id)
		if err == nil {
			_ = s.deps.Recordings.Save(ctx, meta)
		} else {
			_ = s.deps.Recordings.Save(ctx, ports.RecordingMeta{ID: id, CallID: callID, MediaType: policy.Mode, StartedAt: time.Now().UTC()})
		}
	}
}

func (s *Service) stopRecording(ctx context.Context, callID string) {
	s.mu.Lock()
	rt := s.calls[callID]
	id := ""
	if rt != nil {
		id = rt.recordingID
	}
	s.mu.Unlock()
	if id == "" {
		return
	}
	_ = s.deps.Media.StopRecording(ctx, id)
	if s.deps.Recordings != nil {
		if meta, err := s.deps.Media.RecordingInfo(ctx, id); err == nil {
			_ = s.deps.Recordings.Save(ctx, meta)
		}
	}
}

func (s *Service) startVoicemail(ctx context.Context, callID string) {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		return
	}
	opts := dto.RoomOptions{SessionType: dto.SessionTypeAudio}
	_ = s.deps.Media.CreateRoom(ctx, callID, opts)
	s.beginRecordingIfNeeded(ctx, callID)
	_ = s.deps.Media.InjectAudio(ctx, callID, "", dto.AudioSource{Loop: false})
	_ = s.publishCall(ctx, callID, "call.voicemail", "", map[string]any{
		"call_id": callID, "message": "请在提示音后留言，结束后将自动挂断",
	})
	slog.Info("进入留言", "call_id", callID)
	time.AfterFunc(25*time.Second, func() {
		_ = s.Hangup(context.Background(), callID, dto.HangupReasonTimeout)
	})
}

func customerLeg(rt *runtimeCall) string {
	for _, l := range rt.legs {
		if l.Role == dto.LegRoleCustomer {
			return l.ID
		}
	}
	if len(rt.legs) > 0 {
		return rt.legs[0].ID
	}
	return ""
}

func currentAgent(rt *runtimeCall) string {
	if rt.offeredAgent != "" {
		return rt.offeredAgent
	}
	for _, l := range rt.legs {
		if l.Role == dto.LegRoleAgent && l.AgentID != nil {
			return *l.AgentID
		}
	}
	return ""
}

func looksPSTN(dest string) bool {
	d := strings.TrimSpace(dest)
	if strings.HasPrefix(d, "+") || strings.HasPrefix(d, "00") {
		return true
	}
	n := 0
	for _, r := range d {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n >= 8
}
