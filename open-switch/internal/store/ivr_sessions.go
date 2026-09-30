package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
)

// IVRSessions 写入 os_ivr_sessions。
type IVRSessions struct{ DB *gorm.DB }

func (s IVRSessions) UpsertIVRSession(ctx context.Context, callID, flowID string, flowVersion int, nodeID, stateJSON string, deadline *time.Time) error {
	if callID == "" || flowID == "" || nodeID == "" {
		return nil
	}
	if stateJSON == "" {
		stateJSON = "{}"
	}
	now := time.Now().UTC()
	return s.DB.WithContext(ctx).Exec(`
INSERT INTO os_ivr_sessions (call_id, flow_id, flow_version, node_id, state, deadline_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (call_id) DO UPDATE SET
  flow_id = EXCLUDED.flow_id,
  flow_version = EXCLUDED.flow_version,
  node_id = EXCLUDED.node_id,
  state = EXCLUDED.state,
  deadline_at = EXCLUDED.deadline_at,
  updated_at = EXCLUDED.updated_at`,
		callID, flowID, flowVersion, nodeID, stateJSON, deadline, now,
	).Error
}

func (s IVRSessions) GetIVRSession(ctx context.Context, callID string) (ports.IVRSessionView, error) {
	var row struct {
		CallID      string
		FlowID      string
		FlowVersion int
		NodeID      string
		State       string
		DeadlineAt  *time.Time
	}
	if err := s.DB.WithContext(ctx).Table("os_ivr_sessions").Where("call_id = ?", callID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRSessionView{}, errs.NotFound("IVR 会话不存在")
		}
		return ports.IVRSessionView{}, err
	}
	return ports.IVRSessionView{
		CallID: row.CallID, FlowID: row.FlowID,
		FlowVersion: row.FlowVersion, NodeID: row.NodeID, StateJSON: row.State, DeadlineAt: row.DeadlineAt,
	}, nil
}

func (s IVRSessions) DeleteIVRSession(ctx context.Context, callID string) error {
	if callID == "" {
		return nil
	}
	return s.DB.WithContext(ctx).Table("os_ivr_sessions").Where("call_id = ?", callID).Delete(&struct{}{}).Error
}
