package control

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
	"open-voip/internal/ports/dto"
)

type ivrRuntime struct {
	doc      ivrDoc
	node     string
	entered  time.Time
	timeout  time.Duration
}

type ivrDoc struct {
	Start string             `json:"start"`
	Nodes map[string]ivrNode `json:"nodes"`
}

type ivrNode struct {
	Type        string            `json:"type"`
	Prompt      string            `json:"prompt"`
	File        string            `json:"file"`
	TimeoutSec  int               `json:"timeout_sec"`
	Choices     map[string]string `json:"choices"`
	Default     string            `json:"default"`
	QueueID     string            `json:"queue_id"`
	SessionType string            `json:"session_type"`
	Next        string            `json:"next"`
	Open        string            `json:"open"`
	Closed      string            `json:"closed"`
}

func (s *Service) bootIVR(ctx context.Context, callID, flowID string) error {
	snap, err := s.deps.Config.GetLatestIVR(ctx, flowID)
	if err != nil {
		return err
	}
	return s.startIVRPayload(ctx, callID, snap)
}

func (s *Service) attachIVR(ctx context.Context, callID, snapshotID string) error {
	_ = snapshotID
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.rec.QueueID == nil {
		return errs.NotFound("通话不存在")
	}
	q, err := s.deps.Config.GetQueue(ctx, *rt.rec.QueueID)
	if err != nil {
		return err
	}
	if q.IVRFlowID == "" {
		return errs.InvalidRequest("队列未绑定 IVR")
	}
	return s.bootIVR(ctx, callID, q.IVRFlowID)
}

func (s *Service) startIVRPayload(ctx context.Context, callID string, snap ports.IVRSnapshot) error {
	var doc ivrDoc
	if err := json.Unmarshal([]byte(snap.PayloadJSON), &doc); err != nil || doc.Start == "" {
		return errs.InvalidRequest("IVR 快照无效")
	}
	opts := dto.RoomOptions{SessionType: dto.SessionTypeAudio}
	if err := s.deps.Media.CreateRoom(ctx, callID, opts); err != nil {
		return err
	}
	bot := ports.CallLegRecord{ID: uuid.New().String(), CallID: callID, Role: dto.LegRoleIVRBot, CreatedAt: time.Now().UTC()}
	_ = s.deps.Calls.InsertLeg(ctx, bot)
	s.mu.Lock()
	rt := s.calls[callID]
	if rt != nil {
		rt.legs = append(rt.legs, bot)
		rt.ivr = &ivrRuntime{doc: doc, node: doc.Start, entered: time.Now().UTC(), timeout: 15 * time.Second}
	}
	s.mu.Unlock()
	cust := ""
	if rt != nil {
		for _, l := range rt.legs {
			if l.Role == dto.LegRoleCustomer {
				cust = l.ID
			}
		}
	}
	if cust != "" {
		_ = s.deps.Media.SubscribeDTMF(ctx, callID, cust, func(ctx context.Context, digit dto.DTMFDigit) {
			s.onDTMF(ctx, callID, string(digit))
		})
	}
	if err := s.transition(ctx, callID, stateIVR); err != nil {
		return err
	}
	_ = s.publishCall(ctx, callID, "ivr.started", "", map[string]any{"call_id": callID, "snapshot_id": snap.SnapshotID})
	s.runIVRNode(ctx, callID)
	return nil
}

func (s *Service) tickIVR(ctx context.Context, callID string) {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.ivr == nil {
		return
	}
	to := rt.ivr.timeout
	if to <= 0 {
		to = 12 * time.Second
	}
	if time.Since(rt.ivr.entered) < to {
		return
	}
	node := rt.ivr.doc.Nodes[rt.ivr.node]
	next := node.Default
	if next == "" {
		next = node.Next
	}
	if next == "" {
		_ = s.enterQueue(ctx, callID)
		return
	}
	s.gotoIVR(ctx, callID, next)
}

func (s *Service) onDTMF(ctx context.Context, callID, digit string) {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.ivr == nil {
		return
	}
	node := rt.ivr.doc.Nodes[rt.ivr.node]
	if node.Type != "menu" || node.Choices == nil {
		return
	}
	next, ok := node.Choices[digit]
	if !ok {
		return
	}
	s.gotoIVR(ctx, callID, next)
}

func (s *Service) gotoIVR(ctx context.Context, callID, nodeID string) {
	s.mu.Lock()
	rt := s.calls[callID]
	if rt != nil && rt.ivr != nil {
		rt.ivr.node = nodeID
		rt.ivr.entered = time.Now().UTC()
	}
	s.mu.Unlock()
	s.runIVRNode(ctx, callID)
}

func (s *Service) runIVRNode(ctx context.Context, callID string) {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.ivr == nil {
		return
	}
	node, ok := rt.ivr.doc.Nodes[rt.ivr.node]
	if !ok {
		_ = s.enterQueue(ctx, callID)
		return
	}
	_ = s.deps.Media.InjectAudio(ctx, callID, "", dto.AudioSource{FilePath: node.File, Loop: node.Type == "menu"})
	_ = s.publishCall(ctx, callID, "ivr.prompt", "", map[string]any{"call_id": callID, "prompt": node.Prompt, "type": node.Type})
	switch node.Type {
	case "hangup":
		_ = s.Hangup(ctx, callID, dto.HangupReasonNormal)
	case "route_queue":
		qid := node.QueueID
		s.mu.Lock()
		if rt2 := s.calls[callID]; rt2 != nil {
			rt2.rec.QueueID = &qid
			if node.SessionType != "" {
				rt2.rec.SessionType = dto.SessionType(node.SessionType)
			}
			rt2.ivr = nil
		}
		s.mu.Unlock()
		_ = s.enterQueue(ctx, callID)
	case "time_check":
		open := true
		if rt.rec.QueueID != nil {
			open = withinHours(s.deps.Config, ctx, *rt.rec.QueueID)
		}
		next := node.Open
		if !open {
			next = node.Closed
		}
		if next == "" {
			_ = s.enterQueue(ctx, callID)
			return
		}
		s.gotoIVR(ctx, callID, next)
	case "play":
		if node.Next != "" {
			time.AfterFunc(2*time.Second, func() { s.gotoIVR(context.Background(), callID, node.Next) })
		}
	}
}

func (s *Service) enterQueue(ctx context.Context, callID string) error {
	if err := s.transition(ctx, callID, stateQueued); err != nil {
		return err
	}
	s.mu.Lock()
	rt := s.calls[callID]
	if rt != nil {
		rt.queuedAt = time.Now().UTC()
		rt.ivr = nil
	}
	s.mu.Unlock()
	if rt != nil {
		opts := dto.RoomOptions{SessionType: rt.rec.SessionType, EnableVideo: rt.rec.SessionType != dto.SessionTypeAudio}
		_ = s.deps.Media.CreateRoom(ctx, callID, opts)
		promptFile := rt.waitPrompt
		if !strings.HasSuffix(strings.ToLower(promptFile), ".wav") {
			promptFile = ""
		}
		_ = s.deps.Media.InjectAudio(ctx, callID, "", dto.AudioSource{FilePath: promptFile, Loop: true})
	}
	_ = s.cdrUpsert(ctx, callID, "queued")
	s.publishPosition(ctx, callID)
	return s.tryDispatch(ctx, callID)
}

func (s *Service) publishPosition(ctx context.Context, callID string) {
	s.mu.Lock()
	rt := s.calls[callID]
	pos := 1
	msg := "正在等待空闲坐席"
	if rt != nil {
		for _, o := range s.calls {
			if o.rec.State == stateQueued && o.rec.QueueID != nil && rt.rec.QueueID != nil &&
				*o.rec.QueueID == *rt.rec.QueueID && (o.rec.Priority > rt.rec.Priority || (o.rec.Priority == rt.rec.Priority && o.queuedAt.Before(rt.queuedAt))) {
				pos++
			}
		}
		if rt.waitPrompt != "" {
			msg = strings.ReplaceAll(rt.waitPrompt, "{position}", strconv.Itoa(pos))
		} else {
			msg = "您前面还有 " + strconv.Itoa(pos-1) + " 位，请稍候"
		}
	}
	s.mu.Unlock()
	_ = s.publishCall(ctx, callID, "queue.position", "", map[string]any{"call_id": callID, "position": pos, "message": msg})
}

func withinHours(cfg ports.ConfigSnapshotPort, ctx context.Context, queueID string) bool {
	h, err := cfg.GetBusinessHours(ctx, queueID)
	if err != nil || h.WeekdayHours == "" || h.WeekdayHours == "always" {
		return true
	}
	var m map[string]string
	if json.Unmarshal([]byte(h.WeekdayHours), &m) != nil {
		return true
	}
	now := cfg.Now(ctx)
	if h.Timezone != "" {
		if loc, err := time.LoadLocation(h.Timezone); err == nil {
			now = now.In(loc)
		}
	}
	key := strings.ToLower(now.Weekday().String()[:3])
	win, ok := m[key]
	if !ok {
		win = m[strconv.Itoa(int(now.Weekday()))]
	}
	if win == "" || win == "closed" {
		return false
	}
	parts := strings.Split(win, "-")
	if len(parts) != 2 {
		return true
	}
	cur := now.Format("15:04")
	return cur >= parts[0] && cur <= parts[1]
}
