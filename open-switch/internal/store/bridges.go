package store

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"uuid"

	"open-switch/internal/errs"
	"open-switch/internal/scope"
)

// Bridges 写入 os_bridges。
type Bridges struct{ DB *gorm.DB }

func (s Bridges) ActivatePair(ctx context.Context, applicationID, callID, legA, legB string) (string, error) {
	if applicationID == "" {
		applicationID = scope.Application(ctx)
	}
	if applicationID == "" || callID == "" || legA == "" || legB == "" {
		return "", nil
	}
	id := uuid.New().String()
	participants, _ := json.Marshal([]map[string]string{
		{"leg_id": legA},
		{"leg_id": legB},
	})
	now := time.Now().UTC()
	err := s.DB.WithContext(ctx).Exec(`
INSERT INTO os_bridges (id, application_id, call_id, mode, state, participants, created_at)
VALUES (?, ?, ?, 'pair', 'active', ?, ?)`,
		id, applicationID, callID, string(participants), now,
	).Error
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s Bridges) ReplacePair(ctx context.Context, applicationID, callID, bridgeID, legA, legB string) error {
	if bridgeID == "" || callID == "" || legA == "" || legB == "" {
		return errs.InvalidRequest("桥接参数不完整")
	}
	if applicationID == "" {
		applicationID = scope.Application(ctx)
	}
	participants, _ := json.Marshal([]map[string]string{
		{"leg_id": legA},
		{"leg_id": legB},
	})
	q := s.DB.WithContext(ctx).Table("os_bridges").Where("id = ? AND call_id = ? AND ended_at IS NULL", bridgeID, callID)
	if applicationID != "" {
		q = q.Where("application_id = ?", applicationID)
	}
	res := q.Update("participants", string(participants))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("桥接不存在或已结束")
	}
	return nil
}

func (s Bridges) EndByCall(ctx context.Context, applicationID, callID string) ([]string, error) {
	if callID == "" {
		return nil, nil
	}
	if applicationID == "" {
		applicationID = scope.Application(ctx)
	}
	q := s.DB.WithContext(ctx).Table("os_bridges").Where("call_id = ? AND ended_at IS NULL", callID)
	if applicationID != "" {
		q = q.Where("application_id = ?", applicationID)
	}
	var ids []string
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	upd := s.DB.WithContext(ctx).Table("os_bridges").Where("id IN ?", ids)
	if applicationID != "" {
		upd = upd.Where("application_id = ?", applicationID)
	}
	if err := upd.Update("ended_at", now).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (s Bridges) EndBridge(ctx context.Context, applicationID, callID, bridgeID string) error {
	if bridgeID == "" || callID == "" {
		return nil
	}
	if applicationID == "" {
		applicationID = scope.Application(ctx)
	}
	now := time.Now().UTC()
	q := s.DB.WithContext(ctx).Table("os_bridges").Where("id = ? AND call_id = ? AND ended_at IS NULL", bridgeID, callID)
	if applicationID != "" {
		q = q.Where("application_id = ?", applicationID)
	}
	res := q.Update("ended_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("桥接不存在或已结束")
	}
	return nil
}
