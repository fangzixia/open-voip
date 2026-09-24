package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
	"open-call/internal/store/models"
)

// CallStore 实现 CallPersistencePort。
type CallStore struct {
	db *gorm.DB
}

// NewCallStore 创建通话持久化适配器。
func NewCallStore(db *gorm.DB) *CallStore {
	return &CallStore{db: db}
}

var _ ports.CallPersistencePort = (*CallStore)(nil)

func (s *CallStore) InsertCall(ctx context.Context, rec ports.CallRecord) error {
	row := models.Call{
		ID:           rec.ID,
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
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *CallStore) UpdateCall(ctx context.Context, rec ports.CallRecord) error {
	updates := map[string]any{
		"state":        rec.State,
		"session_type": string(rec.SessionType),
		"updated_at":   rec.UpdatedAt,
		"ended_at":     rec.EndedAt,
		"queue_id":     rec.QueueID,
		"priority":     rec.Priority,
	}
	res := s.db.WithContext(ctx).Model(&models.Call{}).Where("id = ?", rec.ID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("通话不存在")
	}
	return nil
}

func (s *CallStore) GetCall(ctx context.Context, callID string) (ports.CallRecord, error) {
	var row models.Call
	if err := s.db.WithContext(ctx).First(&row, "id = ?", callID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.CallRecord{}, errs.NotFound("通话不存在")
		}
		return ports.CallRecord{}, err
	}
	return ports.CallRecord{
		ID:           row.ID,
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
	row := models.CallLeg{
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
	if err := s.db.WithContext(ctx).Where("call_id = ?", callID).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ports.CallLegRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, ports.CallLegRecord{
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
	if err := s.db.WithContext(ctx).First(&row, "call_id = ? AND id = ?", callID, legID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.CallLegRecord{}, errs.NotFound("通话腿不存在")
		}
		return ports.CallLegRecord{}, err
	}
	return ports.CallLegRecord{
		ID:        row.ID,
		CallID:    row.CallID,
		Role:      dto.LegRole(row.Role),
		AgentID:   row.AgentID,
		CreatedAt: row.CreatedAt,
	}, nil
}
