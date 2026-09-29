package store

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"open-switch/internal/datetime"
	"open-switch/internal/ports"
	"open-switch/internal/scope"
	"time"
)

type CallEventRow struct {
	ApplicationID string          `json:"application_id"`
	ID            int64           `json:"id"`
	CallID        string          `json:"call_id"`
	Seq           int64           `json:"seq"`
	Version       int64           `json:"version"`
	CommandID     *string         `json:"command_id,omitempty"`
	AgentID       string          `json:"agent_id"`
	TargetOnly    bool            `json:"target_only"`
	Type          string          `json:"type"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     time.Time       `json:"created_at"`
}

func (CallEventRow) TableName() string { return "os_call_events" }

// CallEvents persists controller events for cursor-based replay.
type CallEvents struct{ DB *gorm.DB }

// PublishCallEvent implements ports.CallEventPublisher. Events are durable before
// they are exposed through the cursor API.
func (s CallEvents) PublishCallEvent(ctx context.Context, ev ports.CallEvent) error {
	return s.Append(ctx, &ev)
}

func (s CallEvents) Append(ctx context.Context, ev *ports.CallEvent) error {
	if ev.ApplicationID == "" {
		ev.ApplicationID = scope.Application(ctx)
	}
	if ev.ApplicationID == "" && ev.CallID != "" {
		if err := s.DB.WithContext(ctx).Raw("SELECT application_id FROM os_calls WHERE id = ?", ev.CallID).Scan(&ev.ApplicationID).Error; err != nil {
			return err
		}
	}
	raw, err := datetime.Marshal(ev.Payload)
	if err != nil {
		return err
	}
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A global transaction lock keeps committed rows in ID order. Without it,
		// a reader could advance past an uncommitted lower ID from another call.
		var locked int64
		if err := tx.Raw("SELECT 1 FROM (SELECT pg_advisory_xact_lock(67104231)) AS lock_held").Scan(&locked).Error; err != nil {
			return err
		}
		var seq int64
		if err := tx.Raw("SELECT COALESCE(MAX(seq), 0) + 1 FROM os_call_events WHERE application_id = ? AND call_id::text = ?", ev.ApplicationID, ev.CallID).Scan(&seq).Error; err != nil {
			return err
		}
		createdAt := time.Now().UTC()
		var callID any
		if ev.CallID != "" {
			callID = ev.CallID
		}
		var commandID any
		if ev.CommandID != "" {
			commandID = ev.CommandID
		}
		if err := tx.Raw("INSERT INTO os_call_events (application_id, call_id, seq, version, command_id, agent_id, target_only, type, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?) RETURNING id", ev.ApplicationID, callID, seq, ev.Version, commandID, ev.AgentID, ev.TargetOnly, ev.Type, string(raw), createdAt).Scan(&ev.ID).Error; err != nil {
			return err
		}
		ev.Seq = seq
		return nil
	})
}

func (s CallEvents) List(ctx context.Context, afterID int64, callID string, limit int) ([]CallEventRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := s.DB.WithContext(ctx).Where("id > ?", afterID)
	if appID := scope.Application(ctx); appID != "" {
		q = q.Where("application_id = ?", appID)
	}
	if callID != "" {
		q = q.Where("call_id = ?", callID)
	}
	var rows []CallEventRow
	err := q.Order("id").Limit(limit).Find(&rows).Error
	return rows, err
}
