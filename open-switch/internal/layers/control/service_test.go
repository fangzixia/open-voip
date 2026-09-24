package control

import (
	"context"
	"sync"
	"testing"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

type fakePersist struct {
	mu    sync.Mutex
	calls map[string]ports.CallRecord
	legs  map[string][]ports.CallLegRecord
}

func newFakePersist() *fakePersist {
	return &fakePersist{calls: map[string]ports.CallRecord{}, legs: map[string][]ports.CallLegRecord{}}
}

func (f *fakePersist) InsertCall(_ context.Context, rec ports.CallRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[rec.ID] = rec
	return nil
}
func (f *fakePersist) UpdateCall(_ context.Context, rec ports.CallRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[rec.ID] = rec
	return nil
}
func (f *fakePersist) GetCall(_ context.Context, id string) (ports.CallRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.calls[id]
	if !ok {
		return ports.CallRecord{}, errs.NotFound("no")
	}
	return r, nil
}
func (f *fakePersist) InsertLeg(_ context.Context, rec ports.CallLegRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.legs[rec.CallID] = append(f.legs[rec.CallID], rec)
	return nil
}
func (f *fakePersist) ListLegs(_ context.Context, callID string) ([]ports.CallLegRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ports.CallLegRecord{}, f.legs[callID]...), nil
}
func (f *fakePersist) GetLeg(_ context.Context, callID, legID string) (ports.CallLegRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.legs[callID] {
		if l.ID == legID {
			return l, nil
		}
	}
	return ports.CallLegRecord{}, errs.NotFound("no")
}

type fakeACD struct {
	agent  string
	agents *fakeAgents
}

func (f *fakeACD) RequestAgent(_ context.Context, _ dto.DispatchRequest) (dto.DispatchResult, error) {
	if f.agents != nil && f.agent != "" {
		f.agents.mu.Lock()
		f.agents.states[f.agent] = "ringing"
		f.agents.mu.Unlock()
	}
	return dto.DispatchResult{AgentID: f.agent}, nil
}

type fakeCfg struct{}

func (fakeCfg) GetQueue(_ context.Context, id string) (ports.QueueSnapshot, error) {
	return ports.QueueSnapshot{ID: id, Name: "语音服务", MaxWaitSec: 300}, nil
}
func (fakeCfg) GetLatestIVR(context.Context, string) (ports.IVRSnapshot, error) {
	return ports.IVRSnapshot{}, errs.ErrNotImplemented
}
func (fakeCfg) GetBusinessHours(context.Context, string) (ports.BusinessHours, error) {
	return ports.BusinessHours{WeekdayHours: "always"}, nil
}
func (fakeCfg) ResolveDID(context.Context, string) (string, error) {
	return "", errs.NotFound("DID 未配置")
}
func (fakeCfg) Now(context.Context) time.Time { return time.Now().UTC() }

type fakeAgents struct {
	mu     sync.Mutex
	states map[string]string
	ByExt  func() (ports.AgentInfo, error)
}

func newFakeAgents() *fakeAgents {
	return &fakeAgents{states: map[string]string{"ag1": "idle"}}
}
func TestHoldAndOutboundExtension(t *testing.T) {
	svc, _, _, _ := newTestService("ag1")
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Answer(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Hold(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetCall(ctx, id)
	if view.State != stateHeld {
		t.Fatalf("hold state=%s", view.State)
	}
	if err := svc.Hold(ctx, id, false); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeAgents) ByExtension(_ context.Context, extension string) (ports.AgentInfo, error) {
	if f.ByExt != nil {
		return f.ByExt()
	}
	return ports.AgentInfo{}, errs.ErrNotFound
}
func (f *fakeAgents) ByID(_ context.Context, id string) (ports.AgentInfo, error) {
	return ports.AgentInfo{AgentID: id}, nil
}
func (f *fakeAgents) SetState(_ context.Context, agentID, from, to, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur := f.states[agentID]
	if from != "" && cur != from {
		return errs.Conflict("bad", "")
	}
	f.states[agentID] = to
	return nil
}

type fakeMedia struct {
	created    bool
	closed     bool
	starts     int
	stops      int
	lastPolicy dto.RecordingPolicy
	sipOK      bool
}

func (f *fakeMedia) CreateRoom(context.Context, string, dto.RoomOptions) error {
	f.created = true
	return nil
}
func (f *fakeMedia) CloseRoom(context.Context, string) error { f.closed = true; return nil }
func (f *fakeMedia) JoinWebRTC(context.Context, string, string, dto.LegRole) (dto.LocalOffer, error) {
	return dto.LocalOffer{SDP: "v=0", Type: "offer"}, nil
}
func (f *fakeMedia) AcceptAnswer(context.Context, string, string, string) error { return nil }
func (f *fakeMedia) TrickleICE(context.Context, string, string, dto.ICECandidateInit) error {
	return nil
}
func (f *fakeMedia) IssueTURNCredentials(context.Context, string, time.Duration) (dto.TURNConfig, error) {
	return dto.TURNConfig{}, nil
}
func (f *fakeMedia) SetTrackMuted(context.Context, string, string, bool, bool) error { return nil }
func (f *fakeMedia) SetHold(context.Context, string, string, bool) error             { return nil }
func (f *fakeMedia) RequestRenegotiation(context.Context, string, string, bool) error {
	return nil
}
func (f *fakeMedia) InjectAudio(context.Context, string, string, dto.AudioSource) error {
	return nil
}
func (f *fakeMedia) SubscribeDTMF(context.Context, string, string, ports.DTMFHandler) error {
	return nil
}
func (f *fakeMedia) StartRecording(_ context.Context, _ string, policy dto.RecordingPolicy) (string, error) {
	f.starts++
	f.lastPolicy = policy
	if policy.Mode == "" || policy.Mode == "off" {
		return "", nil
	}
	return "rec1", nil
}
func (f *fakeMedia) StopRecording(context.Context, string) error {
	f.stops++
	return nil
}
func (f *fakeMedia) OriginateSIP(context.Context, string, string, string, string) error {
	if f.sipOK {
		return nil
	}
	return errs.Unprocessable("SIP 未启用", errs.CodeSIPDisabled)
}
func (f *fakeMedia) BridgeLegs(context.Context, string, string, string) error { return nil }
func (f *fakeMedia) LeaveRoom(context.Context, string, string) error          { return nil }
func (f *fakeMedia) SendDTMF(context.Context, string, string, dto.DTMFDigit) error {
	return nil
}
func (f *fakeMedia) RecordingInfo(context.Context, string) (ports.RecordingMeta, error) {
	return ports.RecordingMeta{ID: "rec1"}, nil
}

type fakeCDR struct{ last string }

func (f *fakeCDR) Upsert(_ context.Context, req ports.CDRWriteRequest) error {
	f.last = req.Result
	return nil
}

type recPolicy struct {
	policy dto.RecordingPolicy
}

func (r recPolicy) ForQueue(context.Context, string) (dto.RecordingPolicy, error) {
	if r.policy.Mode == "" {
		return dto.RecordingPolicy{Mode: "off"}, nil
	}
	return r.policy, nil
}

type fakeEvents struct{ types []string }

func (f *fakeEvents) PublishCallEvent(_ context.Context, ev ports.CallEvent) error {
	f.types = append(f.types, ev.Type)
	return nil
}

func newTestService(agent string) (*Service, *fakeMedia, *fakeAgents, *fakeEvents) {
	media := &fakeMedia{}
	agents := newFakeAgents()
	ev := &fakeEvents{}
	svc := NewService(Deps{
		Media:           media,
		ACD:             &fakeACD{agent: agent, agents: agents},
		Config:          fakeCfg{},
		Agents:          agents,
		RecordingPolicy: recPolicy{},
		CDR:             &fakeCDR{},
		Calls:           newFakePersist(),
		CallEvents:      ev,
	})
	return svc, media, agents, ev
}

func TestInboundAnswerHangup(t *testing.T) {
	svc, media, agents, ev := newTestService("ag1")
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1", SessionType: dto.SessionTypeAudio})
	if err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetCall(ctx, id)
	if view.State != stateRinging {
		t.Fatalf("state=%s", view.State)
	}
	if err := svc.Answer(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	if !media.created {
		t.Fatal("expected CreateRoom")
	}
	if agents.states["ag1"] != "on_call" {
		t.Fatalf("agent=%s", agents.states["ag1"])
	}
	if err := svc.Hangup(ctx, id, dto.HangupReasonNormal); err != nil {
		t.Fatal(err)
	}
	if agents.states["ag1"] != "acw" {
		t.Fatalf("after hangup want acw got %s", agents.states["ag1"])
	}
	if !media.closed {
		t.Fatal("expected CloseRoom")
	}
	joined := map[string]bool{}
	for _, typ := range ev.types {
		joined[typ] = true
	}
	for _, need := range []string{"call.ringing", "call.answered", "call.ended"} {
		if !joined[need] {
			t.Fatalf("missing event %s in %v", need, ev.types)
		}
	}
}

func TestDeclineRequeue(t *testing.T) {
	svc, _, agents, _ := newTestService("ag1")
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Decline(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetCall(ctx, id)
	if view.State != stateRinging && view.State != stateQueued {
		t.Fatalf("state=%s", view.State)
	}
	if agents.states["ag1"] != "ringing" && agents.states["ag1"] != "idle" {
		t.Fatalf("agent=%s", agents.states["ag1"])
	}
}

func TestDoubleAnswerIdempotent(t *testing.T) {
	svc, _, _, _ := newTestService("ag1")
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Answer(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	err = svc.Answer(ctx, id, "ag1")
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetCall(ctx, id)
	if err != nil || len(view.Legs) != 2 || view.State != stateActive {
		t.Fatalf("duplicate answer changed call: %+v, %v", view, err)
	}
}

type overflowCfg struct{ fakeCfg }

func (overflowCfg) GetQueue(_ context.Context, id string) (ports.QueueSnapshot, error) {
	if id == "q1" {
		return ports.QueueSnapshot{ID: "q1", Name: "语音", MaxWaitSec: 300, OverflowAction: "queue", OverflowQueueID: "q2"}, nil
	}
	return ports.QueueSnapshot{ID: id, Name: "溢出队列", MaxWaitSec: 300}, nil
}

func TestOverflowAndVideoRequest(t *testing.T) {
	svc, _, _, ev := newTestService("")
	svc.deps.Config = overflowCfg{}
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	if rt := svc.calls[id]; rt != nil {
		rt.maxWait = 0
		rt.queuedAt = time.Now().UTC().Add(-time.Second)
	}
	svc.mu.Unlock()
	svc.tick(ctx)
	view, _ := svc.GetCall(ctx, id)
	if view.QueueID != "q2" {
		t.Fatalf("queue after overflow=%s", view.QueueID)
	}
	found := false
	for _, typ := range ev.types {
		if typ == "queue.overflow" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing overflow event in %v", ev.types)
	}

	svc2, _, _, ev2 := newTestService("ag1")
	id2, err := svc2.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Answer(ctx, id2, "ag1"); err != nil {
		t.Fatal(err)
	}
	if err := svc2.RequestVideo(ctx, id2, "leg"); err != nil {
		t.Fatal(err)
	}
	if err := svc2.RespondVideo(ctx, id2, true); err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, typ := range ev2.types {
		if typ == "video.accepted" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("missing video.accepted in %v", ev2.types)
	}
}

func TestConsultTransferKeepsOriginal(t *testing.T) {
	svc, _, agents, _ := newTestService("ag1")
	agents.states["ag2"] = "idle"
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Answer(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Transfer(ctx, id, dto.TransferRequest{Mode: "consult", TargetAgentID: "ag2"}); err != nil {
		t.Fatal(err)
	}
	if agents.states["ag1"] != "on_call" {
		t.Fatalf("consult should keep original on_call, got %s", agents.states["ag1"])
	}
	if agents.states["ag2"] != "ringing" {
		t.Fatalf("target=%s", agents.states["ag2"])
	}
	if err := svc.Answer(ctx, id, "ag2"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteTransfer(ctx, id); err != nil {
		t.Fatal(err)
	}
	if agents.states["ag1"] != "acw" {
		t.Fatalf("original after complete=%s", agents.states["ag1"])
	}
}

func TestRecordingStartsOnActiveNotOnHold(t *testing.T) {
	svc, media, _, ev := newTestService("ag1")
	svc.deps.RecordingPolicy = recPolicy{policy: dto.RecordingPolicy{Mode: "audio", NotifyGuest: true, NotifyMessage: "告知", RetainDays: 30}}
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Answer(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	if media.starts != 1 || media.lastPolicy.Mode != "audio" || media.lastPolicy.RetainDays != 30 {
		t.Fatalf("answer recording starts=%d policy=%+v", media.starts, media.lastPolicy)
	}
	noticed := false
	for _, typ := range ev.types {
		if typ == "recording.notice" {
			noticed = true
		}
	}
	if !noticed {
		t.Fatalf("missing recording.notice in %v", ev.types)
	}
	if err := svc.Hold(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.Hold(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	if media.starts != 1 {
		t.Fatalf("hold/unhold must not restart recording, starts=%d", media.starts)
	}
	if err := svc.Hangup(ctx, id, dto.HangupReasonNormal); err != nil {
		t.Fatal(err)
	}
	if media.stops != 1 {
		t.Fatalf("hangup should stop recording, stops=%d", media.stops)
	}
}

func TestRecordingStartsOnSIPOutbound(t *testing.T) {
	svc, media, _, _ := newTestService("ag1")
	media.sipOK = true
	svc.deps.RecordingPolicy = recPolicy{policy: dto.RecordingPolicy{Mode: "audio", RetainDays: 90, NotifyMessage: "告知"}}
	ctx := context.Background()
	id, err := svc.Outbound(ctx, dto.OutboundRequest{AgentID: "ag1", Destination: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		view, err := svc.GetCall(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if view.State == stateActive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("SIP answer did not activate call")
		}
		time.Sleep(time.Millisecond)
	}
	if media.starts != 1 || media.lastPolicy.Mode != "audio" {
		t.Fatalf("outbound recording starts=%d policy=%+v", media.starts, media.lastPolicy)
	}
	if err := svc.Hangup(ctx, id, dto.HangupReasonNormal); err != nil {
		t.Fatal(err)
	}
	if media.stops != 1 {
		t.Fatalf("outbound hangup stops=%d", media.stops)
	}
}

func TestRecordingOffSkipsMedia(t *testing.T) {
	svc, media, _, _ := newTestService("ag1")
	ctx := context.Background()
	id, err := svc.StartInbound(ctx, dto.InboundRequest{QueueID: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Answer(ctx, id, "ag1"); err != nil {
		t.Fatal(err)
	}
	if media.starts != 0 {
		t.Fatalf("policy off should not StartRecording, starts=%d", media.starts)
	}
}
