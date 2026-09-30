package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-switch/internal/errs"
)

// Bridges 写入 os_bridges。
type Bridges struct{ DB *gorm.DB }

func (s Bridges) ActivatePair(ctx context.Context, callID, legA, legB string) (string, error) {
	if callID == "" || legA == "" || legB == "" {
		return "", nil
	}
	id := uuid.New().String()
	participants, _ := json.Marshal([]map[string]string{
		{"leg_id": legA},
		{"leg_id": legB},
	})
	now := time.Now().UTC()
	err := s.DB.WithContext(ctx).Exec(`
INSERT INTO os_bridges (id, call_id, mode, state, participants, created_at)
VALUES (?, ?, 'pair', 'active', ?, ?)`,
		id, callID, string(participants), now,
	).Error
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s Bridges) ReplacePair(ctx context.Context, callID, bridgeID, legA, legB string) error {
	if bridgeID == "" || callID == "" || legA == "" || legB == "" {
		return errs.InvalidRequest("桥接参数不完整")
	}
	participants, _ := json.Marshal([]map[string]string{
		{"leg_id": legA},
		{"leg_id": legB},
	})
	res := s.DB.WithContext(ctx).Table("os_bridges").
		Where("id = ? AND call_id = ? AND ended_at IS NULL", bridgeID, callID).
		Update("participants", string(participants))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("桥接不存在或已结束")
	}
	return nil
}

func (s Bridges) EndByCall(ctx context.Context, callID string) ([]string, error) {
	if callID == "" {
		return nil, nil
	}
	var ids []string
	if err := s.DB.WithContext(ctx).Table("os_bridges").
		Where("call_id = ? AND ended_at IS NULL", callID).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	if err := s.DB.WithContext(ctx).Table("os_bridges").Where("id IN ?", ids).Update("ended_at", now).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (s Bridges) EndBridge(ctx context.Context, callID, bridgeID string) error {
	if bridgeID == "" || callID == "" {
		return nil
	}
	now := time.Now().UTC()
	res := s.DB.WithContext(ctx).Table("os_bridges").
		Where("id = ? AND call_id = ? AND ended_at IS NULL", bridgeID, callID).
		Update("ended_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("桥接不存在或已结束")
	}
	return nil
}
