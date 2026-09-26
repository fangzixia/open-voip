package control

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

// CreateDirect creates a controller-owned call without querying a Platform API.
// The first leg is returned to the controller for WebRTC negotiation or SIP ingress.
func (s *Service) CreateDirect(ctx context.Context, req dto.DirectCallRequest) (ports.CallView, error) {
	if req.CallID == "" {
		req.CallID = uuid.NewString()
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
	// Audio rooms use PCMU from creation so an outbound SIP leg can be added
	// after a WebRTC offer without silently switching codecs mid-call.
	if req.SessionType == dto.SessionTypeAudio {
		if m, ok := s.deps.Media.(interface{ PrepareSIP(string) }); ok {
			m.PrepareSIP(req.CallID)
		}
	}
	if err := s.deps.Media.CreateRoom(ctx, req.CallID, dto.RoomOptions{SessionType: req.SessionType, Direct: true}); err != nil {
		return ports.CallView{}, err
	}
	now := time.Now().UTC()
	rec := ports.CallRecord{ID: req.CallID, Direction: req.Direction, SessionType: req.SessionType, State: stateCreated, Caller: req.Caller, Callee: req.Callee, CreatedAt: now, UpdatedAt: now}
	leg := ports.CallLegRecord{ID: uuid.NewString(), CallID: req.CallID, Role: req.InitialLegRole, CreatedAt: now}
	if req.AgentID != "" {
		leg.AgentID = &req.AgentID
	}
	var err error
	if atomic, ok := s.deps.Calls.(interface {
		InsertDirectCall(context.Context, ports.CallRecord, ports.CallLegRecord) error
	}); ok {
		err = atomic.InsertDirectCall(ctx, rec, leg)
	} else {
		err = s.deps.Calls.InsertCall(ctx, rec)
		if err == nil {
			err = s.deps.Calls.InsertLeg(ctx, leg)
		}
	}
	if err != nil {
		_ = s.deps.Media.CloseRoom(ctx, req.CallID)
		return ports.CallView{}, err
	}
	rt := &runtimeCall{rec: rec, legs: []ports.CallLegRecord{leg}, caller: req.Caller, callee: req.Callee}
	s.mu.Lock()
	s.calls[req.CallID] = rt
	s.mu.Unlock()
	_ = s.publishCall(ctx, req.CallID, "call.created", "", map[string]any{"call_id": req.CallID, "direction": req.Direction, "leg_id": leg.ID})
	return toView(rt), nil
}

func (s *Service) AddDirectLeg(ctx context.Context, callID string, req dto.DirectLegRequest) (ports.CallView, error) {
	if req.Role != dto.LegRoleCustomer && req.Role != dto.LegRoleAgent && req.Role != dto.LegRoleSupervisor {
		return ports.CallView{}, errs.InvalidRequest("role 只支持 customer、agent、supervisor")
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
	leg := ports.CallLegRecord{ID: uuid.NewString(), CallID: callID, Role: req.Role, CreatedAt: time.Now().UTC()}
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

func (s *Service) DialDirectSIP(ctx context.Context, callID string, req dto.DirectSIPRequest) (ports.CallView, error) {
	if req.Destination == "" {
		return ports.CallView{}, errs.InvalidRequest("destination 必填")
	}
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State == stateEnded || rt.rec.QueueID != nil {
		return ports.CallView{}, errs.Conflict("通话不可拨号", "")
	}
	if len(rt.legs) >= 2 {
		return ports.CallView{}, errs.Conflict("直接控制通话最多支持两个媒体腿", "")
	}
	if rt.rec.SessionType != dto.SessionTypeAudio {
		return ports.CallView{}, errs.Unprocessable("SIP 腿仅支持语音", "")
	}
	if m, ok := s.deps.Media.(interface{ PrepareSIP(string) }); ok {
		m.PrepareSIP(callID)
	}
	if err := s.deps.Media.CreateRoom(ctx, callID, dto.RoomOptions{SessionType: dto.SessionTypeAudio}); err != nil {
		return ports.CallView{}, err
	}
	leg := ports.CallLegRecord{ID: uuid.NewString(), CallID: callID, Role: dto.LegRolePSTN, CreatedAt: time.Now().UTC()}
	if err := s.deps.Calls.InsertLeg(ctx, leg); err != nil {
		return ports.CallView{}, err
	}
	s.mu.Lock()
	rt.legs = append(rt.legs, leg)
	view := toView(rt)
	s.mu.Unlock()
	_ = s.publishCall(ctx, callID, "leg.dialing", "", map[string]any{"call_id": callID, "leg_id": leg.ID, "destination": req.Destination})
	go func() {
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
			return
		}
		if err != nil {
			_ = s.publishCall(lockedCtx, callID, "leg.failed", "", map[string]any{"call_id": callID, "leg_id": leg.ID, "error": err.Error()})
			return
		}
		_ = s.publishCall(lockedCtx, callID, "leg.answered", "", map[string]any{"call_id": callID, "leg_id": leg.ID})
	}()
	return view, nil
}

func (s *Service) BridgeDirect(ctx context.Context, callID, a, b string) error {
	if a == "" || b == "" || a == b {
		return errs.InvalidRequest("需要两个不同的 leg_id")
	}
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
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
		if m, ok := s.deps.Media.(interface{ UnbridgeLegs(string) }); ok {
			m.UnbridgeLegs(callID)
		}
		return err
	}
	return s.publishCall(ctx, callID, "call.answered", "", map[string]any{"call_id": callID, "leg_a": a, "leg_b": b})
}

func (s *Service) LeaveDirectLeg(ctx context.Context, callID, legID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
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
	if remover, ok := s.deps.Calls.(interface {
		DeleteLeg(context.Context, string, string) error
	}); ok {
		if err := remover.DeleteLeg(ctx, callID, legID); err != nil {
			return err
		}
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
	_ = s.publishCall(ctx, callID, "recording.stopped", "", map[string]any{"call_id": callID, "recording_id": id, "file_path": meta.FilePath, "file_size": meta.FileSize})
	return meta, nil
}
