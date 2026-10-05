package cccore

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
	"open-switch/internal/store/models"
)

func (s *Service) versionFor(ctx context.Context) (int64, error) {
	if version := scope.ConfigVersion(ctx); version > 0 {
		return version, nil
	}
	return activeVersion(s.db.WithContext(ctx))
}

// ActiveVersion 实现 ports.ConfigSnapshotPort。
func (s *Service) ActiveVersion(ctx context.Context) (int64, error) {
	return activeVersion(s.db.WithContext(ctx))
}

func (s *Service) GetQueue(ctx context.Context, queueID string) (ports.QueueSnapshot, error) {
	version, err := s.versionFor(ctx)
	if err != nil {
		return ports.QueueSnapshot{}, err
	}
	var row models.Queue
	if err := s.db.WithContext(ctx).Where("config_version = ? AND id = ?", version, queueID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.QueueSnapshot{}, errs.NotFound("队列不存在")
		}
		return ports.QueueSnapshot{}, err
	}
	var skillIDs []string
	if err := s.db.WithContext(ctx).Model(&models.QueueSkill{}).Where("config_version = ? AND queue_id = ?", version, queueID).Pluck("skill_id", &skillIDs).Error; err != nil {
		return ports.QueueSnapshot{}, err
	}
	out := ports.QueueSnapshot{ConfigVersion: version, ID: row.ID, Name: row.Name, VideoEnabled: row.VideoEnabled, MaxWaitSec: row.MaxWaitSec, IVRFlowID: derefString(row.IVRFlowID), PostCallIVRFlowID: derefString(row.PostCallIVRFlowID), OverflowAction: row.OverflowAction, OverflowQueueID: derefString(row.OverflowQueueID), WaitPrompt: row.WaitPrompt, AudioProfile: row.AudioProfile, AnnounceRecording: row.AnnounceRecording, SkillIDs: skillIDs, AfterHoursAction: row.AfterHoursAction, ForceHangupOnCheckout: row.ForceHangupOnCheckout, ListenAnnounce: row.ListenAnnounce, PriorityEnabled: row.PriorityEnabled}
	return out, nil
}

func (s *Service) GetLatestIVR(ctx context.Context, flowID string) (ports.IVRSnapshot, error) {
	version, err := s.versionFor(ctx)
	if err != nil {
		return ports.IVRSnapshot{}, err
	}
	var row models.IVRPublishedSnapshot
	if err := s.db.WithContext(ctx).Where("config_version = ? AND flow_id = ?", version, flowID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRSnapshot{}, errs.NotFound("IVR 快照不存在")
		}
		return ports.IVRSnapshot{}, err
	}
	return ports.IVRSnapshot{ConfigVersion: version, SnapshotID: row.FlowID, FlowID: row.FlowID, Version: row.Version, PayloadJSON: row.PayloadJSON}, nil
}

func (s *Service) GetIVRSnapshot(ctx context.Context, configVersion int64, flowID string, flowVersion int) (ports.IVRSnapshot, error) {
	if configVersion < 1 {
		var err error
		configVersion, err = s.versionFor(ctx)
		if err != nil {
			return ports.IVRSnapshot{}, err
		}
	}
	var row models.IVRPublishedSnapshot
	q := s.db.WithContext(ctx).Where("config_version = ? AND flow_id = ?", configVersion, flowID)
	if flowVersion > 0 {
		q = q.Where("version = ?", flowVersion)
	}
	if err := q.Order("version DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRSnapshot{}, errs.NotFound("IVR 快照不存在")
		}
		return ports.IVRSnapshot{}, err
	}
	return ports.IVRSnapshot{ConfigVersion: configVersion, SnapshotID: row.FlowID, FlowID: row.FlowID, Version: row.Version, PayloadJSON: row.PayloadJSON}, nil
}

func (s *Service) GetBusinessHours(ctx context.Context, queueID string) (ports.BusinessHours, error) {
	version, err := s.versionFor(ctx)
	if err != nil {
		return ports.BusinessHours{}, err
	}
	var row models.Queue
	if err := s.db.WithContext(ctx).Where("config_version = ? AND id = ?", version, queueID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.BusinessHours{}, errs.NotFound("队列不存在")
		}
		return ports.BusinessHours{}, err
	}
	return ports.BusinessHours{Timezone: "UTC", WeekdayHours: row.BusinessHoursJSON}, nil
}

func (s *Service) ResolveDID(ctx context.Context, trunkID, did string) (ports.DIDRouteSnapshot, error) {
	did = NormalizeDID(did)
	if did == "" {
		return ports.DIDRouteSnapshot{}, errs.InvalidRequest("DID 无效")
	}
	if trunkID == "" {
		trunkID = "*"
	}
	type row struct {
		ConfigVersion int64
		ID            string
		TrunkID       string
		DID           string
		TargetType    string
		TargetID      *string
	}
	var found row
	q := s.db.WithContext(ctx).Raw(`SELECT d.config_version, d.id, d.trunk_id,
 d.normalized_did AS did, d.target_type, d.target_id
FROM os_did_routes d
JOIN os_active_config a ON a.version = d.config_version
WHERE d.normalized_did = ? AND d.trunk_id IN (?, '*') 
ORDER BY CASE WHEN d.trunk_id = ? THEN 0 ELSE 1 END
LIMIT 1`, did, trunkID, trunkID)
	if err := q.Scan(&found).Error; err != nil {
		return ports.DIDRouteSnapshot{}, err
	}
	if found.ID == "" {
		return ports.DIDRouteSnapshot{}, errs.NotFound("DID 未配置")
	}
	return ports.DIDRouteSnapshot{ConfigVersion: found.ConfigVersion, RouteID: found.ID, TrunkID: found.TrunkID, DID: found.DID, TargetType: found.TargetType, TargetID: derefString(found.TargetID)}, nil
}

func (s *Service) Now(context.Context) time.Time { return time.Now().UTC() }

// RequestAgent 原子地预留一名符合条件且已签入的坐席。
func (s *Service) RequestAgent(ctx context.Context, req dto.DispatchRequest) (dto.DispatchResult, error) {
	version, err := s.versionFor(ctx)
	if err != nil {
		return dto.DispatchResult{}, err
	}
	if req.CallID == "" || req.QueueID == "" {
		return dto.DispatchResult{}, errs.InvalidRequest("call_id/queue_id 必填")
	}
	var queue models.Queue
	if err := s.db.WithContext(ctx).Where("config_version = ? AND id = ?", version, req.QueueID).First(&queue).Error; err != nil {
		return dto.DispatchResult{}, err
	}
	var picked string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", req.CallID).Error; err != nil {
			return err
		}
		if err := tx.Raw("SELECT agent_id FROM os_agent_sessions WHERE current_call_id = ? AND state IN ('ringing','on_call') LIMIT 1", req.CallID).Scan(&picked).Error; err != nil {
			return err
		}
		if picked != "" {
			return nil
		}
		videoClause := ""
		order := "s.updated_at, s.agent_id"
		args := []any{version, req.QueueID, version, req.QueueID}
		extra := ""
		for _, skillID := range req.SkillIDs {
			extra += " AND EXISTS (SELECT 1 FROM os_agent_skills ask WHERE ask.config_version = ? AND ask.agent_id = s.agent_id AND ask.skill_id = ?)"
			args = append(args, version, skillID)
		}
		if queue.DispatchStrategy == "round_robin" {
			if err := tx.Exec("INSERT INTO os_queue_dispatch_cursor (queue_id, last_agent_id) VALUES (?, NULL) ON CONFLICT DO NOTHING", req.QueueID).Error; err != nil {
				return err
			}
			var last string
			if err := tx.Raw("SELECT last_agent_id FROM os_queue_dispatch_cursor WHERE queue_id = ? FOR UPDATE", req.QueueID).Scan(&last).Error; err != nil {
				return err
			}
			order = "CASE WHEN s.agent_id::text > ? THEN 0 ELSE 1 END, s.agent_id"
			args = append(args, last)
		}
		if req.RequireVideo {
			videoClause = " AND a.video_capable = TRUE"
		}
		query := `SELECT s.agent_id
FROM os_agent_sessions s
JOIN os_agents a ON a.config_version = ? AND a.id = s.agent_id
JOIN os_agent_session_queues sq ON sq.agent_id = s.agent_id
WHERE sq.queue_id = ? AND s.state = 'idle' AND s.pending_checkout = FALSE AND a.enabled = TRUE` + videoClause + `
AND EXISTS (SELECT 1 FROM os_queue_agents qa WHERE qa.config_version = a.config_version AND qa.queue_id = sq.queue_id AND qa.agent_id = s.agent_id)
AND NOT EXISTS (
 SELECT 1 FROM os_queue_skills qs
 WHERE qs.config_version = ? AND qs.queue_id = ?
 AND NOT EXISTS (SELECT 1 FROM os_agent_skills ags WHERE ags.config_version = qs.config_version AND ags.agent_id = s.agent_id AND ags.skill_id = qs.skill_id)
)
` + extra + " ORDER BY " + order + " LIMIT 1 FOR UPDATE OF s SKIP LOCKED"
		// 查询参数顺序须与 JOIN 及 WHERE 条件一致。
		if err := tx.Raw(query, args...).Scan(&picked).Error; err != nil {
			return err
		}
		if picked == "" {
			return nil
		}
		now := time.Now().UTC()
		res := tx.Model(&models.AgentSession{}).Where("agent_id = ? AND state = 'idle'", picked).Updates(map[string]any{"state": "ringing", "current_call_id": req.CallID, "busy_reason": "", "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			picked = ""
			return nil
		}
		if err := tx.Create(&models.AgentStateLog{ID: uuid.New().String(), AgentID: picked, FromState: "idle", ToState: "ringing", Reason: "acd", CallID: req.CallID, CreatedAt: now}).Error; err != nil {
			return err
		}
		if queue.DispatchStrategy == "round_robin" {
			if err := tx.Exec("UPDATE os_queue_dispatch_cursor SET last_agent_id = ? WHERE queue_id = ?", picked, req.QueueID).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`INSERT INTO os_acd_attempts (id, call_id, queue_id, agent_id, attempt, state, started_at)
 SELECT ?, ?, ?, ?, COALESCE(MAX(attempt), 0) + 1, 'offering', ? FROM os_acd_attempts WHERE call_id = ?`,
			uuid.New().String(), req.CallID, req.QueueID, picked, now, req.CallID).Error; err != nil {
			return err
		}
		if err := s.publishAgentTx(ctx, tx, req.CallID, picked, "ringing", "acd"); err != nil {
			return err
		}
		return s.events.WithDB(tx).PublishCallEvent(ctx, ports.CallEvent{
			CallID: req.CallID, AgentID: picked, Type: "acd.agent_reserved",
			Payload: map[string]any{"call_id": req.CallID, "queue_id": req.QueueID, "agent_id": picked},
		})
	})
	if err != nil {
		return dto.DispatchResult{}, err
	}
	return dto.DispatchResult{AgentID: picked}, nil
}

func (s *Service) ByExtension(ctx context.Context, extension string) (ports.AgentInfo, error) {
	return s.findAgent(ctx, "extension = ?", extension)
}

func (s *Service) ByID(ctx context.Context, agentID string) (ports.AgentInfo, error) {
	return s.findAgent(ctx, "id = ?", agentID)
}

func (s *Service) findAgent(ctx context.Context, predicate string, value any) (ports.AgentInfo, error) {
	version, err := s.versionFor(ctx)
	if err != nil {
		return ports.AgentInfo{}, err
	}
	var row models.Agent
	if err := s.db.WithContext(ctx).Where("config_version = ? AND "+predicate, version, value).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.AgentInfo{}, errs.NotFound("坐席不存在")
		}
		return ports.AgentInfo{}, err
	}
	if !row.Enabled {
		return ports.AgentInfo{}, errs.Forbidden("坐席已禁用")
	}
	state := "offline"
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("agent_id = ?", row.ID).First(&sess).Error; err == nil {
		state = sess.State
	}
	return ports.AgentInfo{ConfigVersion: version, TerminalType: row.TerminalType, SIPUsername: row.SIPUsername, AgentID: row.ID, UserID: row.UserID, Extension: row.Extension, VideoCapable: row.VideoCapable, DisplayName: row.DisplayName, State: state}, nil
}

// SetCallState 是呼叫控制侧的 SetState：单独携带 callID，避免状态迁移依赖编码在 reason 里的文本。
func (s *Service) SetCallState(ctx context.Context, callID, agentID, fromState, toState, reason string) error {
	return s.setState(ctx, callID, agentID, fromState, toState, reason)
}

func (s *Service) setState(ctx context.Context, callID, agentID, fromState, toState, reason string) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sess models.AgentSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("agent_id = ?", agentID).First(&sess).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.Conflict("坐席未签入", errs.CodeAgentNotIdle)
			}
			return err
		}
		if reason == "force-check-out" && toState == "offline" && sess.CurrentCallID != "" {
			return tx.Model(&sess).Update("pending_checkout", true).Error
		}
		if callID != "" && sess.CurrentCallID != "" && sess.CurrentCallID != callID {
			var old models.Call
			err := tx.Where("id = ?", sess.CurrentCallID).First(&old).Error
			stale := errors.Is(err, gorm.ErrRecordNotFound) || old.State == "ended"
			if stale {
				sess.CurrentCallID = ""
				if err := tx.Model(&sess).Update("current_call_id", nil).Error; err != nil {
					return err
				}
			} else if toState == "idle" || toState == "acw" || toState == "busy" || toState == "offline" {
				return nil
			} else {
				return errs.Conflict("坐席正在处理另一通话", errs.CodeAgentBusy)
			}
		}
		if fromState != "" && sess.State != fromState {
			return errs.Conflict("坐席状态已变更", errs.CodeAgentNotIdle)
		}
		if callID == "" {
			callID = sess.CurrentCallID
		}
		current := callID
		if toState == "idle" || toState == "acw" || toState == "busy" || toState == "offline" {
			current = ""
		}
		if sess.PendingCheckout && current == "" {
			toState = "offline"
		}
		now := time.Now().UTC()
		if toState == "offline" {
			if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentSessionQueue{}).Error; err != nil {
				return err
			}
			if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentSession{}).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&sess).Updates(map[string]any{"state": toState, "current_call_id": nullableUUID(current), "busy_reason": reason, "pending_checkout": false, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.AgentStateLog{ID: uuid.New().String(), AgentID: agentID, FromState: sess.State, ToState: toState, Reason: reason, CallID: callID, CreatedAt: now}).Error; err != nil {
			return err
		}
		if callID != "" {
			state := "ended"
			if toState == "on_call" {
				state = "connected"
			}
			var ended any = now
			if state == "connected" {
				ended = nil
			}
			if err := tx.Exec("UPDATE os_acd_attempts SET state=?,ended_at=?,failure_reason=? WHERE call_id=? AND agent_id=? AND state IN ('offering','connected')", state, ended, reason, callID, agentID).Error; err != nil {
				return err
			}
		}
		return s.publishAgentTx(ctx, tx, callID, agentID, toState, reason)
	})
	return err
}

func (s *Service) CheckIn(ctx context.Context, agentID string, queueIDs []string) (ports.AgentSessionView, error) {
	version, err := s.versionFor(ctx)
	if err != nil {
		return ports.AgentSessionView{}, err
	}
	if _, err := s.ByID(ctx, agentID); err != nil {
		return ports.AgentSessionView{}, err
	}
	if len(queueIDs) == 0 {
		if err := s.db.WithContext(ctx).Model(&models.QueueAgent{}).Where("config_version = ? AND agent_id = ?", version, agentID).Pluck("queue_id", &queueIDs).Error; err != nil {
			return ports.AgentSessionView{}, err
		}
	}
	if len(queueIDs) == 0 {
		return ports.AgentSessionView{}, errs.InvalidRequest("没有可签入的队列")
	}
	now := time.Now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "agent:"+agentID).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, queueID := range queueIDs {
			if seen[queueID] {
				return errs.InvalidRequest("队列重复")
			}
			seen[queueID] = true
			var count int64
			if err := tx.Model(&models.QueueAgent{}).Where("config_version = ? AND queue_id = ? AND agent_id = ?", version, queueID, agentID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return errs.Forbidden("坐席未绑定该队列")
			}
		}
		var sess models.AgentSession
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("agent_id = ?", agentID).First(&sess).Error
		if err == nil && (sess.State == "ringing" || sess.State == "on_call") {
			return errs.Conflict("振铃或通话中不能重新签入", errs.CodeAgentBusy)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentSessionQueue{}).Error; err != nil {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			sess = models.AgentSession{ID: uuid.New().String(), AgentID: agentID, State: "idle", CheckedInAt: now, UpdatedAt: now}
			if err := tx.Create(&sess).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&sess).Updates(map[string]any{"state": "idle", "busy_reason": "", "current_call_id": nil, "pending_checkout": false, "checked_in_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		for _, queueID := range queueIDs {
			if err := tx.Create(&models.AgentSessionQueue{AgentID: agentID, QueueID: queueID}).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&models.AgentStateLog{ID: uuid.New().String(), AgentID: agentID, FromState: "offline", ToState: "idle", Reason: "check-in", CreatedAt: now}).Error; err != nil {
			return err
		}
		return s.publishAgentTx(ctx, tx, "", agentID, "idle", "check-in")
	})
	if err != nil {
		return ports.AgentSessionView{}, err
	}
	return s.AgentSession(ctx, agentID)
}

func (s *Service) CheckOut(ctx context.Context, agentID string) error {
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if sess.State == "ringing" || sess.State == "on_call" {
		return errs.Conflict("振铃或通话中不能签出", errs.CodeAgentBusy)
	}
	return s.setState(ctx, "", agentID, sess.State, "offline", "check-out")
}

func (s *Service) SetPresence(ctx context.Context, agentID, state, reason string) (ports.AgentSessionView, error) {
	if state != "idle" && state != "busy" && state != "acw" {
		return ports.AgentSessionView{}, errs.InvalidRequest("只允许设置 idle、busy 或 acw")
	}
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.AgentSessionView{}, errs.Conflict("请先签入", errs.CodeAgentNotIdle)
		}
		return ports.AgentSessionView{}, err
	}
	if sess.State == "ringing" || sess.State == "on_call" {
		return ports.AgentSessionView{}, errs.Conflict("振铃或通话中不能切换状态", errs.CodeAgentBusy)
	}
	if err := s.setState(ctx, "", agentID, sess.State, state, reason); err != nil {
		return ports.AgentSessionView{}, err
	}
	return s.AgentSession(ctx, agentID)
}

func (s *Service) AgentSession(ctx context.Context, agentID string) (ports.AgentSessionView, error) {
	out := ports.AgentSessionView{AgentID: agentID, State: "offline", QueueIDs: []string{}}
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, nil
		}
		return out, err
	}
	out.State, out.BusyReason, out.CurrentCallID = sess.State, sess.BusyReason, sess.CurrentCallID
	if err := s.db.WithContext(ctx).Model(&models.AgentSessionQueue{}).Where("agent_id = ?", agentID).Pluck("queue_id", &out.QueueIDs).Error; err != nil {
		return ports.AgentSessionView{}, err
	}
	return out, nil
}

// ListAgentSessions 一次返回所有已签入坐席的会话，未出现的坐席视为 offline。
func (s *Service) ListAgentSessions(ctx context.Context) ([]ports.AgentSessionView, error) {
	var sessions []models.AgentSession
	if err := s.db.WithContext(ctx).Order("agent_id").Find(&sessions).Error; err != nil {
		return nil, err
	}
	var links []models.AgentSessionQueue
	if err := s.db.WithContext(ctx).Find(&links).Error; err != nil {
		return nil, err
	}
	queues := make(map[string][]string, len(sessions))
	for _, l := range links {
		queues[l.AgentID] = append(queues[l.AgentID], l.QueueID)
	}
	out := make([]ports.AgentSessionView, 0, len(sessions))
	for _, sess := range sessions {
		qids := queues[sess.AgentID]
		if qids == nil {
			qids = []string{}
		}
		out = append(out, ports.AgentSessionView{AgentID: sess.AgentID, State: sess.State, BusyReason: sess.BusyReason, CurrentCallID: sess.CurrentCallID, QueueIDs: qids})
	}
	return out, nil
}

func (s *Service) QueueStatus(ctx context.Context, queueID string) (ports.QueueStatusView, error) {
	if _, err := s.GetQueue(ctx, queueID); err != nil {
		return ports.QueueStatusView{}, err
	}
	out := ports.QueueStatusView{QueueID: queueID}
	if err := s.db.WithContext(ctx).Model(&struct{ CallID string }{}).Table("os_queue_entries").Where("queue_id = ? AND state = 'waiting'", queueID).Count(&out.Waiting).Error; err != nil {
		return out, err
	}
	base := s.db.WithContext(ctx).Table("os_agent_sessions s").Joins("JOIN os_agent_session_queues sq ON sq.agent_id=s.agent_id").Where("sq.queue_id = ?", queueID)
	if err := base.Session(&gorm.Session{}).Where("s.state = 'idle'").Count(&out.AvailableAgents).Error; err != nil {
		return out, err
	}
	if err := base.Session(&gorm.Session{}).Where("s.state = 'ringing'").Count(&out.RingingAgents).Error; err != nil {
		return out, err
	}
	if err := base.Session(&gorm.Session{}).Where("s.state IN ('on_call','busy')").Count(&out.BusyAgents).Error; err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) notifyMessageForMode(mode string) string {
	switch mode {
	case "video_composite":
		return s.options.VideoNotifyMessage
	case "audio":
		return s.options.AudioNotifyMessage
	default:
		return ""
	}
}

func (s *Service) ForQueue(ctx context.Context, queueID string) (dto.RecordingPolicy, error) {
	out := dto.RecordingPolicy{Mode: s.options.RecordingMode}
	out.NotifyMessage = s.notifyMessageForMode(out.Mode)
	if queueID == "" {
		return out, nil
	}
	q, err := s.GetQueue(ctx, queueID)
	if err != nil {
		return dto.RecordingPolicy{}, err
	}
	version, _ := s.versionFor(ctx)
	var row models.Queue
	if err := s.db.WithContext(ctx).Where("config_version = ? AND id = ?", version, q.ID).First(&row).Error; err != nil {
		return dto.RecordingPolicy{}, err
	}
	out.Mode, out.NotifyGuest = row.RecordingPolicy, row.AnnounceRecording
	out.NotifyMessage = s.notifyMessageForMode(out.Mode)
	return out, nil
}

// NotifyMessageForMode 返回指定录制模式的告知文案。
func (s *Service) NotifyMessageForMode(_ context.Context, mode string) string {
	return s.notifyMessageForMode(mode)
}

func (s *Service) publishAgentTx(ctx context.Context, tx *gorm.DB, callID, agentID, state, reason string) error {
	return s.events.WithDB(tx).PublishCallEvent(ctx, ports.CallEvent{CallID: callID, AgentID: agentID, TargetOnly: true, Type: "agent.routing_state_changed", Payload: map[string]any{"agent_id": agentID, "state": state, "busy_reason": reason, "reason": reason, "call_id": callID}})
}

func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// RecoverReservations 进程恢复后释放已随通话结束但仍占用的坐席预留。
func (s *Service) RecoverReservations(ctx context.Context) error {
	var rows []models.AgentSession
	if err := s.db.WithContext(ctx).Raw(`SELECT s.* FROM os_agent_sessions s
 LEFT JOIN os_calls c ON c.id=s.current_call_id
 WHERE s.current_call_id IS NOT NULL AND (c.id IS NULL OR c.state='ended')`).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		next := "idle"
		if row.State == "on_call" {
			next = "acw"
		}
		if err := s.setState(ctx, row.CurrentCallID, row.AgentID, row.State, next, "process_recovery"); err != nil {
			return err
		}
	}
	return nil
}
