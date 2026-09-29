package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
	"open-switch/internal/store/models"
)

// CallStore 实现 CallPersistencePort。
type CallStore struct {
	db     *gorm.DB
	limits map[string]int
}

// SetApplicationLimits configures hard admission limits for API and SIP ingress alike.
func (s *CallStore) SetApplicationLimits(limits map[string]int) { s.limits = limits }

// NewCallStore 创建通话持久化适配器。
func NewCallStore(db *gorm.DB) *CallStore {
	return &CallStore{db: db}
}

var _ ports.CallPersistencePort = (*CallStore)(nil)

func (s *CallStore) InsertCall(ctx context.Context, rec ports.CallRecord) error {
	if rec.ApplicationID == "" {
		rec.ApplicationID = scope.Application(ctx)
	}
	if rec.ApplicationID == "" {
		return errs.InvalidRequest("application_id 必填")
	}
	if rec.Version == 0 {
		rec.Version = 1
	}
	row := models.Call{
		ApplicationID: rec.ApplicationID, BusinessRef: rec.BusinessRef, Metadata: rec.Metadata, Version: rec.Version, ConfigVersion: rec.ConfigVersion,
		ID:     rec.ID,
		Caller: rec.Caller, Callee: rec.Callee, AgentID: rec.AgentID, OfferedAgent: rec.OfferedAgent, AnsweredAt: rec.AnsweredAt,
		Direction:    rec.Direction,
		SessionType:  string(rec.SessionType),
		State:        rec.State,
		QueueID:      rec.QueueID,
		ParentCallID: rec.ParentCallID,
		Priority:     rec.Priority,
		CreatedAt:    rec.CreatedAt,
		UpdatedAt:    rec.UpdatedAt,
		EndedAt:      rec.EndedAt,
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if limit := s.limits[rec.ApplicationID]; limit > 0 {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "admission:"+rec.ApplicationID).Error; err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&models.Call{}).Where("application_id = ? AND ended_at IS NULL", rec.ApplicationID).Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(limit) {
				return errs.Conflict("应用并发呼叫已达上限", "")
			}
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if rec.ConfigVersion != nil {
			if err := tx.Exec("INSERT INTO os_routing_sessions (call_id,application_id,state,queue_id,config_version,created_at,updated_at) VALUES (?,?,?,?,?,?,?)", rec.ID, rec.ApplicationID, rec.State, rec.QueueID, *rec.ConfigVersion, rec.CreatedAt, rec.UpdatedAt).Error; err != nil {
				return err
			}
		}
		return (CallEvents{DB: tx}).PublishCallEvent(ctx, ports.CallEvent{ApplicationID: rec.ApplicationID, CallID: rec.ID, Version: 1, Type: "call.created", Payload: map[string]any{"call_id": rec.ID, "direction": rec.Direction}})
	})
}

// InsertCallWithLeg records the call and its first leg atomically.
func (s *CallStore) InsertCallWithLeg(ctx context.Context, rec ports.CallRecord, leg ports.CallLegRecord) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		store := &CallStore{db: tx, limits: s.limits}
		if err := store.InsertCall(ctx, rec); err != nil {
			return err
		}
		return store.InsertLeg(ctx, leg)
	})
}

func (s *CallStore) DeleteLeg(ctx context.Context, callID, legID string) error {
	q := s.db.WithContext(ctx).Where("call_id = ? AND id = ?", callID, legID)
	if appID := scope.Application(ctx); appID != "" {
		q = q.Where("application_id = ?", appID)
	}
	return q.Delete(&models.CallLeg{}).Error
}

func (s *CallStore) UpdateCall(ctx context.Context, rec ports.CallRecord) error {
	updates := map[string]any{
		"version": gorm.Expr("version + 1"),
		"state":   rec.State,
		"caller":  rec.Caller, "callee": rec.Callee, "agent_id": rec.AgentID, "offered_agent": rec.OfferedAgent, "answered_at": rec.AnsweredAt, "direction": rec.Direction,
		"session_type": string(rec.SessionType),
		"updated_at":   rec.UpdatedAt,
		"ended_at":     rec.EndedAt,
		"queue_id":     rec.QueueID,
		"priority":     rec.Priority,
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.Call{}).Where("id = ?", rec.ID)
		if app := scope.Application(ctx); app != "" {
			q = q.Where("application_id = ?", app)
		}
		res := q.Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.NotFound("通话不存在")
		}
		var current models.Call
		if err := tx.First(&current, "id = ?", rec.ID).Error; err != nil {
			return err
		}
		if rec.State == "ended" {
			if err := tx.Exec("UPDATE os_business_actions SET status='expired',updated_at=NOW() WHERE application_id=? AND call_id=? AND status='pending'", current.ApplicationID, rec.ID).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("UPDATE os_routing_sessions SET state=?,queue_id=?,updated_at=?,ended_at=? WHERE call_id=?", rec.State, rec.QueueID, rec.UpdatedAt, rec.EndedAt, rec.ID).Error; err != nil {
			return err
		}
		if rec.State == "queued" && rec.QueueID != nil {
			if err := tx.Exec(`INSERT INTO os_queue_entries (call_id,application_id,queue_id,priority,enqueued_at,deadline_at,state)
              SELECT ?,?,?,?, ?, ?::timestamptz + make_interval(secs => q.max_wait_sec), 'waiting'
              FROM os_queues q WHERE q.application_id=? AND q.config_version=? AND q.id=?
              ON CONFLICT (call_id) DO UPDATE SET queue_id=EXCLUDED.queue_id, priority=EXCLUDED.priority,
              enqueued_at=EXCLUDED.enqueued_at,deadline_at=EXCLUDED.deadline_at,state='waiting'`,
				rec.ID, current.ApplicationID, *rec.QueueID, rec.Priority, rec.UpdatedAt, rec.UpdatedAt, current.ApplicationID, current.ConfigVersion, *rec.QueueID).Error; err != nil {
				return err
			}
		} else if err := tx.Exec("UPDATE os_queue_entries SET state=? WHERE call_id=?", rec.State, rec.ID).Error; err != nil {
			return err
		}
		return (CallEvents{DB: tx}).PublishCallEvent(ctx, ports.CallEvent{ApplicationID: current.ApplicationID, CallID: rec.ID, Version: current.Version, Type: "call.state_changed", Payload: map[string]any{"call_id": rec.ID, "state": rec.State, "queue_id": rec.QueueID, "version": current.Version}})
	})
}

func (s *CallStore) GetCall(ctx context.Context, callID string) (ports.CallRecord, error) {
	var row models.Call
	q := s.db.WithContext(ctx).Where("id = ?", callID)
	if appID := scope.Application(ctx); appID != "" {
		q = q.Where("application_id = ?", appID)
	}
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.CallRecord{}, errs.NotFound("通话不存在")
		}
		return ports.CallRecord{}, err
	}
	return ports.CallRecord{
		ApplicationID: row.ApplicationID, BusinessRef: row.BusinessRef, Metadata: row.Metadata, Version: row.Version, ConfigVersion: row.ConfigVersion,
		ID:     row.ID,
		Caller: row.Caller, Callee: row.Callee, AgentID: row.AgentID, OfferedAgent: row.OfferedAgent, AnsweredAt: row.AnsweredAt,
		Direction:    row.Direction,
		SessionType:  dto.SessionType(row.SessionType),
		State:        row.State,
		QueueID:      row.QueueID,
		ParentCallID: row.ParentCallID,
		Priority:     row.Priority,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
		EndedAt:      row.EndedAt,
	}, nil
}

func (s *CallStore) InsertLeg(ctx context.Context, rec ports.CallLegRecord) error {
	if rec.ApplicationID == "" {
		rec.ApplicationID = scope.Application(ctx)
	}
	if rec.ApplicationID == "" {
		if err := s.db.WithContext(ctx).Model(&models.Call{}).Where("id = ?", rec.CallID).Pluck("application_id", &rec.ApplicationID).Error; err != nil {
			return err
		}
	}
	if rec.Type == "" {
		rec.Type = "webrtc"
	}
	if rec.State == "" {
		rec.State = "new"
	}
	row := models.CallLeg{
		ApplicationID: rec.ApplicationID, Type: rec.Type, State: rec.State, ParticipantRef: rec.ParticipantRef,
		ID:        rec.ID,
		CallID:    rec.CallID,
		Role:      string(rec.Role),
		AgentID:   rec.AgentID,
		CreatedAt: rec.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *CallStore) ListLegs(ctx context.Context, callID string) ([]ports.CallLegRecord, error) {
	var rows []models.CallLeg
	q := s.db.WithContext(ctx).Where("call_id = ?", callID)
	if appID := scope.Application(ctx); appID != "" {
		q = q.Where("application_id = ?", appID)
	}
	if err := q.Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ports.CallLegRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, ports.CallLegRecord{
			ApplicationID: row.ApplicationID, Type: row.Type, State: row.State, ParticipantRef: row.ParticipantRef,
			ID:        row.ID,
			CallID:    row.CallID,
			Role:      dto.LegRole(row.Role),
			AgentID:   row.AgentID,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func (s *CallStore) GetLeg(ctx context.Context, callID, legID string) (ports.CallLegRecord, error) {
	var row models.CallLeg
	q := s.db.WithContext(ctx).Where("call_id = ? AND id = ?", callID, legID)
	if appID := scope.Application(ctx); appID != "" {
		q = q.Where("application_id = ?", appID)
	}
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.CallLegRecord{}, errs.NotFound("通话腿不存在")
		}
		return ports.CallLegRecord{}, err
	}
	return ports.CallLegRecord{
		ApplicationID: row.ApplicationID, Type: row.Type, State: row.State, ParticipantRef: row.ParticipantRef,
		ID:        row.ID,
		CallID:    row.CallID,
		Role:      dto.LegRole(row.Role),
		AgentID:   row.AgentID,
		CreatedAt: row.CreatedAt,
	}, nil
}

func (s *CallStore) Unfinished(ctx context.Context) ([]ports.CallRecord, error) {
	var ids []string
	if err := s.db.WithContext(ctx).Model(&models.Call{}).Where("state <> ?", "ended").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	out := make([]ports.CallRecord, 0, len(ids))
	for _, id := range ids {
		rec, err := s.GetCall(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}
