package control

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

func (s *Service) recoverIVRCall(ctx context.Context, rec ports.CallRecord, legs []ports.CallLegRecord, sess ports.IVRSessionView) error {
	cfgVer := int64(0)
	if rec.ConfigVersion != nil {
		cfgVer = *rec.ConfigVersion
	}
	snap, err := s.deps.Config.GetIVRSnapshot(ctx, cfgVer, sess.FlowID, sess.FlowVersion)
	if err != nil {
		return err
	}
	var doc ivrDoc
	if err := json.Unmarshal([]byte(snap.PayloadJSON), &doc); err != nil || doc.Start == "" {
		return errs.InvalidRequest("IVR 快照无效")
	}
	node, ok := doc.Nodes[sess.NodeID]
	if !ok {
		return errs.InvalidRequest("IVR 节点不存在")
	}
	var st struct {
		InvalidAttempts int    `json:"invalid_attempts"`
		ActionID        string `json:"action_id"`
	}
	_ = json.Unmarshal([]byte(sess.StateJSON), &st)
	if st.ActionID != "" {
		ba, err := s.deps.BusinessActions.GetBusinessAction(ctx, st.ActionID)
		if err != nil || ba.Status != "pending" {
			st.ActionID = ""
		}
	}
	entered, timeout := ivrTimingFromSession(sess, node)
	rt := &runtimeCall{
		rec: rec, legs: legs, caller: rec.Caller, callee: rec.Callee,
		activeAgent: rec.AgentID, offeredAgent: rec.OfferedAgent, answeredAt: rec.AnsweredAt,
		ivr: &ivrRuntime{
			doc: doc, flowID: sess.FlowID, flowVersion: sess.FlowVersion,
			node: sess.NodeID, entered: entered, timeout: timeout,
			invalidAttempts: st.InvalidAttempts, actionID: st.ActionID,
		},
	}
	s.mu.Lock()
	s.calls[rec.ID] = rt
	s.mu.Unlock()

	opts := dto.RoomOptions{SessionType: rec.SessionType}
	if err := s.deps.Media.CreateRoom(ctx, rec.ID, opts); err != nil {
		return err
	}
	cust := customerLeg(rt)
	if cust != "" {
		_ = s.deps.Media.SubscribeDTMF(ctx, rec.ID, cust, func(ctx context.Context, digit dto.DTMFDigit) {
			s.onDTMF(ctx, rec.ID, string(digit))
		})
	}
	switch node.Type {
	case "menu":
		// 仅恢复计时与按键订阅，避免重复放音。
	case "play", "business_action":
		// 由 tickIVR 按 deadline 推进或等待业务回填。
	default:
		s.runIVRNode(ctx, rec.ID)
	}
	if sess.DeadlineAt != nil && time.Now().UTC().After(*sess.DeadlineAt) {
		s.tickIVR(ctx, rec.ID)
	}
	slog.Info("IVR 会话已恢复", "call_id", rec.ID, "node_id", sess.NodeID, "flow_id", sess.FlowID)
	return nil
}

func ivrTimingFromSession(sess ports.IVRSessionView, node ivrNode) (entered time.Time, timeout time.Duration) {
	now := time.Now().UTC()
	timeout = 15 * time.Second
	switch node.Type {
	case "menu":
		sec := node.TimeoutSec
		if sec == 0 {
			sec = 8
		}
		timeout = time.Duration(sec) * time.Second
	case "play":
		sec := node.TimeoutSec
		if sec == 0 {
			sec = 2
		}
		timeout = time.Duration(sec) * time.Second
	case "business_action":
		if node.TimeoutSec > 0 {
			timeout = time.Duration(node.TimeoutSec) * time.Second
		}
	}
	entered = now
	if sess.DeadlineAt != nil && !sess.DeadlineAt.IsZero() {
		entered = sess.DeadlineAt.Add(-timeout)
	}
	return entered, timeout
}
