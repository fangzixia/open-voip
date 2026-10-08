package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
)

// CreateDirect 由控制器直接创建通话，不查询业务平台 API。
// 返回的第一条腿供 WebRTC 协商或 SIP 入站使用。
func (s *Service) CreateDirect(ctx context.Context, req dto.DirectCallRequest) (ports.CallView, error) {
	if req.CallID == "" {
		req.CallID = uuid.New().String()
	}
	if _, err := uuid.Parse(req.CallID); err != nil {
		return ports.CallView{}, errs.InvalidRequest("call_id 必须为 UUID")
	}
	if req.Direction == "" {
		req.Direction = "inbound"
	}
	if req.Direction != "inbound" && req.Direction != "outbound" && req.Direction != "internal" {
		return ports.CallView{}, errs.InvalidRequest("direction 无效")
	}
	if req.SessionType == "" {
		req.SessionType = dto.SessionTypeAudio
	}
	if req.SessionType != dto.SessionTypeAudio && req.SessionType != dto.SessionTypeVideo {
		return ports.CallView{}, errs.InvalidRequest("session_type 无效")
	}
	if req.InitialLegRole == "" {
		req.InitialLegRole = dto.LegRoleCustomer
	}
	if req.InitialLegRole != dto.LegRoleCustomer && req.InitialLegRole != dto.LegRoleAgent && req.InitialLegRole != dto.LegRolePSTN {
		return ports.CallView{}, errs.InvalidRequest("initial_leg_role 无效")
	}
	ctx, unlock := s.command(ctx, req.CallID)
	defer unlock()
	if existing, err := s.deps.Calls.GetCall(ctx, req.CallID); err == nil {
		if existing.QueueID != nil || existing.Direction != req.Direction || existing.Caller != req.Caller || existing.Callee != req.Callee || existing.SessionType != req.SessionType {
			return ports.CallView{}, errs.Conflict("call_id 已用于其他通话", "")
		}
		legs, err := s.deps.Calls.ListLegs(ctx, req.CallID)
		if err != nil {
			return ports.CallView{}, err
		}
		if len(legs) == 0 || legs[0].Role != req.InitialLegRole || deref(legs[0].AgentID) != req.AgentID {
			return ports.CallView{}, errs.Conflict("call_id 已用于其他通话", "")
		}
		return s.GetCall(ctx, req.CallID)
	} else if !errors.Is(err, errs.ErrNotFound) {
		return ports.CallView{}, err
	}
	// 语音房间自创建起使用 PCMU，以便在 WebRTC 协商后追加出局 SIP 腿，
	// 且不会在通话中途静默切换编解码。
	if req.SessionType == dto.SessionTypeAudio {
		s.deps.Media.PrepareSIP(req.CallID)
	}
	if err := s.deps.Media.CreateRoom(ctx, req.CallID, dto.RoomOptions{SessionType: req.SessionType, Direct: true}); err != nil {
		return ports.CallView{}, err
	}
	now := time.Now().UTC()
	var cfgVer *int64
	if s.deps.Config != nil {
		if v, err := s.deps.Config.ActiveVersion(ctx); err == nil && v > 0 {
			cfgVer = &v
			ctx = scope.WithConfigVersion(ctx, v)
		}
	}
	rec := ports.CallRecord{Version: 1, ConfigVersion: cfgVer, ID: req.CallID, Direction: req.Direction, SessionType: req.SessionType, State: stateCreated, Caller: req.Caller, Callee: req.Callee, CreatedAt: now, UpdatedAt: now}
	leg := ports.CallLegRecord{ID: uuid.New().String(), CallID: req.CallID, Role: req.InitialLegRole, CreatedAt: now}
	if req.AgentID != "" {
		leg.AgentID = &req.AgentID
	}
	err := s.deps.Calls.InsertCallWithLeg(ctx, rec, leg)
	if err != nil {
		_ = s.deps.Media.CloseRoom(ctx, req.CallID)
		return ports.CallView{}, err
	}
	rt := &runtimeCall{rec: rec, legs: []ports.CallLegRecord{leg}, caller: req.Caller, callee: req.Callee}
	s.mu.Lock()
	s.calls[req.CallID] = rt
	s.mu.Unlock()
	return toView(rt), nil
}

// CreateStubCall 创建无媒体腿的空通话，供后续按方案加腿与桥接。
func (s *Service) CreateStubCall(ctx context.Context, req dto.StubCallRequest) (ports.CallView, error) {
	if req.CallID == "" {
		req.CallID = uuid.New().String()
	}
	if _, err := uuid.Parse(req.CallID); err != nil {
		return ports.CallView{}, errs.InvalidRequest("call_id 必须为 UUID")
	}
	ctx, unlock := s.command(ctx, req.CallID)
	defer unlock()
	if existing, err := s.deps.Calls.GetCall(ctx, req.CallID); err == nil {
		return s.GetCall(ctx, existing.ID)
	} else if !errors.Is(err, errs.ErrNotFound) {
		return ports.CallView{}, err
	}
	meta := ""
	if len(req.Metadata) > 0 {
		raw, err := json.Marshal(req.Metadata)
		if err != nil {
			return ports.CallView{}, errs.InvalidRequest("metadata 无效")
		}
		meta = string(raw)
	}
	now := time.Now().UTC()
	var cfgVer *int64
	if s.deps.Config != nil {
		if v, err := s.deps.Config.ActiveVersion(ctx); err == nil && v > 0 {
			cfgVer = &v
			ctx = scope.WithConfigVersion(ctx, v)
		}
	}
	rec := ports.CallRecord{
		Version: 1, ConfigVersion: cfgVer, ID: req.CallID, BusinessRef: req.BusinessRef, Metadata: meta,
		Direction: "internal", SessionType: dto.SessionTypeAudio, State: stateCreated, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.deps.Calls.InsertCall(ctx, rec); err != nil {
		return ports.CallView{}, err
	}
	rt := &runtimeCall{rec: rec}
	s.mu.Lock()
	s.calls[req.CallID] = rt
	s.mu.Unlock()
	return toView(rt), nil
}

func (s *Service) AddDirectLeg(ctx context.Context, callID string, req dto.DirectLegRequest) (ports.CallView, error) {
	if req.Role != dto.LegRoleCustomer && req.Role != dto.LegRoleAgent && req.Role != dto.LegRoleSupervisor && req.Role != dto.LegRoleApplication {
		return ports.CallView{}, errs.InvalidRequest("role 只支持 customer、agent、supervisor、application")
	}
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State == stateEnded || rt.rec.QueueID != nil {
		return ports.CallView{}, errs.Conflict("通话不可添加直接控制腿", "")
	}
	if len(rt.legs) >= 2 {
		return ports.CallView{}, errs.Conflict("直接控制通话最多支持两个媒体腿", "")
	}
	leg := ports.CallLegRecord{ID: uuid.New().String(), CallID: callID, Role: req.Role, CreatedAt: time.Now().UTC()}
	if req.AgentID != "" {
		leg.AgentID = &req.AgentID
	}
	if err := s.deps.Calls.InsertLeg(ctx, leg); err != nil {
		return ports.CallView{}, err
	}
	s.mu.Lock()
	rt.legs = append(rt.legs, leg)
	view := toView(rt)
	s.mu.Unlock()
	_ = s.publishCall(ctx, callID, "leg.added", req.AgentID, map[string]any{"call_id": callID, "leg_id": leg.ID, "role": req.Role})
	return view, nil
}

func (s *Service) DialDirectSIP(ctx context.Context, callID string, req dto.DirectSIPRequest) (ports.DirectSIPResult, error) {
	if req.Destination == "" {
		return ports.DirectSIPResult{}, errs.InvalidRequest("destination 必填")
	}
	if req.TrunkID == "" && req.RouteGroupID != "" {
		req.TrunkID = req.RouteGroupID
	}
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State == stateEnded || rt.rec.QueueID != nil {
		return ports.DirectSIPResult{}, errs.Conflict("通话不可拨号", "")
	}
	requestHash := sipDialRequestHash(req)
	legID := uuid.New().String()
	if key := scope.Idempotency(ctx); key != "" {
		legID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(callID+"|dial|"+key)).String()
		if rt.directDialHashes != nil {
			if old, ok := rt.directDialHashes[key]; ok && old != requestHash {
				return ports.DirectSIPResult{}, errs.Conflict("幂等键已用于其他拨号请求", "")
			}
		}
		for _, l := range rt.legs {
			if l.ID == legID {
				return ports.DirectSIPResult{View: toView(rt), LegID: legID}, nil
			}
		}
	}
	if len(rt.legs) >= 2 {
		return ports.DirectSIPResult{}, errs.Conflict("直接控制通话最多支持两个媒体腿", "")
	}
	if rt.rec.SessionType != dto.SessionTypeAudio {
		return ports.DirectSIPResult{}, errs.Unprocessable("SIP 腿仅支持语音", "")
	}
	if err := s.deps.Media.PreflightOriginateSIP(ctx, req.Destination, req.TrunkID); err != nil {
		return ports.DirectSIPResult{}, err
	}
	cmdID := ""
	if s.deps.Commands != nil {
		cmd, reused, err := s.deps.Commands.Accept(ctx, callID, scope.Idempotency(ctx), sipDialRequestHash(req), "sip.dial", map[string]any{
			"call_id": callID, "leg_id": legID, "destination": req.Destination, "trunk_id": req.TrunkID,
		})
		if err != nil {
			return ports.DirectSIPResult{}, err
		}
		cmdID = cmd.ID
		if reused {
			return ports.DirectSIPResult{}, errs.Conflict("拨号命令已被领取或此前失败，请查询原通话状态", "")
		}

		if err := s.deps.Commands.MarkRunning(ctx, cmdID); err != nil {
			return ports.DirectSIPResult{}, err
		}
	}
	s.deps.Media.PrepareSIP(callID)
	if err := s.deps.Media.CreateRoom(ctx, callID, dto.RoomOptions{SessionType: dto.SessionTypeAudio}); err != nil {
		return ports.DirectSIPResult{}, err
	}
	leg := ports.CallLegRecord{ID: legID, CallID: callID, Role: dto.LegRolePSTN, CreatedAt: time.Now().UTC()}
	if err := s.deps.Calls.InsertLeg(ctx, leg); err != nil {
		return ports.DirectSIPResult{}, err
	}
	s.mu.Lock()
	rt.legs = append(rt.legs, leg)
	view := toView(rt)
	s.mu.Unlock()
	if key := scope.Idempotency(ctx); key != "" {
		if rt.directDialHashes == nil {
			rt.directDialHashes = map[string]string{}
		}
		rt.directDialHashes[key] = requestHash
	}
	s.emitCall(ctx, callID, "leg.dialing", "", map[string]any{"call_id": callID, "leg_id": leg.ID, "destination": req.Destination, "command_id": cmdID})
	if len(rt.legs) == 1 {
		s.mu.Lock()
		rt.rec.Direction = "outbound"
		rt.callee = req.Destination
		s.mu.Unlock()
		if err := s.transition(ctx, callID, stateRinging); err != nil {
			return ports.DirectSIPResult{}, err
		}
	}
	go func(commandID string) {
		dialCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		err := s.deps.Media.OriginateSIP(dialCtx, callID, leg.ID, req.Destination, req.TrunkID)
		lockedCtx, release := s.command(context.Background(), callID)
		defer release()
		s.mu.Lock()
		current := s.calls[callID]
		alive := current != nil && current.rec.State != stateEnded
		s.mu.Unlock()
		if !alive {
			_ = s.deps.Media.LeaveRoom(lockedCtx, callID, leg.ID)
			if s.deps.Commands != nil && commandID != "" {
				_ = s.deps.Commands.Complete(lockedCtx, commandID, "failed", "call_ended", map[string]any{"leg_id": leg.ID})
			}
			return
		}
		if err != nil {
			if len(current.legs) == 1 {
				_ = s.hangupWithFailure(lockedCtx, callID, dto.HangupReasonError, err)
			}
			msg, code := failureFromErr(err)
			s.emitCall(lockedCtx, callID, "leg.failed", "", map[string]any{
				"call_id": callID, "leg_id": leg.ID, "error": err.Error(),
				"message": msg, "error_code": code,
			})
			if s.deps.Commands != nil && commandID != "" {
				_ = s.deps.Commands.Complete(lockedCtx, commandID, "failed", "dial_failed", map[string]any{"leg_id": leg.ID, "error": err.Error()})
			}
			return
		}
		if len(current.legs) == 1 {
			s.mu.Lock()
			current.answeredAt = new(time.Now().UTC())
			current.callee = req.Destination
			s.mu.Unlock()
			if err := s.transition(lockedCtx, callID, stateActive); err != nil {
				_ = s.hangupWithFailure(lockedCtx, callID, dto.HangupReasonError, err)
				return
			}
			_ = s.cdrUpsert(lockedCtx, callID, "answered")
		}
		s.emitCall(lockedCtx, callID, "leg.connected", "", map[string]any{"call_id": callID, "leg_id": leg.ID, "type": "sip"})
		s.emitCall(lockedCtx, callID, "leg.answered", "", map[string]any{"call_id": callID, "leg_id": leg.ID})
		if s.deps.Commands != nil && commandID != "" {
			_ = s.deps.Commands.Complete(lockedCtx, commandID, "succeeded", "", map[string]any{"leg_id": leg.ID})
		}
	}(cmdID)
	return ports.DirectSIPResult{View: view, CommandID: cmdID, LegID: leg.ID}, nil
}

func sipDialRequestHash(req dto.DirectSIPRequest) string {
	sum := sha256.Sum256([]byte(req.Destination + "|" + req.TrunkID + "|" + req.RouteGroupID))
	return hex.EncodeToString(sum[:])
}

func (s *Service) BridgeDirect(ctx context.Context, callID, a, b string) error {
	if a == "" || b == "" || a == b {
		return errs.InvalidRequest("需要两个不同的 leg_id")
	}
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if err := s.guardExpectedVersion(ctx, callID); err != nil {
		return err
	}
	hash := "bridge|" + a + "|" + b
	return s.runIdempotentMutation(ctx, callID, "bridge", hash, func() error {
		return s.bridgeDirectOnce(ctx, callID, a, b)
	})
}

func (s *Service) bridgeDirectOnce(ctx context.Context, callID, a, b string) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State == stateEnded || rt.rec.QueueID != nil {
		return errs.Conflict("通话不可桥接", "")
	}
	if _, err := s.deps.Calls.GetLeg(ctx, callID, a); err != nil {
		return err
	}
	if _, err := s.deps.Calls.GetLeg(ctx, callID, b); err != nil {
		return err
	}
	if err := s.deps.Media.BridgeLegs(ctx, callID, a, b); err != nil {
		return err
	}
	if rt.answeredAt == nil {
		now := time.Now().UTC()
		rt.answeredAt = &now
	}
	if err := s.transition(ctx, callID, stateActive); err != nil {
		s.deps.Media.UnbridgeLegs(callID)
		return err
	}
	bridgePayload := map[string]any{"call_id": callID, "leg_a": a, "leg_b": b}
	if s.deps.Bridges != nil {
		bridgeID, err := s.deps.Bridges.ActivatePair(ctx, callID, a, b)
		if err != nil {
			return err
		}
		if bridgeID != "" {
			bridgePayload["bridge_id"] = bridgeID
		}
	}
	if err := s.publishCall(ctx, callID, "bridge.active", "", bridgePayload); err != nil {
		return err
	}
	return s.publishCall(ctx, callID, "call.answered", "", bridgePayload)
}

func (s *Service) ReplaceBridge(ctx context.Context, callID, bridgeID, legA, legB string) error {
	if legA == "" || legB == "" || legA == legB {
		return errs.InvalidRequest("需要两个不同的 leg_id")
	}
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if err := s.guardExpectedVersion(ctx, callID); err != nil {
		return err
	}
	hash := "bridge.replace|" + bridgeID + "|" + legA + "|" + legB
	return s.runIdempotentMutation(ctx, callID, "bridge.replace", hash, func() error {
		s.mu.Lock()
		rt := s.calls[callID]
		s.mu.Unlock()
		if rt == nil || rt.rec.State == stateEnded {
			return errs.Conflict("通话不可桥接", "")
		}
		if _, err := s.deps.Calls.GetLeg(ctx, callID, legA); err != nil {
			return err
		}
		if _, err := s.deps.Calls.GetLeg(ctx, callID, legB); err != nil {
			return err
		}
		if s.deps.Bridges == nil {
			return errs.ErrNotImplemented
		}
		if err := s.deps.Bridges.ReplacePair(ctx, callID, bridgeID, legA, legB); err != nil {
			return err
		}
		s.deps.Media.UnbridgeLegs(callID)
		if err := s.deps.Media.BridgeLegs(ctx, callID, legA, legB); err != nil {
			return err
		}
		payload := map[string]any{"call_id": callID, "bridge_id": bridgeID, "leg_a": legA, "leg_b": legB}
		return s.publishCall(ctx, callID, "bridge.updated", "", payload)
	})
}

func (s *Service) EndBridge(ctx context.Context, callID, bridgeID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if s.deps.Bridges == nil {
		return errs.ErrNotImplemented
	}
	if err := s.deps.Bridges.EndBridge(ctx, callID, bridgeID); err != nil {
		return err
	}
	s.emitCall(ctx, callID, "bridge.ended", "", map[string]any{"call_id": callID, "bridge_id": bridgeID})
	return nil
}

func (s *Service) LeaveDirectLeg(ctx context.Context, callID, legID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	hash := "leg.leave|" + legID
	return s.runIdempotentMutation(ctx, callID, "leg.leave", hash, func() error {
		return s.leaveDirectLegOnce(ctx, callID, legID)
	})
}

func (s *Service) leaveDirectLegOnce(ctx context.Context, callID, legID string) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State == stateEnded || rt.rec.QueueID != nil {
		return errs.Conflict("通话不可移除媒体腿", "")
	}
	if _, err := s.deps.Calls.GetLeg(ctx, callID, legID); err != nil {
		return err
	}
	if err := s.deps.Media.LeaveRoom(ctx, callID, legID); err != nil {
		return err
	}
	if err := s.deps.Calls.DeleteLeg(ctx, callID, legID); err != nil {
		return err
	}
	s.mu.Lock()
	for i, leg := range rt.legs {
		if leg.ID == legID {
			rt.legs = append(rt.legs[:i], rt.legs[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	return s.publishCall(ctx, callID, "leg.left", "", map[string]any{"call_id": callID, "leg_id": legID})
}

func (s *Service) StartDirectRecording(ctx context.Context, callID string, policy dto.RecordingPolicy) (ports.RecordingMeta, error) {
	if policy.Mode != "audio" && policy.Mode != "video_composite" {
		return ports.RecordingMeta{}, errs.InvalidRequest("mode 无效")
	}
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State == stateEnded || rt.rec.QueueID != nil {
		return ports.RecordingMeta{}, errs.Conflict("通话不可录制", "")
	}
	if rt.recordingID != "" {
		return s.deps.Media.RecordingInfo(ctx, rt.recordingID)
	}
	id, err := s.deps.Media.StartRecording(ctx, callID, policy)
	if err != nil {
		return ports.RecordingMeta{}, err
	}
	meta, err := s.deps.Media.RecordingInfo(ctx, id)
	if err != nil {
		return ports.RecordingMeta{}, err
	}
	s.mu.Lock()
	rt.recordingID = id
	s.mu.Unlock()
	_ = s.publishCall(ctx, callID, "recording.started", "", map[string]any{"call_id": callID, "recording_id": id, "media_type": meta.MediaType})
	return meta, nil
}

func (s *Service) StopDirectRecording(ctx context.Context, callID string) (ports.RecordingMeta, error) {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.QueueID != nil || rt.recordingID == "" {
		return ports.RecordingMeta{}, errs.NotFound("录音不存在")
	}
	id := rt.recordingID
	if err := s.stopRecording(ctx, callID); err != nil {
		return ports.RecordingMeta{}, err
	}
	meta, err := s.deps.Media.RecordingInfo(ctx, id)
	if err != nil {
		return ports.RecordingMeta{}, err
	}
	s.mu.Lock()
	rt.recordingID = ""
	s.mu.Unlock()
	payload := map[string]any{"call_id": callID, "recording_id": id, "file_path": meta.FilePath, "file_size": meta.FileSize}
	if meta.SampleRateHz > 0 {
		payload["sample_rate_hz"] = meta.SampleRateHz
	}
	if len(meta.LegPaths) > 0 {
		payload["leg_paths"] = meta.LegPaths
	}
	_ = s.publishCall(ctx, callID, "recording.stopped", "", payload)
	return meta, nil
}
