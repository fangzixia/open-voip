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

// IVRSessions 写入 os_ivr_sessions。
type IVRSessions struct{ DB *gorm.DB }

func (s IVRSessions) UpsertIVRSession(ctx context.Context, applicationID, callID, flowID string, flowVersion int, nodeID, stateJSON string, deadline *time.Time) error {
	if applicationID == "" {
		applicationID = scope.Application(ctx)
	}
	if applicationID == "" || callID == "" || flowID == "" || nodeID == "" {
		return nil
	}
	if stateJSON == "" {
		stateJSON = "{}"
	}
	now := time.Now().UTC()
	return s.DB.WithContext(ctx).Exec(`
INSERT INTO os_ivr_sessions (call_id, application_id, flow_id, flow_version, node_id, state, deadline_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (call_id) DO UPDATE SET
  flow_id = EXCLUDED.flow_id,
  flow_version = EXCLUDED.flow_version,
  node_id = EXCLUDED.node_id,
  state = EXCLUDED.state,
  deadline_at = EXCLUDED.deadline_at,
  updated_at = EXCLUDED.updated_at`,
		callID, applicationID, flowID, flowVersion, nodeID, stateJSON, deadline, now,
	).Error
}

func (s IVRSessions) GetIVRSession(ctx context.Context, callID string) (ports.IVRSessionView, error) {
	var row struct {
		CallID        string
		ApplicationID string
		FlowID        string
		FlowVersion   int
		NodeID        string
		State         string
		DeadlineAt    *time.Time
	}
	q := s.DB.WithContext(ctx).Table("os_ivr_sessions").Where("call_id = ?", callID)
	if app := scope.Application(ctx); app != "" {
		q = q.Where("application_id = ?", app)
	}
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRSessionView{}, errs.NotFound("IVR 会话不存在")
		}
		return ports.IVRSessionView{}, err
	}
	return ports.IVRSessionView{
		CallID: row.CallID, ApplicationID: row.ApplicationID, FlowID: row.FlowID,
		FlowVersion: row.FlowVersion, NodeID: row.NodeID, StateJSON: row.State, DeadlineAt: row.DeadlineAt,
	}, nil
}

func (s IVRSessions) DeleteIVRSession(ctx context.Context, callID string) error {
	if callID == "" {
		return nil
	}
	q := s.DB.WithContext(ctx).Table("os_ivr_sessions").Where("call_id = ?", callID)
	if app := scope.Application(ctx); app != "" {
		q = q.Where("application_id = ?", app)
	}
	return q.Delete(&struct{}{}).Error
}
