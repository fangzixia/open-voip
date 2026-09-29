package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/scope"
)

// RoutingSessions 读取 os_routing_sessions 与队列项。
type RoutingSessions struct{ DB *gorm.DB }

func (s RoutingSessions) Get(ctx context.Context, callID string) (ports.RoutingSessionView, error) {
	var row struct {
		CallID        string
		ApplicationID string
		State         string
		QueueID       *string
		ConfigVersion int64
		UpdatedAt     time.Time
	}
	q := s.DB.WithContext(ctx).Table("os_routing_sessions").Where("call_id = ?", callID)
	if app := scope.Application(ctx); app != "" {
		q = q.Where("application_id = ?", app)
	}
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.RoutingSessionView{}, errs.NotFound("路由会话不存在")
		}
		return ports.RoutingSessionView{}, err
	}
	out := ports.RoutingSessionView{
		CallID: row.CallID, ApplicationID: row.ApplicationID, State: row.State,
		ConfigVersion: row.ConfigVersion, UpdatedAt: row.UpdatedAt,
	}
	if row.QueueID != nil {
		out.QueueID = *row.QueueID
	}
	var entry struct {
		State      string
		EnqueuedAt time.Time
	}
	if err := s.DB.WithContext(ctx).Table("os_queue_entries").Where("call_id = ?", callID).First(&entry).Error; err == nil {
		out.QueueEntryState = entry.State
		out.EnqueuedAt = &entry.EnqueuedAt
	}
	return out, nil
}
