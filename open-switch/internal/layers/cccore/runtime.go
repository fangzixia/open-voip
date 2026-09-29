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
	"open-switch/internal/store"
	"open-switch/internal/store/models"
)

func (s *Service) versionFor(ctx context.Context, appID string) (int64, error) {
	if version := scope.ConfigVersion(ctx); version > 0 {
		return version, nil
	}
	return activeVersion(s.db.WithContext(ctx), appID)
}

func (s *Service) GetQueue(ctx context.Context, queueID string) (ports.QueueSnapshot, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.QueueSnapshot{}, err
	}
	version, err := s.versionFor(ctx, appID)
	if err != nil {
		return ports.QueueSnapshot{}, err
	}
	var row models.Queue
	if err := s.db.WithContext(ctx).Where("application_id = ? AND config_version = ? AND id = ?", appID, version, queueID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.QueueSnapshot{}, errs.NotFound("队列不存在")
		}
		return ports.QueueSnapshot{}, err
	}
	var skillIDs []string
	if err := s.db.WithContext(ctx).Model(&models.QueueSkill{}).Where("application_id = ? AND config_version = ? AND queue_id = ?", appID, version, queueID).Pluck("skill_id", &skillIDs).Error; err != nil {
		return ports.QueueSnapshot{}, err
	}
	out := ports.QueueSnapshot{ApplicationID: appID, ConfigVersion: version, ID: row.ID, Name: row.Name, VideoEnabled: row.VideoEnabled, MaxWaitSec: row.MaxWaitSec, IVRFlowID: derefString(row.IVRFlowID), OverflowAction: row.OverflowAction, OverflowQueueID: derefString(row.OverflowQueueID), WaitPrompt: row.WaitPrompt, AnnounceRecording: row.AnnounceRecording, SkillIDs: skillIDs, AfterHoursAction: row.AfterHoursAction, ForceHangupOnCheckout: row.ForceHangupOnCheckout, ListenAnnounce: row.ListenAnnounce, PriorityEnabled: row.PriorityEnabled}
	return out, nil
}

func (s *Service) GetLatestIVR(ctx context.Context, flowID string) (ports.IVRSnapshot, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.IVRSnapshot{}, err
	}
	version, err := s.versionFor(ctx, appID)
	if err != nil {
		return ports.IVRSnapshot{}, err
	}
	var row models.IVRPublishedSnapshot
	if err := s.db.WithContext(ctx).Where("application_id = ? AND config_version = ? AND flow_id = ?", appID, version, flowID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRSnapshot{}, errs.NotFound("IVR 快照不存在")
		}
		return ports.IVRSnapshot{}, err
	}
	return ports.IVRSnapshot{ApplicationID: appID, ConfigVersion: version, SnapshotID: row.FlowID, FlowID: row.FlowID, Version: row.Version, PayloadJSON: row.PayloadJSON}, nil
}

func (s *Service) GetIVRSnapshot(ctx context.Context, configVersion int64, flowID string, flowVersion int) (ports.IVRSnapshot, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.IVRSnapshot{}, err
	}
	if configVersion < 1 {
		configVersion, err = s.versionFor(ctx, appID)
		if err != nil {
			return ports.IVRSnapshot{}, err
		}
	}
	var row models.IVRPublishedSnapshot
	q := s.db.WithContext(ctx).Where("application_id = ? AND config_version = ? AND flow_id = ?", appID, configVersion, flowID)
	if flowVersion > 0 {
		q = q.Where("version = ?", flowVersion)
	}
	if err := q.Order("version DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRSnapshot{}, errs.NotFound("IVR 快照不存在")
		}
		return ports.IVRSnapshot{}, err
	}
	return ports.IVRSnapshot{ApplicationID: appID, ConfigVersion: configVersion, SnapshotID: row.FlowID, FlowID: row.FlowID, Version: row.Version, PayloadJSON: row.PayloadJSON}, nil
}

func (s *Service) GetBusinessHours(ctx context.Context, queueID string) (ports.BusinessHours, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.BusinessHours{}, err
	}
	version, err := s.versionFor(ctx, appID)
	if err != nil {
		return ports.BusinessHours{}, err
	}
	var row models.Queue
	if err := s.db.WithContext(ctx).Where("application_id = ? AND config_version = ? AND id = ?", appID, version, queueID).First(&row).Error; err != nil {
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
		ApplicationID string
		ConfigVersion int64
		ID            string
		TrunkID       string
		DID           string
		TargetType    string
		TargetID      *string
	}
	var found row
	q := s.db.WithContext(ctx).Raw(`SELECT d.application_id, d.config_version, d.id, d.trunk_id,
 d.normalized_did AS did, d.target_type, d.target_id
FROM os_did_routes d
JOIN os_active_config a ON a.application_id = d.application_id AND a.version = d.config_version
WHERE d.normalized_did = ? AND d.trunk_id IN (?, '*') AND (? = '' OR d.application_id = ?)
ORDER BY CASE WHEN d.trunk_id = ? THEN 0 ELSE 1 END
LIMIT 1`, did, trunkID, scope.Application(ctx), scope.Application(ctx), trunkID)
	if err := q.Scan(&found).Error; err != nil {
		return ports.DIDRouteSnapshot{}, err
	}
	if found.ID == "" {
		return ports.DIDRouteSnapshot{}, errs.NotFound("DID 未配置")
	}
	return ports.DIDRouteSnapshot{ApplicationID: found.ApplicationID, ConfigVersion: found.ConfigVersion, RouteID: found.ID, TrunkID: found.TrunkID, DID: found.DID, TargetType: found.TargetType, TargetID: derefString(found.TargetID)}, nil
}

func (s *Service) Now(context.Context) time.Time { return time.Now().UTC() }

// RequestAgent 原子地预留一名符合条件且已签入的坐席。
func (s *Service) RequestAgent(ctx context.Context, req dto.DispatchRequest) (dto.DispatchResult, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return dto.DispatchResult{}, err
	}
	version, err := s.versionFor(ctx, appID)
	if err != nil {
		return dto.DispatchResult{}, err
	}
	if req.CallID == "" || req.QueueID == "" {
		return dto.DispatchResult{}, errs.InvalidRequest("call_id/queue_id 必填")
	}
	var queue models.Queue
	if err := s.db.WithContext(ctx).Where("application_id = ? AND config_version = ? AND id = ?", appID, version, req.QueueID).First(&queue).Error; err != nil {
		return dto.DispatchResult{}, err
	}
	var picked string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", req.CallID).Error; err != nil {
			return err
		}
		if err := tx.Raw("SELECT agent_id FROM os_agent_sessions WHERE application_id = ? AND current_call_id = ? AND state IN ('ringing','on_call') LIMIT 1", appID, req.CallID).Scan(&picked).Error; err != nil {
			return err
		}
		if picked != "" {
			return nil
		}
		videoClause := ""
		order := "s.updated_at, s.agent_id"
		args := []any{version, appID, req.QueueID, appID, version, req.QueueID}
		extra := ""
		for _, skillID := range req.SkillIDs {
			extra += " AND EXISTS (SELECT 1 FROM os_agent_skills ask WHERE ask.application_id = s.application_id AND ask.config_version = ? AND ask.agent_id = s.agent_id AND ask.skill_id = ?)"
			args = append(args, version, skillID)
		}
		if queue.DispatchStrategy == "round_robin" {
			if err := tx.Exec("INSERT INTO os_queue_dispatch_cursor (application_id,queue_id,last_agent_id) VALUES (?,?,'') ON CONFLICT DO NOTHING", appID, req.QueueID).Error; err != nil {
				return err
			}
			var last string
			if err := tx.Raw("SELECT last_agent_id FROM os_queue_dispatch_cursor WHERE application_id = ? AND queue_id = ? FOR UPDATE", appID, req.QueueID).Scan(&last).Error; err != nil {
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
JOIN os_agents a ON a.application_id = s.application_id AND a.config_version = ? AND a.id = s.agent_id
JOIN os_agent_session_queues sq ON sq.application_id = s.application_id AND sq.agent_id = s.agent_id
WHERE s.application_id = ? AND sq.queue_id = ? AND s.state = 'idle' AND s.pending_checkout = FALSE AND a.enabled = TRUE` + videoClause + `
AND EXISTS (SELECT 1 FROM os_queue_agents qa WHERE qa.application_id = s.application_id AND qa.config_version = a.config_version AND qa.queue_id = sq.queue_id AND qa.agent_id = s.agent_id)
AND NOT EXISTS (
 SELECT 1 FROM os_queue_skills qs
 WHERE qs.application_id = ? AND qs.config_version = ? AND qs.queue_id = ?
 AND NOT EXISTS (SELECT 1 FROM os_agent_skills ags WHERE ags.application_id = qs.application_id AND ags.config_version = qs.config_version AND ags.agent_id = s.agent_id AND ags.skill_id = qs.skill_id)
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
		res := tx.Model(&models.AgentSession{}).Where("application_id = ? AND agent_id = ? AND state = 'idle'", appID, picked).Updates(map[string]any{"state": "ringing", "current_call_id": req.CallID, "busy_reason": "", "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			picked = ""
			return nil
		}
		if err := tx.Create(&models.AgentStateLog{ID: uuid.NewString(), ApplicationID: appID, AgentID: picked, FromState: "idle", ToState: "ringing", Reason: "acd", CallID: req.CallID, CreatedAt: now}).Error; err != nil {
			return err
		}
		if queue.DispatchStrategy == "round_robin" {
			if err := tx.Exec("UPDATE os_queue_dispatch_cursor SET last_agent_id = ? WHERE application_id = ? AND queue_id = ?", picked, appID, req.QueueID).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`INSERT INTO os_acd_attempts (id,application_id,call_id,queue_id,agent_id,attempt,state,started_at)
 SELECT ?,?,?,?,?,COALESCE(MAX(attempt),0)+1,'offering',? FROM os_acd_attempts WHERE call_id=?`,
			uuid.NewString(), appID, req.CallID, req.QueueID, picked, now, req.CallID).Error; err != nil {
			return err
		}
		if err := publishAgentTx(ctx, tx, req.CallID, picked, "ringing", "acd"); err != nil {
			return err
		}
		return (store.CallEvents{DB: tx}).PublishCallEvent(ctx, ports.CallEvent{
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
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.AgentInfo{}, err
	}
	version, err := s.versionFor(ctx, appID)
	if err != nil {
		return ports.AgentInfo{}, err
	}
	var row models.Agent
	if err := s.db.WithContext(ctx).Where("application_id = ? AND config_version = ? AND "+predicate, appID, version, value).First(&row).Error; err != nil {
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
	if err := s.db.WithContext(ctx).Where("application_id = ? AND agent_id = ?", appID, row.ID).First(&sess).Error; err == nil {
		state = sess.State
	}
	return ports.AgentInfo{ConfigVersion: version, TerminalType: row.TerminalType, SIPUsername: row.SIPUsername, AgentID: row.ID, UserID: row.UserID, Extension: row.Extension, VideoCapable: row.VideoCapable, DisplayName: row.DisplayName, State: state}, nil
}

// SetCallState 是呼叫控制侧的 SetState：单独携带 callID，避免状态迁移依赖编码在 reason 里的文本。
func (s *Service) SetCallState(ctx context.Context, callID, agentID, fromState, toState, reason string) error {
	appID, err := applicationID(ctx)
	if err != nil {
		return err
	}
	return s.setState(ctx, appID, callID, agentID, fromState, toState, reason)
}

func (s *Service) setState(ctx context.Context, appID, callID, agentID, fromState, toState, reason string) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sess models.AgentSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("application_id = ? AND agent_id = ?", appID, agentID).First(&sess).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.Conflict("坐席未签入", errs.CodeAgentNotIdle)
			}
			return err
		}
		if reason == "force-check-out" && toState == "offline" && sess.CurrentCallID != "" {
			return tx.Model(&sess).Update("pending_checkout", true).Error
		}
		if callID != "" && sess.CurrentCallID != "" && sess.CurrentCallID != callID {
			return errs.Conflict("过期通话不能更新坐席", errs.CodeAgentBusy)
		}
		if fromState != "" && sess.State != fromState {
			return errs.Conflict("坐席状态已变更", errs.CodeAgentNotIdle)
		}
		if callID == "" {
			callID = sess.CurrentCallID
		}
		if (toState == "ringing" || toState == "on_call") && sess.CurrentCallID != "" && callID != "" && sess.CurrentCallID != callID {
			return errs.Conflict("坐席正在处理另一通话", errs.CodeAgentBusy)
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
			if err := tx.Where("application_id = ? AND agent_id = ?", appID, agentID).Delete(&models.AgentSessionQueue{}).Error; err != nil {
				return err
			}
			if err := tx.Where("application_id = ? AND agent_id = ?", appID, agentID).Delete(&models.AgentSession{}).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&sess).Updates(map[string]any{"state": toState, "current_call_id": nullableUUID(current), "busy_reason": reason, "pending_checkout": false, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.AgentStateLog{ID: uuid.NewString(), ApplicationID: appID, AgentID: agentID, FromState: sess.State, ToState: toState, Reason: reason, CallID: callID, CreatedAt: now}).Error; err != nil {
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
			if err := tx.Exec("UPDATE os_acd_attempts SET state=?,ended_at=?,failure_reason=? WHERE application_id=? AND call_id=? AND agent_id=? AND state IN ('offering','connected')", state, ended, reason, appID, callID, agentID).Error; err != nil {
				return err
			}
		}
		return publishAgentTx(ctx, tx, callID, agentID, toState, reason)
	})
	return err
}

func (s *Service) CheckIn(ctx context.Context, agentID string, queueIDs []string) (ports.AgentSessionView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.AgentSessionView{}, err
	}
	version, err := s.versionFor(ctx, appID)
	if err != nil {
		return ports.AgentSessionView{}, err
	}
	if _, err := s.ByID(ctx, agentID); err != nil {
		return ports.AgentSessionView{}, err
	}
	if len(queueIDs) == 0 {
		if err := s.db.WithContext(ctx).Model(&models.QueueAgent{}).Where("application_id = ? AND config_version = ? AND agent_id = ?", appID, version, agentID).Pluck("queue_id", &queueIDs).Error; err != nil {
			return ports.AgentSessionView{}, err
		}
	}
	if len(queueIDs) == 0 {
		return ports.AgentSessionView{}, errs.InvalidRequest("没有可签入的队列")
	}
	now := time.Now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "agent:"+appID+":"+agentID).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, queueID := range queueIDs {
			if seen[queueID] {
				return errs.InvalidRequest("队列重复")
			}
			seen[queueID] = true
			var count int64
			if err := tx.Model(&models.QueueAgent{}).Where("application_id = ? AND config_version = ? AND queue_id = ? AND agent_id = ?", appID, version, queueID, agentID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return errs.Forbidden("坐席未绑定该队列")
			}
		}
		var sess models.AgentSession
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("application_id = ? AND agent_id = ?", appID, agentID).First(&sess).Error
		if err == nil && (sess.State == "ringing" || sess.State == "on_call") {
			return errs.Conflict("振铃或通话中不能重新签入", errs.CodeAgentBusy)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Where("application_id = ? AND agent_id = ?", appID, agentID).Delete(&models.AgentSessionQueue{}).Error; err != nil {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			sess = models.AgentSession{ApplicationID: appID, ID: uuid.NewString(), AgentID: agentID, State: "idle", CheckedInAt: now, UpdatedAt: now}
			if err := tx.Create(&sess).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&sess).Updates(map[string]any{"state": "idle", "busy_reason": "", "current_call_id": nil, "pending_checkout": false, "checked_in_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		for _, queueID := range queueIDs {
			if err := tx.Create(&models.AgentSessionQueue{ApplicationID: appID, AgentID: agentID, QueueID: queueID}).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&models.AgentStateLog{ID: uuid.NewString(), ApplicationID: appID, AgentID: agentID, FromState: "offline", ToState: "idle", Reason: "check-in", CreatedAt: now}).Error; err != nil {
			return err
		}
		return publishAgentTx(ctx, tx, "", agentID, "idle", "check-in")
	})
	if err != nil {
		return ports.AgentSessionView{}, err
	}
	return s.AgentSession(ctx, agentID)
}

func (s *Service) CheckOut(ctx context.Context, agentID string) error {
	appID, err := applicationID(ctx)
	if err != nil {
		return err
	}
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("application_id = ? AND agent_id = ?", appID, agentID).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if sess.State == "ringing" || sess.State == "on_call" {
		return errs.Conflict("振铃或通话中不能签出", errs.CodeAgentBusy)
	}
	return s.setState(ctx, appID, "", agentID, sess.State, "offline", "check-out")
}

func (s *Service) SetPresence(ctx context.Context, agentID, state, reason string) (ports.AgentSessionView, error) {
	if state != "idle" && state != "busy" && state != "acw" {
		return ports.AgentSessionView{}, errs.InvalidRequest("只允许设置 idle、busy 或 acw")
	}
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.AgentSessionView{}, err
	}
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("application_id = ? AND agent_id = ?", appID, agentID).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.AgentSessionView{}, errs.Conflict("请先签入", errs.CodeAgentNotIdle)
		}
		return ports.AgentSessionView{}, err
	}
	if sess.State == "ringing" || sess.State == "on_call" {
		return ports.AgentSessionView{}, errs.Conflict("振铃或通话中不能切换状态", errs.CodeAgentBusy)
	}
	if err := s.setState(ctx, appID, "", agentID, sess.State, state, reason); err != nil {
		return ports.AgentSessionView{}, err
	}
	return s.AgentSession(ctx, agentID)
}

func (s *Service) AgentSession(ctx context.Context, agentID string) (ports.AgentSessionView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.AgentSessionView{}, err
	}
	out := ports.AgentSessionView{AgentID: agentID, State: "offline", QueueIDs: []string{}}
	var sess models.AgentSession
	if err := s.db.WithContext(ctx).Where("application_id = ? AND agent_id = ?", appID, agentID).First(&sess).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, nil
		}
		return out, err
	}
	out.State, out.BusyReason, out.CurrentCallID = sess.State, sess.BusyReason, sess.CurrentCallID
	if err := s.db.WithContext(ctx).Model(&models.AgentSessionQueue{}).Where("application_id = ? AND agent_id = ?", appID, agentID).Pluck("queue_id", &out.QueueIDs).Error; err != nil {
		return ports.AgentSessionView{}, err
	}
	return out, nil
}

func (s *Service) QueueStatus(ctx context.Context, queueID string) (ports.QueueStatusView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.QueueStatusView{}, err
	}
	if _, err := s.GetQueue(ctx, queueID); err != nil {
		return ports.QueueStatusView{}, err
	}
	out := ports.QueueStatusView{QueueID: queueID}
	if err := s.db.WithContext(ctx).Model(&struct{ CallID string }{}).Table("os_queue_entries").Where("application_id = ? AND queue_id = ? AND state = 'waiting'", appID, queueID).Count(&out.Waiting).Error; err != nil {
		return out, err
	}
	base := s.db.WithContext(ctx).Table("os_agent_sessions s").Joins("JOIN os_agent_session_queues sq ON sq.application_id=s.application_id AND sq.agent_id=s.agent_id").Where("s.application_id = ? AND sq.queue_id = ?", appID, queueID)
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

func (s *Service) ForQueue(ctx context.Context, queueID string) (dto.RecordingPolicy, error) {
	out := dto.RecordingPolicy{Mode: s.options.RecordingMode, NotifyMessage: s.options.NotifyMessage, RetainDays: s.options.RetainDays}
	if queueID == "" {
		return out, nil
	}
	q, err := s.GetQueue(ctx, queueID)
	if err != nil {
		return dto.RecordingPolicy{}, err
	}
	appID, _ := applicationID(ctx)
	version, _ := s.versionFor(ctx, appID)
	var row models.Queue
	if err := s.db.WithContext(ctx).Where("application_id = ? AND config_version = ? AND id = ?", appID, version, q.ID).First(&row).Error; err != nil {
		return dto.RecordingPolicy{}, err
	}
	out.Mode, out.NotifyGuest = row.RecordingPolicy, row.AnnounceRecording
	return out, nil
}

func publishAgentTx(ctx context.Context, tx *gorm.DB, callID, agentID, state, reason string) error {
	return (store.CallEvents{DB: tx}).PublishCallEvent(ctx, ports.CallEvent{CallID: callID, AgentID: agentID, TargetOnly: true, Type: "agent.routing_state_changed", Payload: map[string]any{"agent_id": agentID, "state": state, "busy_reason": reason, "reason": reason, "call_id": callID}})
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
 LEFT JOIN os_calls c ON c.id=s.current_call_id AND c.application_id=s.application_id
 WHERE s.current_call_id IS NOT NULL AND (c.id IS NULL OR c.state='ended')`).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		next := "idle"
		if row.State == "on_call" {
			next = "acw"
		}
		if err := s.setState(scope.WithApplication(ctx, row.ApplicationID), row.ApplicationID, row.CurrentCallID, row.AgentID, row.State, next, "process_recovery"); err != nil {
			return err
		}
	}
	return nil
}
