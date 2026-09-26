package store

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"open-switch/internal/ports"
	"time"
)

type CallEventRow struct {
	ID         int64           `json:"id"`
	CallID     string          `json:"call_id"`
	Seq        int64           `json:"seq"`
	AgentID    string          `json:"agent_id"`
	TargetOnly bool            `json:"target_only"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}

func (CallEventRow) TableName() string { return "os_call_events" }

// CallEvents persists controller events for cursor-based replay.
type CallEvents struct{ DB *gorm.DB }

func (s CallEvents) Append(ctx context.Context, ev *ports.CallEvent) error {
	raw, err := json.Marshal(ev.Payload)
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
		if err := tx.Raw("SELECT COALESCE(MAX(seq), 0) + 1 FROM os_call_events WHERE call_id = ?", ev.CallID).Scan(&seq).Error; err != nil {
			return err
		}
		createdAt := time.Now().UTC()
		if err := tx.Raw("INSERT INTO os_call_events (call_id, seq, agent_id, target_only, type, payload, created_at) VALUES (?, ?, ?, ?, ?, ?::jsonb, ?) RETURNING id", ev.CallID, seq, ev.AgentID, ev.TargetOnly, ev.Type, string(raw), createdAt).Scan(&ev.ID).Error; err != nil {
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
	if callID != "" {
		q = q.Where("call_id = ?", callID)
	}
	var rows []CallEventRow
	err := q.Order("id").Limit(limit).Find(&rows).Error
	return rows, err
}
