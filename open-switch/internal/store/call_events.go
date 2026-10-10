package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"open-switch/internal/datetime"
	"open-switch/internal/ports"
)

type CallEventRow struct {
	ID         int64           `json:"id"`
	CallID     string          `json:"call_id"`
	Seq        int64           `json:"seq"`
	Version    int64           `json:"version"`
	CommandID  *string         `json:"command_id,omitempty"`
	AgentID    string          `json:"agent_id"`
	TargetOnly bool            `json:"target_only"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}

func (CallEventRow) TableName() string { return "os_call_events" }

// CallEvents 持久化控制器事件，供游标 API 增量回放。
type CallEvents struct {
	DB *gorm.DB
	// BeforeCommit atomically stages dependent outbox rows with the event.
	BeforeCommit func(context.Context, *gorm.DB, int64) error
	// AfterAppend only wakes consumers. It must not perform database or network
	// work: WithDB can still belong to a surrounding uncommitted transaction.
	AfterAppend func(ctx context.Context, row CallEventRow)
}

// WithDB 在事务内发布时复用 AfterAppend，避免裸构造 CallEvents{DB: tx} 丢掉投递钩子。
func (s CallEvents) WithDB(db *gorm.DB) CallEvents {
	return CallEvents{DB: db, BeforeCommit: s.BeforeCommit, AfterAppend: s.AfterAppend}
}

// PublishCallEvent 实现 ports.CallEventPublisher：事件先落库，再通过游标对外暴露。
func (s CallEvents) PublishCallEvent(ctx context.Context, ev ports.CallEvent) error {
	return s.Append(ctx, &ev)
}

func (s CallEvents) Append(ctx context.Context, ev *ports.CallEvent) error {
	if s.DB == nil {
		return fmt.Errorf("call event store missing")
	}
	raw, err := datetime.Marshal(ev.Payload)
	if err != nil {
		return err
	}
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	createdAt := time.Now().UTC()
	var seq int64
	var callID any
	if ev.CallID != "" {
		callID = ev.CallID
	}
	var commandID any
	if ev.CommandID != "" {
		commandID = ev.CommandID
	}
	// The call row stays locked until insertion commits. Concurrent recording
	// failure/control events cannot reverse sequence order or reuse sequence 0.
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if ev.CallID != "" {
			res := tx.Raw(`UPDATE os_calls SET event_seq = event_seq + 1 WHERE id = ? RETURNING event_seq`, ev.CallID).Scan(&seq)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("call %s missing for event", ev.CallID)
			}
		}
		if err := tx.Raw("INSERT INTO os_call_events (call_id, seq, version, command_id, agent_id, target_only, type, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?) RETURNING id", callID, seq, ev.Version, commandID, agentUUIDPtr(ev.AgentID), ev.TargetOnly, ev.Type, string(raw), createdAt).Scan(&ev.ID).Error; err != nil {
			return err
		}
		if s.BeforeCommit != nil {
			return s.BeforeCommit(ctx, tx, ev.ID)
		}
		return nil
	})
	if err != nil {
		ev.ID = 0
		return err
	}
	ev.Seq = seq
	if s.AfterAppend != nil && ev.ID > 0 {
		row := CallEventRow{
			ID:         ev.ID,
			CallID:     ev.CallID,
			Seq:        ev.Seq,
			Version:    ev.Version,
			AgentID:    ev.AgentID,
			TargetOnly: ev.TargetOnly,
			Type:       ev.Type,
			Payload:    append(json.RawMessage(nil), raw...),
			CreatedAt:  createdAt,
		}
		if ev.CommandID != "" {
			cid := ev.CommandID
			row.CommandID = &cid
		}
		s.AfterAppend(ctx, row)
	}
	return nil
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

// MinRetainedEventID 返回最早事件 ID（无事件时返回 0）。
func (s CallEvents) MinRetainedEventID(ctx context.Context) (int64, error) {
	var minID int64
	err := s.DB.WithContext(ctx).Raw("SELECT COALESCE(MIN(id), 0) FROM os_call_events").Scan(&minID).Error
	return minID, err
}
