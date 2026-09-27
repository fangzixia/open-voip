package configpub

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/store/models"
)

// DIDDTO DID 路由。
type DIDDTO struct {
	ID         string `json:"id"`
	TrunkID    string `json:"trunk_id"`
	DID        string `json:"did"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id,omitempty"`
}

// SnapshotService 管理业务侧 DID 草稿与队列/IVR 绑定。
type SnapshotService struct {
	db *gorm.DB
}

// NewSnapshotService 创建配置快照服务。
func NewSnapshotService(db *gorm.DB) *SnapshotService {
	return &SnapshotService{db: db}
}

// ListDID DID 列表。
func (s *SnapshotService) ListDID(ctx context.Context) ([]DIDDTO, error) {
	var rows []models.DIDRoute
	if err := s.db.WithContext(ctx).Order("d_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DIDDTO, 0, len(rows))
	for _, r := range rows {
		targetID := ""
		if r.TargetID != nil {
			targetID = *r.TargetID
		}
		out = append(out, DIDDTO{ID: r.ID, TrunkID: r.TrunkID, DID: r.DID, TargetType: r.TargetType, TargetID: targetID})
	}
	return out, nil
}

// UpsertDID 创建或更新 DID。
func (s *SnapshotService) UpsertDID(ctx context.Context, trunkID, did, targetType, targetID string) (DIDDTO, error) {
	if trunkID == "" {
		trunkID = "*"
	}
	if did == "" || (targetType != "queue" && targetType != "ivr" && targetType != "reject") {
		return DIDDTO{}, errs.InvalidRequest("did 或 target_type 无效")
	}
	if targetType == "reject" && targetID != "" {
		return DIDDTO{}, errs.InvalidRequest("reject 路由不能包含 target_id")
	}
	if targetType != "reject" && targetID == "" {
		return DIDDTO{}, errs.InvalidRequest("target_id 必填")
	}
	if targetType == "queue" {
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.Queue{}).Where("id = ?", targetID).Count(&count).Error; err != nil {
			return DIDDTO{}, err
		}
		if count == 0 {
			return DIDDTO{}, errs.InvalidRequest("目标队列不存在")
		}
	}
	if targetType == "ivr" {
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.IVRPublishedSnapshot{}).Where("flow_id = ?", targetID).Count(&count).Error; err != nil {
			return DIDDTO{}, err
		}
		if count == 0 {
			return DIDDTO{}, errs.InvalidRequest("目标 IVR 尚未发布")
		}
	}
	now := time.Now().UTC()
	var row models.DIDRoute
	err := s.db.WithContext(ctx).Where("trunk_id = ? AND d_id = ?", trunkID, did).First(&row).Error
	var target *string
	if targetID != "" {
		target = &targetID
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = models.DIDRoute{ID: uuid.New().String(), TrunkID: trunkID, DID: did, TargetType: targetType, TargetID: target, CreatedAt: now, UpdatedAt: now}
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return DIDDTO{}, err
		}
	} else if err != nil {
		return DIDDTO{}, err
	} else {
		row.TargetType = targetType
		row.TargetID = target
		row.UpdatedAt = now
		if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
			return DIDDTO{}, err
		}
	}
	return DIDDTO{ID: row.ID, TrunkID: row.TrunkID, DID: row.DID, TargetType: row.TargetType, TargetID: targetID}, nil
}

// DeleteDID 删除。
func (s *SnapshotService) DeleteDID(ctx context.Context, id string) error {
	res := s.db.WithContext(ctx).Delete(&models.DIDRoute{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("DID 不存在")
	}
	return nil
}

// BindQueueIVR 队列绑定 IVR。
func (s *SnapshotService) BindQueueIVR(ctx context.Context, queueID, flowID string) error {
	var fid *string
	if flowID != "" {
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.IVRPublishedSnapshot{}).Where("flow_id = ?", flowID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errs.InvalidRequest("只能绑定已经发布的 IVR 流程")
		}
		fid = &flowID
	}
	res := s.db.WithContext(ctx).Model(&models.Queue{}).Where("id = ?", queueID).Updates(map[string]any{"ivr_flow_id": fid, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("队列不存在")
	}
	return nil
}
