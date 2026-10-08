package control

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
)

type ivrRuntime struct {
	doc             ivrDoc
	flowID          string
	flowVersion     int
	node            string
	entered         time.Time
	timeout         time.Duration
	invalidAttempts int
	actionID        string
	started         time.Time
	hops            int
	awaitingAnswer  bool
}

// IVR 允许环（如“按 0 重听”），用跳转次数和总时长兜底，防止无出口的环无限占用线路。
const (
	maxIVRHops     = 100
	maxIVRDuration = 30 * time.Minute
)

type ivrDoc struct {
	Start string             `json:"start"`
	Nodes map[string]ivrNode `json:"nodes"`
}

type ivrNode struct {
	Type           string            `json:"type"`
	Action         string            `json:"action"`
	Prompt         string            `json:"prompt"`
	File           string            `json:"file"`
	TimeoutSec     int               `json:"timeout_sec"`
	MaxRetries     *int              `json:"max_retries"`
	Choices        map[string]string `json:"choices"`
	Default        string            `json:"default"`
	Invalid        string            `json:"invalid"`
	QueueID        string            `json:"queue_id"`
	SessionType    string            `json:"session_type"`
	Next           string            `json:"next"`
	Open           string            `json:"open"`
	Closed         string            `json:"closed"`
	Schedule       string            `json:"schedule,omitempty"`
	WaitingGt      int               `json:"waiting_gt,omitempty"`
	Busy           string            `json:"busy,omitempty"`
	AcceptedDigits string            `json:"accepted_digits,omitempty"`
	ResultKey      string            `json:"result_key,omitempty"`
}

func (s *Service) injectCustomerAudio(ctx context.Context, callID string, src dto.AudioSource) error {
	s.mu.Lock()
	leg := customerLeg(s.calls[callID])
	s.mu.Unlock()
	return s.deps.Media.InjectAudio(ctx, callID, leg, src)
}

// bootIVR 读取流程的最新发布快照，确保运行中的呼叫使用固定版本。
func (s *Service) bootIVR(ctx context.Context, callID, flowID string) error {
	snap, err := s.deps.Config.GetLatestIVR(ctx, flowID)
	if err != nil {
		return err
	}
	return s.startIVRPayload(ctx, callID, snap)
}

// attachIVR 根据通话所属队列找到绑定的 IVR 流程并启动。
func (s *Service) attachIVR(ctx context.Context, callID, snapshotID string) error {
	// 接口保留 snapshotID 参数；当前运行逻辑以队列绑定的最新发布版本为准。
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

// parseIVRPayload 拒绝不再支持的快照，避免运行时停留在未知节点。
func parseIVRPayload(payload string) (ivrDoc, error) {
	var doc ivrDoc
	if err := json.Unmarshal([]byte(payload), &doc); err != nil || doc.Start == "" {
		return doc, errs.InvalidRequest("IVR 快照无效")
	}
	if _, ok := doc.Nodes[doc.Start]; !ok {
		return doc, errs.InvalidRequest("IVR 起始节点不存在")
	}
	for _, node := range doc.Nodes {
		switch node.Type {
		case "play", "menu", "collect_input", "business_action", "hangup", "route_queue", "time_condition", "queue_condition", "voicemail", "tts", "asr":
		default:
			return doc, errs.InvalidRequest("不支持的 IVR 节点类型: " + node.Type)
		}
	}
	return doc, nil
}

// startIVRPayload 创建 IVR 媒体房间和机器人通话腿，并订阅客户的按键信号。
func (s *Service) startIVRPayload(ctx context.Context, callID string, snap ports.IVRSnapshot) error {
	doc, err := parseIVRPayload(snap.PayloadJSON)
	if err != nil {
		return err
	}
	opts := dto.RoomOptions{SessionType: dto.SessionTypeAudio}
	if err := s.deps.Media.CreateRoom(ctx, callID, opts); err != nil {
		return err
	}
	s.mu.Lock()
	rt := s.calls[callID]
	hasBot := false
	if rt != nil {
		for _, l := range rt.legs {
			if l.Role == dto.LegRoleIVRBot {
				hasBot = true
				break
			}
		}
	}
	s.mu.Unlock()
	if !hasBot {
		bot := ports.CallLegRecord{ID: uuid.New().String(), CallID: callID, Role: dto.LegRoleIVRBot, CreatedAt: time.Now().UTC()}
		_ = s.deps.Calls.InsertLeg(ctx, bot)
		s.mu.Lock()
		rt = s.calls[callID]
		if rt != nil {
			rt.legs = append(rt.legs, bot)
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	rt = s.calls[callID]
	if rt != nil {
		rt.ivr = &ivrRuntime{
			doc: doc, flowID: snap.FlowID, flowVersion: snap.Version,
			node: doc.Start, entered: time.Now().UTC(), timeout: 15 * time.Second,
			started: time.Now().UTC(),
		}
	}
	s.mu.Unlock()
	cust := ""
	if rt != nil {
		for _, l := range rt.legs {
			if l.Role == dto.LegRoleCustomer || l.Role == dto.LegRolePSTN {
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
	_ = s.publishCall(ctx, callID, "routing.entered_ivr", "", map[string]any{"call_id": callID, "snapshot_id": snap.SnapshotID, "flow_id": snap.FlowID})
	// 呼入 SIP 在 200 OK 之前播放的提示音话机不会播出，首个节点等应答后再执行。
	deferred := s.deps.Media.DeferUntilAnswered(callID, func() {
		actx, unlock := s.command(context.Background(), callID)
		defer unlock()
		s.beginIVR(actx, callID)
	})
	if deferred {
		s.mu.Lock()
		if rt := s.calls[callID]; rt != nil && rt.ivr != nil {
			rt.ivr.awaitingAnswer = true
		}
		s.mu.Unlock()
		return nil
	}
	s.runIVRNode(ctx, callID)
	return nil
}

// beginIVR 在呼叫应答后执行首个节点，节点计时与总时长都从应答时刻起算。
func (s *Service) beginIVR(ctx context.Context, callID string) {
	s.mu.Lock()
	rt := s.calls[callID]
	if rt == nil || rt.ivr == nil || !rt.ivr.awaitingAnswer {
		s.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	rt.ivr.awaitingAnswer = false
	rt.ivr.entered, rt.ivr.started = now, now
	s.mu.Unlock()
	s.runIVRNode(ctx, callID)
}

// OnIVRPromptFinished 由媒体层在 Inject 放音结束时回调。
//
// 对 type=play 且非循环的节点：放音一结束就跳转 next，不必再等 timeout_sec，
// 缩短「欢迎语播完到入队/下一节点」的空白。menu 等仍靠 DTMF 与 tickIVR 超时。
func (s *Service) OnIVRPromptFinished(ctx context.Context, callID string, loop bool) {
	if loop {
		return
	}
	s.mu.Lock()
	rt := s.calls[callID]
	if rt == nil || rt.ivr == nil || rt.ivr.awaitingAnswer {
		s.mu.Unlock()
		return
	}
	node, ok := rt.ivr.doc.Nodes[rt.ivr.node]
	if !ok || node.Type != "play" {
		s.mu.Unlock()
		return
	}
	next := node.Next
	s.mu.Unlock()
	if next == "" {
		_ = s.enterQueue(ctx, callID)
		return
	}
	s.gotoIVR(ctx, callID, next)
}

// tickIVR 在节点超时后沿默认或下一节点继续，没有目标时转入排队。
func (s *Service) tickIVR(ctx context.Context, callID string) {
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.ivr == nil {
		return
	}
	s.mu.Lock()
	waiting := rt.ivr.awaitingAnswer
	s.mu.Unlock()
	if waiting {
		return
	}
	if !rt.ivr.started.IsZero() && time.Since(rt.ivr.started) > maxIVRDuration {
		s.emitCall(ctx, callID, "command.failed", "", map[string]any{"call_id": callID, "reason": "ivr_timeout"})
		_ = s.Hangup(ctx, callID, dto.HangupReasonTimeout)
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
	if node.Type == "business_action" {
		if err := s.deps.BusinessActions.ExpireBusinessAction(ctx, rt.ivr.actionID); err != nil {
			return
		}
	}
	next := node.Default
	if node.Type == "play" {
		next = node.Next
	}
	if next == "" {
		_ = s.enterQueue(ctx, callID)
		return
	}
	s.gotoIVR(ctx, callID, next)
}

// onDTMF 处理菜单按键；无效输入达到重试上限后走无效（invalid）或默认（default）分支。
func (s *Service) onDTMF(ctx context.Context, callID, digit string) {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.ivr == nil || rt.rec.State != stateIVR || rt.ivr.awaitingAnswer {
		return
	}
	node := rt.ivr.doc.Nodes[rt.ivr.node]
	if node.Type == "collect_input" {
		if len(digit) == 1 && strings.Contains(node.AcceptedDigits, digit) {
			if err := s.publishCall(ctx, callID, "ivr.input_collected", rt.rec.AgentID, map[string]any{
				"call_id": callID, "agent_id": rt.rec.AgentID, "flow_id": rt.ivr.flowID,
				"flow_version": rt.ivr.flowVersion, "node_id": rt.ivr.node,
				"result_key": node.ResultKey, "input": digit,
			}); err != nil {
				return
			}
			if node.Next != "" {
				s.gotoIVR(ctx, callID, node.Next)
			} else {
				_ = s.Hangup(ctx, callID, dto.HangupReasonNormal)
			}
		}
		return
	}
	if node.Type != "menu" || node.Choices == nil {
		return
	}
	next, ok := node.Choices[digit]
	if !ok {
		rt.ivr.invalidAttempts++
		limit := 2
		if node.MaxRetries != nil {
			limit = *node.MaxRetries
		}
		if rt.ivr.invalidAttempts >= limit {
			target := node.Invalid
			if target == "" {
				target = node.Default
			}
			if target != "" {
				s.gotoIVR(ctx, callID, target)
			}
		}
		return
	}
	s.gotoIVR(ctx, callID, next)
}

// gotoIVR 切换节点并重置该节点的计时和无效输入次数。
func (s *Service) gotoIVR(ctx context.Context, callID, nodeID string) {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	s.mu.Lock()
	rt := s.calls[callID]
	if rt == nil || rt.ivr == nil {
		s.mu.Unlock()
		return
	}
	rt.ivr.hops++
	if rt.ivr.hops > maxIVRHops {
		s.mu.Unlock()
		msg := "IVR 跳转次数过多"
		s.emitCommandFailed(ctx, callID, codeIVRHopLimit, msg, "ivr_hop_limit")
		_ = s.failCall(ctx, callID, dto.HangupReasonError, codeIVRHopLimit, msg)
		return
	}
	rt.ivr.node = nodeID
	rt.ivr.entered = time.Now().UTC()
	rt.ivr.invalidAttempts = 0
	rt.ivr.actionID = ""
	s.mu.Unlock()
	s.runIVRNode(ctx, callID)
}

// runIVRNode 播放提示并执行挂断、转队列、时间判断等节点动作。
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
	if node.Type == "play" || node.Type == "menu" || (node.Type == "collect_input" && node.File != "") {
		if err := s.injectCustomerAudio(ctx, callID, dto.AudioSource{FilePath: node.File, Loop: node.Type == "menu"}); err != nil {
			s.emitCommandFailed(ctx, callID, codeIVRPromptFailed, "IVR 放音失败", "ivr_prompt")
			_ = s.hangupWithFailure(ctx, callID, dto.HangupReasonError, err)
			return
		}
	}
	if node.Type == "play" || node.Type == "menu" || node.Type == "collect_input" {
		seconds := node.TimeoutSec
		if node.Type == "play" && node.File != "" && s.deps.Media != nil {
			if d, err := s.deps.Media.PromptDuration(ctx, node.File); err == nil && d > 0 {
				seconds = playNodeWaitSeconds(seconds, d)
			}
		}
		if seconds == 0 {
			switch node.Type {
			case "menu", "collect_input":
				seconds = 8
			default:
				seconds = 2
			}
		}
		rt.ivr.timeout = time.Duration(seconds) * time.Second
		rt.ivr.entered = time.Now().UTC()
	}
	_ = s.publishCall(ctx, callID, "ivr.prompt", "", map[string]any{"call_id": callID, "prompt": node.Prompt, "type": node.Type})
	switch node.Type {
	case "business_action":
		// 暂停 IVR 媒体推进，等待业务系统通过 API 提交已声明的结果分支。
		if rt.ivr.actionID == "" {
			rt.ivr.actionID = uuid.New().String()
			rt.ivr.entered = time.Now().UTC()
			rt.ivr.timeout = time.Duration(node.TimeoutSec) * time.Second
			if err := s.deps.BusinessActions.BeginBusinessAction(ctx, ports.BusinessAction{ID: rt.ivr.actionID, CallID: callID, NodeID: rt.ivr.node, Action: node.Action, Outcomes: node.Choices, Deadline: rt.ivr.entered.Add(rt.ivr.timeout)}); err != nil {
				s.emitCommandFailed(ctx, callID, codeIVRActionFailed, "IVR 业务动作启动失败", "ivr_action")
				_ = s.hangupWithFailure(ctx, callID, dto.HangupReasonError, err)
			}
		}
	case "hangup":
		_ = s.Hangup(ctx, callID, dto.HangupReasonNormal)
	case "route_queue":
		if node.SessionType != "" {
			rt.rec.SessionType = dto.SessionType(node.SessionType)
		}
		if err := s.routeQueue(ctx, callID, node.QueueID); err != nil {
			s.emitCommandFailed(ctx, callID, codeRouteQueueFailed, "转队列失败", "route_queue")
			_ = s.hangupWithFailure(ctx, callID, dto.HangupReasonError, err)
		}
	case "time_condition":
		open := withinHoursFromSchedule(node.Schedule, s.deps.Config.Now(ctx))
		next := node.Open
		if !open {
			next = node.Closed
		}
		if next == "" {
			_ = s.Hangup(ctx, callID, dto.HangupReasonNormal)
			return
		}
		s.gotoIVR(ctx, callID, next)
	case "queue_condition":
		busy := s.queueConditionBusy(ctx, node)
		next := node.Open
		if busy {
			next = node.Busy
			if next == "" {
				next = node.Closed
			}
		}
		if next == "" {
			_ = s.enterQueue(ctx, callID)
			return
		}
		s.gotoIVR(ctx, callID, next)
	case "voicemail":
		if node.File != "" {
			_ = s.injectCustomerAudio(ctx, callID, dto.AudioSource{FilePath: node.File, Loop: false})
		}
		s.startVoicemail(ctx, callID)
	case "play":
		// 放音节点由定时 tick 在播放时长结束后推进。
	case "tts", "asr":
		s.emitCall(ctx, callID, "command.failed", "", map[string]any{"call_id": callID, "reason": "tts_asr_unsupported", "node_type": node.Type})
		next := node.Default
		if next == "" {
			_ = s.Hangup(ctx, callID, dto.HangupReasonNormal)
			return
		}
		s.gotoIVR(ctx, callID, next)
	}
}

// enterQueue 清理 IVR 状态，播放等候音并开始派单。
func (s *Service) enterQueue(ctx context.Context, callID string) error {
	if err := s.transition(ctx, callID, stateQueued); err != nil {
		return err
	}
	s.mu.Lock()
	rt := s.calls[callID]
	queueID := ""
	if rt != nil && rt.rec.QueueID != nil {
		queueID = *rt.rec.QueueID
	}
	s.mu.Unlock()
	if queueID != "" {
		_ = s.publishCall(ctx, callID, "queue.entered", "", map[string]any{"call_id": callID, "queue_id": queueID})
	}
	s.mu.Lock()
	rt = s.calls[callID]
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
		if promptFile != "" {
			_ = s.injectCustomerAudio(ctx, callID, dto.AudioSource{FilePath: promptFile, Loop: true})
		}
	}
	_ = s.cdrUpsert(ctx, callID, "queued")
	s.publishPosition(ctx, callID)
	return s.tryDispatch(ctx, callID)
}

// publishPosition 按优先级和入队时间计算同队列前方人数。
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
	_ = s.publishCall(ctx, callID, "queue.position_changed", "", map[string]any{"call_id": callID, "position": pos, "message": msg})
}

// withinHours 按队列 business_hours 判断营业窗口（入队等路径，非 IVR 节点）。
func withinHours(cfg ports.ConfigSnapshotPort, ctx context.Context, queueID string) bool {
	h, err := cfg.GetBusinessHours(ctx, queueID)
	if err != nil {
		return false
	}
	return withinHoursFromSchedule(h.WeekdayHours, cfg.Now(ctx))
}

func (s *Service) queueConditionBusy(ctx context.Context, node ivrNode) bool {
	if node.QueueID == "" {
		return true
	}
	st, err := s.deps.Config.QueueStatus(ctx, node.QueueID)
	if err != nil {
		return true
	}
	threshold := node.WaitingGt
	if threshold < 0 {
		threshold = 0
	}
	if int(st.Waiting) > threshold {
		return true
	}
	return st.AvailableAgents == 0
}

// CompleteBusinessAction 仅接受流程中已声明的业务结果，并驱动 IVR 跳转。
func (s *Service) CompleteBusinessAction(ctx context.Context, callID, actionID, outcome string) error {
	ctx, unlock := s.command(ctx, callID)
	defer unlock()
	action, err := s.deps.BusinessActions.GetBusinessAction(ctx, actionID)
	if err != nil {
		return err
	}
	if action.CallID != callID {
		return errs.NotFound("业务动作不存在")
	}
	if action.Status == "completed" {
		if action.Outcome == outcome {
			return nil
		}
		return errs.Conflict("业务动作已提交其他结果", "")
	}
	if _, err := s.GetCall(ctx, callID); err != nil {
		return err
	}
	s.mu.Lock()
	rt := s.calls[callID]
	s.mu.Unlock()
	if rt == nil || rt.ivr == nil || rt.rec.State != stateIVR || rt.ivr.actionID != actionID {
		return errs.Conflict("业务动作已结束", "")
	}
	next, ok := action.Outcomes[outcome]
	if !ok {
		return errs.InvalidRequest("业务结果未在流程中声明")
	}
	if err := s.deps.BusinessActions.ResolveBusinessAction(ctx, actionID, outcome); err != nil {
		return err
	}
	s.gotoIVR(ctx, callID, next)
	return nil
}

// routeQueue 在进入新的排队周期前应用已固定的队列目标配置。
func (s *Service) routeQueue(ctx context.Context, callID, queueID string) error {
	q, err := s.deps.Config.GetQueue(ctx, queueID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	rt := s.calls[callID]
	if rt == nil {
		s.mu.Unlock()
		return errs.NotFound("通话不存在")
	}
	rt.rec.QueueID = new(queueID)
	rt.queueName = q.Name
	rt.postCallIVRFlowID = q.PostCallIVRFlowID
	rt.callee = q.Name
	rt.waitPrompt = q.WaitPrompt
	rt.maxWait = time.Duration(q.MaxWaitSec) * time.Second
	if !q.PriorityEnabled {
		rt.rec.Priority = 0
	}
	s.mu.Unlock()
	return s.enterQueue(ctx, callID)
}

// playNodeWaitSeconds 保证放音节点等待时间覆盖素材时长（含异步开播与尾帧余量）。
func playNodeWaitSeconds(configuredSec int, promptDur time.Duration) int {
	need := promptDur + 600*time.Millisecond
	minSec := int((need + time.Second - 1) / time.Second)
	if minSec < 1 {
		minSec = 1
	}
	if configuredSec < minSec {
		return minSec
	}
	return configuredSec
}
