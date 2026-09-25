package control

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"open-switch/internal/datetime"
	"open-switch/internal/errs"
	"open-switch/internal/observability"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

const (
	stateCreated      = "created"
	stateIVR          = "ivr"
	stateQueued       = "queued"
	stateRinging      = "ringing"
	stateActive       = "active"
	stateHeld         = "held"
	stateTransferring = "transferring"
	stateEnded        = "ended"

	offerTimeout = 30 * time.Second
)

// Deps 为 L3 服务依赖的 Port，由 bootstrap 注入。
type Deps struct {
	Media           ports.MediaPort
	ACD             ports.ACDDispatchPort
	Config          ports.ConfigSnapshotPort
	Agents          ports.AgentDirectoryPort
	RecordingPolicy ports.RecordingPolicyPort
	CDR             ports.CDRRecorderPort
	Calls           ports.CallPersistencePort
	CallEvents      ports.CallEventPublisher
	Recordings      ports.RecordingStorePort
}

type runtimeCall struct {
	sipOfferLeg    string
	sipOfferCancel context.CancelFunc
	rec            ports.CallRecord
	legs           []ports.CallLegRecord
	activeAgent    string
	offeredAgent   string
	offerAt        time.Time
	queuedAt       time.Time
	answeredAt     *time.Time
	caller         string
	callee         string
	queueName      string
	maxWait        time.Duration
	held           bool
	recordingID    string
	videoFromLeg   string
	videoUpgradeOk bool
	videoStartedAt *time.Time
	screenShares   int
	notify         string
	ivr            *ivrRuntime
	waitPrompt     string
	consultFrom    string
	transferMode   string
}

// Service 实现 CallControlPort 与 SignalingPort。
type Service struct {
	deps     Deps
	mu       sync.Mutex
	calls    map[string]*runtimeCall
	commands [256]sync.Mutex
}

// NewService 创建呼叫控制服务。
func NewService(deps Deps) *Service {
	return &Service{deps: deps, calls: map[string]*runtimeCall{}}
}

var _ ports.CallControlPort = (*Service)(nil)
var _ ports.SignalingPort = (*Service)(nil)

// StartInbound 幂等创建呼入及客户通话腿，并按营业时间和队列配置进入 IVR 或排队。
func (s *Service) StartInbound(ctx context.Context, req dto.InboundRequest) (string, error) {
	if req.CallID == "" {
		req.CallID = uuid.New().String()
	}
	ctx, unlock := s.command(ctx, req.CallID)
	defer unlock()
	if existing, err := s.deps.Calls.GetCall(ctx, req.CallID); err == nil {
		if deref(existing.QueueID) != req.QueueID {
			return "", errs.Conflict("call_id 已用于另一队列", "")
		}
		return existing.ID, nil
	} else if !errors.Is(err, errs.ErrNotFound) {
		return "", err
	}
	if req.QueueID == "" {
		return "", errs.InvalidRequest("queue_id 必填")
	}
	if req.SessionType == "" {
		req.SessionType = dto.SessionTypeAudio
	}
	q, err := s.deps.Config.GetQueue(ctx, req.QueueID)
	if err != nil {
		return "", err
	}
	if req.SessionType == dto.SessionTypeVideo && !q.VideoEnabled {
		return "", errs.Unprocessable("该队列不支持视频", errs.CodeAgentNotVideoCapable)
	}

	now := time.Now().UTC()
	callID := req.CallID
	if callID == "" {
		callID = uuid.New().String()
	}
	rec := ports.CallRecord{
		ID:          callID,
		Direction:   "inbound",
		SessionType: req.SessionType,
		State:       stateCreated,
		QueueID:     new(req.QueueID),
		Priority:    req.Priority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.deps.Calls.InsertCall(ctx, rec); err != nil {
		return "", err
	}
	leg := ports.CallLegRecord{
		ID:        uuid.New().String(),
		CallID:    callID,
		Role:      dto.LegRoleCustomer,
		CreatedAt: now,
	}
	if err := s.deps.Calls.InsertLeg(ctx, leg); err != nil {
		return "", err
	}

	maxWait := time.Duration(q.MaxWaitSec) * time.Second
	if maxWait <= 0 {
		maxWait = 5 * time.Minute
	}
	caller := "guest:" + req.GuestSessionID
	if req.Caller != "" {
		caller = req.Caller
	} else if req.GuestSessionID == "" {
		caller = "guest"
	}
	rt := &runtimeCall{
		rec:        rec,
		legs:       []ports.CallLegRecord{leg},
		queuedAt:   now,
		caller:     caller,
		callee:     q.Name,
		queueName:  q.Name,
		maxWait:    maxWait,
		waitPrompt: q.WaitPrompt,
	}

	s.mu.Lock()
	s.calls[callID] = rt
	s.mu.Unlock()
	ctx = observability.WithFields(ctx, observability.Fields{CallID: callID, QueueID: req.QueueID})
	observability.Event(ctx, "control", "call.created", stateCreated, "ok", "", now, "session_type", string(req.SessionType), "priority", req.Priority)
	slog.Info("呼入已创建", "call_id", callID, "queue_id", req.QueueID, "session_type", string(req.SessionType), "priority", req.Priority)

	// 非营业时段优先执行队列的留言、挂断或继续排队策略。
	if !withinHours(s.deps.Config, ctx, req.QueueID) {
		action := q.AfterHoursAction
		if action == "" {
			action = "hangup"
		}
		_ = s.publishCall(ctx, callID, "ivr.prompt", "", map[string]any{"call_id": callID, "message": "当前为非工作时间"})
		if action == "voicemail" {
			s.startVoicemail(ctx, callID)
			return callID, nil
		}
		if action != "queue" {
			_ = s.Hangup(ctx, callID, dto.HangupReasonTimeout)
			return callID, nil
		}
	}

	if !req.SkipIVR && q.IVRFlowID != "" {
		if err := s.bootIVR(ctx, callID, q.IVRFlowID); err == nil {
			return callID, nil
		}
	}

	if err := s.deps.Media.CreateRoom(ctx, callID, dto.RoomOptions{SessionType: req.SessionType}); err != nil {
		return "", err
	}
	if err := s.enterQueue(ctx, callID); err != nil {
		return "", err
	}
	return callID, nil
}

// Answer 校验振铃归属，建立坐席通话腿；咨询转接时先保持客户并通知双方。
func (s *Service) Answer(ctx context.Context, callID, agentID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		return errs.NotFound("通话不存在")
	}
	if (rt.rec.State == stateActive || rt.rec.State == stateHeld) && rt.offeredAgent == "" && rt.activeAgent == agentID {
		return nil
	}
	if rt.rec.State != stateRinging && (rt.rec.State != stateActive || rt.offeredAgent != agentID) {
		return errs.Conflict("当前不是振铃状态", errs.CodeCallNotRinging)
	}
	if rt.offeredAgent != agentID {
		return errs.Forbidden("不是当前振铃坐席")
	}

	if rt.sipOfferLeg != "" && ctx.Value(sipAnswerKey{}) != true {
		return errs.Conflict("请在 SIP 话机接听", "")
	}
	now := time.Now().UTC()
	agentLeg := ports.CallLegRecord{
		ID:        uuid.New().String(),
		CallID:    callID,
		Role:      dto.LegRoleAgent,
		AgentID:   &agentID,
		CreatedAt: now,
	}
	if rt.sipOfferLeg != "" {
		agentLeg.ID = rt.sipOfferLeg
	}
	if err := s.deps.Calls.InsertLeg(ctx, agentLeg); err != nil {
		return err
	}

	opts := dto.RoomOptions{SessionType: rt.rec.SessionType, EnableVideo: rt.rec.SessionType != dto.SessionTypeAudio}
	if err := s.deps.Media.CreateRoom(ctx, callID, opts); err != nil {
		return err
	}
	if err := s.deps.Media.StopInjectedAudio(ctx, callID); err != nil {
		return err
	}

	if err := s.setAgentState(ctx, callID, agentID, "ringing", "on_call", "answer"); err != nil {
		return err
	}

	s.mu.Lock()
	rt.legs = append(rt.legs, agentLeg)
	t := now
	if rt.answeredAt == nil {
		rt.answeredAt = &t
	}
	if rt.rec.SessionType == dto.SessionTypeVideo || rt.rec.SessionType == dto.SessionTypeMixed {
		rt.videoStartedAt = &t
	}
	consulting := rt.consultFrom != "" && rt.consultFrom != agentID
	rt.activeAgent = agentID
	rt.sipOfferCancel = nil
	rt.offeredAgent = ""
	s.mu.Unlock()

	if consulting {
		// 咨询转：客户保持，原坐席与目标先通话。
		if err := s.transition(ctx, callID, stateHeld); err != nil {
			return err
		}
		slog.Info("咨询转目标已接听", "call_id", callID, "agent_id", agentID)
		_ = s.cdrUpsert(ctx, callID, "answered")
		return s.publishCall(ctx, callID, "call.consulting", agentID, map[string]any{
			"call_id": callID, "agent_id": agentID,
		})
	}

	if err := s.transition(ctx, callID, stateActive); err != nil {
		return err
	}
	slog.Info("通话已接通", "call_id", callID, "agent_id", agentID)
	_ = s.cdrUpsert(ctx, callID, "answered")
	return s.publishCall(ctx, callID, "call.answered", agentID, map[string]any{
		"call_id":     callID,
		"agent_id":    agentID,
		"answered_at": datetime.Format(now),
	})
}

// Decline 释放当前振铃坐席；咨询转接失败时恢复原通话，否则重新派单。
func (s *Service) Decline(ctx context.Context, callID, agentID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		return errs.NotFound("通话不存在")
	}
	if rt.rec.State != stateRinging || rt.offeredAgent != agentID {
		return errs.Conflict("当前不是该坐席的振铃", errs.CodeCallNotRinging)
	}
	_ = s.setAgentState(ctx, callID, agentID, "ringing", "idle", "decline")
	s.mu.Lock()
	consultFrom := rt.consultFrom
	rt.offeredAgent = ""
	s.mu.Unlock()
	if consultFrom != "" {
		cust := customerLeg(rt)
		_ = s.deps.Media.SetHold(ctx, callID, cust, false)
		s.mu.Lock()
		rt.consultFrom = ""
		rt.transferMode = ""
		rt.held = false
		s.mu.Unlock()
		_ = s.transition(ctx, callID, stateActive)
		return s.publishCall(ctx, callID, "call.unhold", consultFrom, map[string]any{"call_id": callID, "reason": "consult_declined"})
	}
	if err := s.transition(ctx, callID, stateQueued); err != nil {
		return err
	}
	return s.tryDispatch(ctx, callID)
}

// Hangup 幂等结束通话，依次释放坐席、停止录音、写入话单并关闭媒体房间。
func (s *Service) Hangup(ctx context.Context, callID string, reason dto.HangupReason) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		rec, err := s.deps.Calls.GetCall(ctx, callID)
		if err != nil {
			return err
		}
		if rec.State == stateEnded {
			return nil
		}
		return errs.NotFound("通话运行时不存在")
	}
	if rt.rec.State == stateEnded {
		return nil
	}

	if rt.sipOfferCancel != nil {
		rt.sipOfferCancel()
		rt.sipOfferCancel = nil
	}
	offered := rt.offeredAgent
	prev := rt.rec.State
	consultFrom := rt.consultFrom
	toACW := prev == stateActive || prev == stateHeld || prev == stateTransferring || consultFrom != ""
	seen := map[string]struct{}{}
	if offered != "" {
		seen[offered] = struct{}{}
		from := "on_call"
		to := "idle"
		if prev == stateRinging {
			from = "ringing"
		} else if toACW {
			to = "acw"
		}
		_ = s.setAgentState(ctx, callID, offered, from, to, "hangup")
	}
	for _, leg := range rt.legs {
		if leg.AgentID == nil || *leg.AgentID == "" {
			continue
		}
		if _, ok := seen[*leg.AgentID]; ok {
			continue
		}
		seen[*leg.AgentID] = struct{}{}
		from := "on_call"
		to := "idle"
		if toACW {
			to = "acw"
		}
		_ = s.setAgentState(ctx, callID, *leg.AgentID, from, to, "hangup")
	}
	slog.Info("通话挂断", "call_id", callID, "reason", string(reason), "prev_state", prev)

	if err := s.stopRecording(ctx, callID); err != nil {
		return err
	}
	s.mu.Lock()
	rt.rec.EndedAt = new(time.Now().UTC())
	s.mu.Unlock()

	// 未接通的来电按原因记为放弃或失败，供 CDR 与报表区分。
	result := "answered"
	if rt.answeredAt == nil {
		result = "abandoned"
		if reason == dto.HangupReasonError {
			result = "failed"
		}
		if reason == dto.HangupReasonTimeout {
			result = "abandoned"
		}
	}
	if err := s.cdrUpsert(ctx, callID, result); err != nil {
		return err
	}
	if err := s.transition(ctx, callID, stateEnded); err != nil {
		return err
	}
	_ = s.deps.Media.CloseRoom(ctx, callID)
	_ = s.publishCall(ctx, callID, "call.ended", offered, map[string]any{
		"call_id": callID,
		"reason":  string(reason),
		"result":  result,
	})
	endedAt := *rt.rec.EndedAt
	var waitMS, ringMS, talkMS int64
	if !rt.queuedAt.IsZero() {
		waitEnd := endedAt
		if !rt.offerAt.IsZero() {
			waitEnd = rt.offerAt
		}
		waitMS = waitEnd.Sub(rt.queuedAt).Milliseconds()
	}
	if !rt.offerAt.IsZero() {
		ringEnd := endedAt
		if rt.answeredAt != nil {
			ringEnd = *rt.answeredAt
		}
		ringMS = ringEnd.Sub(rt.offerAt).Milliseconds()
	}
	if rt.answeredAt != nil {
		talkMS = endedAt.Sub(*rt.answeredAt).Milliseconds()
	}
	observability.Event(ctx, "control", "call.trace.summary", "ended", result, string(reason), time.Time{},
		"session_type", string(rt.rec.SessionType),
		"total_ms", endedAt.Sub(rt.rec.CreatedAt).Milliseconds(),
		"queue_wait_ms", waitMS, "ring_ms", ringMS, "talk_ms", talkMS,
		"video_started", rt.videoStartedAt != nil, "recording_id", rt.recordingID)
	s.mu.Lock()
	delete(s.calls, callID)
	s.mu.Unlock()
	return nil
}

// Transfer 串行执行转接，避免同一通话的状态操作并发交错。
func (s *Service) Transfer(ctx context.Context, callID string, req dto.TransferRequest) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	return s.doTransfer(ctx, callID, req)
}

// CompleteTransfer 结束咨询阶段，将客户接入目标坐席。
func (s *Service) CompleteTransfer(ctx context.Context, callID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	return s.completeConsult(ctx, callID)
}

// Outbound 创建外呼并交由具体目的地的呼叫流程处理。
func (s *Service) Outbound(ctx context.Context, req dto.OutboundRequest) (string, error) {
	return s.doOutbound(ctx, req)
}

// StartIVR 按通话所属队列加载当前发布的 IVR 流程。
func (s *Service) StartIVR(ctx context.Context, callID, snapshotID string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	return s.attachIVR(ctx, callID, snapshotID)
}

// GetCall 优先读取内存中的实时状态，缺失时从持久化记录重建视图。
func (s *Service) GetCall(ctx context.Context, callID string) (ports.CallView, error) {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt != nil {
		return toView(rt), nil
	}
	rec, err := s.deps.Calls.GetCall(ctx, callID)
	if err != nil {
		return ports.CallView{}, err
	}
	legs, err := s.deps.Calls.ListLegs(ctx, callID)
	if err != nil {
		return ports.CallView{}, err
	}
	return toView(&runtimeCall{rec: rec, legs: legs}), nil
}

// JoinWebRTC 校验通话腿和当前状态，再向媒体层申请本地 Offer。
func (s *Service) JoinWebRTC(ctx context.Context, callID, legID string) (dto.LocalOffer, error) {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	view, err := s.GetCall(ctx, callID)
	if err != nil {
		return dto.LocalOffer{}, err
	}
	var role dto.LegRole
	found := false
	for _, leg := range view.Legs {
		if leg.ID == legID {
			role = leg.Role
			found = true
			break
		}
	}
	if !found {
		return dto.LocalOffer{}, errs.NotFound("通话腿不存在")
	}
	preAnswerCustomer := role == dto.LegRoleCustomer && (view.State == stateQueued || view.State == stateRinging)
	if !preAnswerCustomer && view.State != stateActive && view.State != stateIVR && view.State != stateHeld && view.State != stateTransferring {
		return dto.LocalOffer{}, errs.Conflict("当前状态无法加入媒体", "")
	}
	return s.deps.Media.JoinWebRTC(ctx, callID, legID, role)
}

func (s *Service) AcceptAnswer(ctx context.Context, callID, legID string, answerSDP string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if _, err := s.deps.Calls.GetLeg(ctx, callID, legID); err != nil {
		return err
	}
	return s.deps.Media.AcceptAnswer(ctx, callID, legID, answerSDP)
}

func (s *Service) TrickleICE(ctx context.Context, callID, legID string, cand dto.ICECandidateInit) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if _, err := s.deps.Calls.GetLeg(ctx, callID, legID); err != nil {
		return err
	}
	return s.deps.Media.TrickleICE(ctx, callID, legID, cand)
}

func (s *Service) SetTrackMuted(ctx context.Context, callID, legID string, audio, video bool) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if _, err := s.deps.Calls.GetLeg(ctx, callID, legID); err != nil {
		return err
	}
	return s.deps.Media.SetTrackMuted(ctx, callID, legID, audio, video)
}

func (s *Service) IssueTURNCredentials(ctx context.Context, callID, subject string) (dto.TURNConfig, error) {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	if _, err := s.GetCall(ctx, callID); err != nil {
		return dto.TURNConfig{}, err
	}
	return s.deps.Media.IssueTURNCredentials(ctx, subject, time.Hour)
}

// ActiveCalls 返回排队/振铃/通话中的数量。
func (s *Service) ActiveCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, rt := range s.calls {
		switch rt.rec.State {
		case stateQueued, stateRinging, stateActive, stateHeld, stateIVR, stateTransferring:
			n++
		}
	}
	return n
}

// Run 后台派单与超时。
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

// tick 按优先级和入队时间先处理等待通话，再扫描其他状态的超时。
func (s *Service) tick(ctx context.Context) {
	type queuedItem struct {
		id     string
		prio   int
		queued time.Time
	}
	s.mu.Lock()
	queued := make([]queuedItem, 0)
	ids := make([]string, 0, len(s.calls))
	for id, rt := range s.calls {
		if rt.rec.State == stateQueued {
			queued = append(queued, queuedItem{id: id, prio: rt.rec.Priority, queued: rt.queuedAt})
			continue
		}
		ids = append(ids, id)
	}
	s.mu.Unlock()
	sort.Slice(queued, func(i, j int) bool {
		if queued[i].prio != queued[j].prio {
			return queued[i].prio > queued[j].prio
		}
		return queued[i].queued.Before(queued[j].queued)
	})
	ordered := make([]string, 0, len(queued)+len(ids))
	for _, q := range queued {
		ordered = append(ordered, q.id)
	}
	ordered = append(ordered, ids...)
	now := time.Now().UTC()
	for _, id := range ordered {
		s.tickCall(ctx, id, now)
	}
}

// tickCall 推进单通呼叫的排队、IVR 或振铃超时状态。
func (s *Service) tickCall(ctx context.Context, id string, now time.Time) {
	ctx, unlock := s.command(ctx, id)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[id]
	s.mu.Unlock()
	if rt == nil {
		return
	}
	switch rt.rec.State {
	case stateQueued:
		if now.Sub(rt.queuedAt) > rt.maxWait {
			if s.overflow(ctx, id) {
				return
			}
			_ = s.Hangup(ctx, id, dto.HangupReasonTimeout)
			return
		}
		s.publishPosition(ctx, id)
		_ = s.tryDispatch(ctx, id)
	case stateIVR:
		s.tickIVR(ctx, id)
	case stateRinging:
		if !rt.offerAt.IsZero() && now.Sub(rt.offerAt) > offerTimeout {
			if rt.sipOfferCancel != nil {
				rt.sipOfferCancel()
				continueSIP := rt.sipOfferLeg != ""
				if continueSIP {
					return
				}
			}
			agentID := rt.offeredAgent
			if agentID != "" {
				_ = s.setAgentState(ctx, id, agentID, "ringing", "idle", "offer_timeout")
			}
			s.mu.Lock()
			consultFrom := ""
			if cur := s.calls[id]; cur != nil {
				consultFrom = cur.consultFrom
				cur.offeredAgent = ""
			}
			s.mu.Unlock()
			if consultFrom != "" {
				cust := customerLeg(rt)
				_ = s.deps.Media.SetHold(ctx, id, cust, false)
				s.mu.Lock()
				if cur := s.calls[id]; cur != nil {
					cur.consultFrom = ""
					cur.transferMode = ""
					cur.held = false
				}
				s.mu.Unlock()
				_ = s.transition(ctx, id, stateActive)
				_ = s.publishCall(ctx, id, "call.unhold", consultFrom, map[string]any{"call_id": id, "reason": "consult_timeout"})
				return
			}
			_ = s.transition(ctx, id, stateQueued)
			_ = s.tryDispatch(ctx, id)
		}
	}
}

// tryDispatch 请求业务侧原子选人；旧派单结果会释放坐席，避免占用已变化的通话。
func (s *Service) tryDispatch(ctx context.Context, callID string) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.State != stateQueued || rt.rec.QueueID == nil {
		return nil
	}
	started := time.Now()
	res, err := s.deps.ACD.RequestAgent(ctx, dto.DispatchRequest{
		CallID:       callID,
		QueueID:      *rt.rec.QueueID,
		RequireVideo: rt.rec.SessionType == dto.SessionTypeVideo,
		SkillIDs:     nil,
	})
	if err != nil {
		observability.Event(ctx, "acd", "acd.dispatch", "response", "error", "dispatch_failed", started, "queue_id", *rt.rec.QueueID)
		return err
	}
	observability.Event(ctx, "acd", "acd.dispatch", "response", "ok", "", started, "queue_id", *rt.rec.QueueID, "agent_id", res.AgentID)
	if res.AgentID == "" {
		return nil
	}
	s.mu.Lock()
	rt = s.calls[callID]
	if rt == nil || rt.rec.State != stateQueued {
		s.mu.Unlock()
		_ = s.setAgentState(ctx, callID, res.AgentID, "ringing", "idle", "stale_offer")
		return nil
	}
	rt.offeredAgent = res.AgentID
	rt.offerAt = time.Now().UTC()
	s.mu.Unlock()
	if err := s.transition(ctx, callID, stateRinging); err != nil {
		return err
	}
	if err := s.ringDevice(ctx, callID, res.AgentID); err != nil {
		return err
	}
	return s.publishCall(ctx, callID, "call.ringing", res.AgentID, map[string]any{
		"call_id":      callID,
		"queue_id":     deref(rt.rec.QueueID),
		"queue_name":   rt.queueName,
		"caller":       rt.caller,
		"session_type": string(rt.rec.SessionType),
	})
}

// transition 同步更新运行时和持久化状态；写库失败时恢复此前的内存记录。
func (s *Service) transition(ctx context.Context, callID, state string) error {
	started := time.Now()
	s.mu.Lock()
	rt := s.calls[callID]
	if rt == nil {
		s.mu.Unlock()
		return errs.NotFound("通话不存在")
	}
	prev := rt.rec.State
	if prev == stateEnded && state != stateEnded {
		s.mu.Unlock()
		return errs.Conflict("通话已结束", "")
	}
	before := rt.rec
	rt.rec.Caller = rt.caller
	rt.rec.Callee = rt.callee
	rt.rec.AgentID = rt.activeAgent
	rt.rec.OfferedAgent = rt.offeredAgent
	rt.rec.AnsweredAt = rt.answeredAt
	rt.rec.State = state
	rt.rec.UpdatedAt = time.Now().UTC()
	rec := rt.rec
	s.mu.Unlock()
	if err := s.deps.Calls.UpdateCall(ctx, rec); err != nil {
		s.mu.Lock()
		rt.rec = before
		s.mu.Unlock()
		observability.Event(ctx, "control", "fsm.transition", state, "error", "persist_failed", started, "from_state", prev, "to_state", state)
		return err
	}
	observability.Event(ctx, "control", "fsm.transition", state, "ok", "", started, "from_state", prev, "to_state", state)
	if state == stateActive && prev != stateActive {
		s.beginRecordingIfNeeded(ctx, callID)
	}
	return nil
}

// cdrUpsert 汇总通话时间、媒体和参与方信息，更新同一条话单。
func (s *Service) cdrUpsert(ctx context.Context, callID, result string) error {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		return nil
	}
	started := time.Now()
	err := s.deps.CDR.Upsert(ctx, ports.CDRWriteRequest{
		CallID:           callID,
		Direction:        rt.rec.Direction,
		QueueID:          deref(rt.rec.QueueID),
		AgentID:          rt.activeAgent,
		Caller:           rt.caller,
		Callee:           rt.callee,
		SessionType:      rt.rec.SessionType,
		Result:           result,
		StartedAt:        rt.rec.CreatedAt,
		AnsweredAt:       rt.answeredAt,
		EndedAt:          rt.rec.EndedAt,
		VideoStartedAt:   rt.videoStartedAt,
		VideoUpgradeOk:   rt.videoUpgradeOk,
		ScreenShareCount: rt.screenShares,
	})
	status, reason := "ok", ""
	if err != nil {
		status, reason = "error", "cdr_upsert_failed"
	}
	observability.Event(ctx, "cdr", "cdr.upsert", "persist", status, reason, started, "result_value", result)
	return err
}

// publishCall 向目标坐席及通话中的其他坐席广播事件，并对收件人去重。
func (s *Service) publishCall(ctx context.Context, callID, typ, agentID string, payload map[string]any) error {
	if s.deps.CallEvents == nil {
		return nil
	}
	err := s.deps.CallEvents.PublishCallEvent(ctx, ports.CallEvent{
		Type: typ, CallID: callID, AgentID: agentID, Payload: payload,
	})
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil {
		return err
	}
	seen := map[string]struct{}{agentID: {}}
	var extra []string
	if rt.offeredAgent != "" {
		extra = append(extra, rt.offeredAgent)
	}
	for _, l := range rt.legs {
		if l.AgentID != nil {
			extra = append(extra, *l.AgentID)
		}
	}
	for _, id := range extra {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		_ = s.deps.CallEvents.PublishCallEvent(ctx, ports.CallEvent{Type: typ, AgentID: id, Payload: payload})
	}
	return err
}

func toView(rt *runtimeCall) ports.CallView {
	v := ports.CallView{
		ID:        rt.rec.ID,
		CreatedAt: rt.rec.CreatedAt, Caller: rt.caller,
		State:           rt.rec.State,
		Direction:       rt.rec.Direction,
		SessionType:     rt.rec.SessionType,
		AgentID:         rt.offeredAgent,
		Held:            rt.held,
		RecordingNotice: rt.notify,
		Legs:            make([]ports.LegView, 0, len(rt.legs)),
	}
	if rt.rec.QueueID != nil {
		v.QueueID = *rt.rec.QueueID
	}
	if v.AgentID == "" {
		for _, leg := range rt.legs {
			if leg.AgentID != nil {
				v.AgentID = *leg.AgentID
			}
		}
	}
	for _, leg := range rt.legs {
		lv := ports.LegView{ID: leg.ID, Role: leg.Role}
		if leg.AgentID != nil {
			lv.AgentID = *leg.AgentID
		}
		v.Legs = append(v.Legs, lv)
	}
	return v
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
