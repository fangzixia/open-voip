package configpub

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/ports"
	"open-call/internal/store/models"
)

// DIDDTO DID 路由。
type DIDDTO struct {
	ID          string `json:"id"`
	DID         string `json:"did"`
	QueueID     string `json:"queue_id"`
	DisplayName string `json:"display_name,omitempty"`
}

// SnapshotService 实现 ConfigSnapshotPort。
type SnapshotService struct {
	db *gorm.DB
}

// NewSnapshotService 创建配置快照服务。
func NewSnapshotService(db *gorm.DB) *SnapshotService {
	return &SnapshotService{db: db}
}

var _ ports.ConfigSnapshotPort = (*SnapshotService)(nil)

func (s *SnapshotService) GetQueue(ctx context.Context, queueID string) (ports.QueueSnapshot, error) {
	var row models.Queue
	if err := s.db.WithContext(ctx).First(&row, "id = ?", queueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.QueueSnapshot{}, errs.NotFound("队列不存在")
		}
		return ports.QueueSnapshot{}, err
	}
	var skills []string
	_ = s.db.WithContext(ctx).Model(&models.QueueSkill{}).Where("queue_id = ?", queueID).Pluck("skill_id", &skills)
	out := ports.QueueSnapshot{
		ID:                    row.ID,
		Name:                  row.Name,
		VideoEnabled:          row.VideoEnabled,
		MaxWaitSec:            row.MaxWaitSec,
		OverflowAction:        row.OverflowAction,
		WaitPrompt:            row.WaitPrompt,
		AnnounceRecording:     row.AnnounceRecording,
		SkillIDs:              skills,
		AfterHoursAction:      row.AfterHoursAction,
		ForceHangupOnCheckout: row.ForceHangupOnCheckout,
		ListenAnnounce:        row.ListenAnnounce,
		PriorityEnabled:       row.PriorityEnabled,
	}
	if row.IVRFlowID != nil {
		out.IVRFlowID = *row.IVRFlowID
	}
	if row.OverflowQueueID != nil {
		out.OverflowQueueID = *row.OverflowQueueID
	}
	return out, nil
}

// GetLatestIVR 只返回已发布的最高版本快照，避免交换机读取编辑中的草稿。
func (s *SnapshotService) GetLatestIVR(ctx context.Context, flowID string) (ports.IVRSnapshot, error) {
	var row models.IVRPublishedSnapshot
	if err := s.db.WithContext(ctx).Where("flow_id = ?", flowID).Order("version DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.IVRSnapshot{}, errs.NotFound("未发布 IVR 快照")
		}
		return ports.IVRSnapshot{}, err
	}
	return ports.IVRSnapshot{
		SnapshotID:  row.ID,
		FlowID:      row.FlowID,
		Version:     row.Version,
		PayloadJSON: row.PayloadJSON,
	}, nil
}

func (s *SnapshotService) GetBusinessHours(ctx context.Context, queueID string) (ports.BusinessHours, error) {
	var row models.Queue
	if err := s.db.WithContext(ctx).First(&row, "id = ?", queueID).Error; err != nil {
		return ports.BusinessHours{Timezone: "UTC", WeekdayHours: "always"}, nil
	}
	hours := row.BusinessHoursJSON
	if hours == "" {
		hours = "always"
	}
	return ports.BusinessHours{Timezone: "UTC", WeekdayHours: hours}, nil
}

// ResolveDID 依次尝试号码的规范化候选值，找到对应的呼入队列。
func (s *SnapshotService) ResolveDID(ctx context.Context, did string) (string, error) {
	var last error
	for _, cand := range DIDCandidates(did) {
		var row models.DIDRoute
		err := s.db.WithContext(ctx).Where("d_id = ?", cand).First(&row).Error
		if err == nil {
			return row.QueueID, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
		last = errs.NotFound("DID 未配置")
	}
	if last == nil {
		return "", errs.NotFound("DID 未配置")
	}
	return "", last
}

// DIDCandidates 精确号码及去国家码/长途前缀后的候选，供呼入匹配。
func DIDCandidates(did string) []string {
	d := strings.TrimSpace(did)
	d = strings.TrimPrefix(d, "+")
	seen := map[string]struct{}{}
	out := make([]string, 0, 6)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(d)
	if strings.HasPrefix(d, "00") {
		add(d[2:])
		d = d[2:]
	}
	if strings.HasPrefix(d, "86") && len(d) > 10 {
		add(d[2:])
		d = d[2:]
	}
	if strings.HasPrefix(d, "0") && len(d) > 1 {
		add(d[1:])
		if len(d) > 3 && len(d[3:]) >= 4 {
			add(d[3:])
		}
		if len(d) > 4 && len(d[4:]) >= 4 {
			add(d[4:])
		}
	}
	return out
}

func (s *SnapshotService) Now(ctx context.Context) time.Time {
	return time.Now().UTC()
}

// ListDID DID 列表。
func (s *SnapshotService) ListDID(ctx context.Context) ([]DIDDTO, error) {
	var rows []models.DIDRoute
	if err := s.db.WithContext(ctx).Order("d_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DIDDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, DIDDTO{ID: r.ID, DID: r.DID, QueueID: r.QueueID, DisplayName: r.DisplayName})
	}
	return out, nil
}

// UpsertDID 创建或更新 DID。
func (s *SnapshotService) UpsertDID(ctx context.Context, did, queueID, name string) (DIDDTO, error) {
	if did == "" || queueID == "" {
		return DIDDTO{}, errs.InvalidRequest("did 与 queue_id 必填")
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Queue{}).Where("id = ?", queueID).Count(&count).Error; err != nil {
		return DIDDTO{}, err
	}
	if count == 0 {
		return DIDDTO{}, errs.InvalidRequest("queue_id 不存在")
	}
	var queueCount int64
	if err := s.db.WithContext(ctx).Model(&models.Queue{}).Where("id = ?", queueID).Count(&queueCount).Error; err != nil {
		return DIDDTO{}, err
	}
	if queueCount == 0 {
		return DIDDTO{}, errs.InvalidRequest("queue_id 不存在")
	}
	now := time.Now().UTC()
	var row models.DIDRoute
	err := s.db.WithContext(ctx).Where("d_id = ?", did).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = models.DIDRoute{ID: uuid.New().String(), DID: did, QueueID: queueID, DisplayName: name, CreatedAt: now, UpdatedAt: now}
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return DIDDTO{}, err
		}
	} else if err != nil {
		return DIDDTO{}, err
	} else {
		row.QueueID = queueID
		row.DisplayName = name
		row.UpdatedAt = now
		if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
			return DIDDTO{}, err
		}
	}
	return DIDDTO{ID: row.ID, DID: row.DID, QueueID: row.QueueID, DisplayName: row.DisplayName}, nil
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
