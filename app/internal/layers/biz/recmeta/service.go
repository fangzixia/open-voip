package recmeta

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-voip/internal/errs"
	"open-voip/internal/ports"
	"open-voip/internal/store/models"
)

// Item 录音列表项。
type Item struct {
	ID        string     `json:"id"`
	CallID    string     `json:"call_id"`
	FilePath  string     `json:"file_path"`
	MediaType string     `json:"media_type"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	FileSize  int64      `json:"file_size"`
}

// ListResult 分页。
type ListResult struct {
	Items    []Item `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Total    int64  `json:"total"`
}

// WrapUpDTO 通话小结。
type WrapUpDTO struct {
	ID        string    `json:"id"`
	CallID    string    `json:"call_id"`
	AgentID   string    `json:"agent_id"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

// QAMarkDTO 质检标记。
type QAMarkDTO struct {
	ID        string    `json:"id"`
	CallID    string    `json:"call_id"`
	OffsetSec int       `json:"offset_sec"`
	Label     string    `json:"label"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Service 录音元数据、小结与质检。
type Service struct {
	db *gorm.DB
}

// NewService 创建录音元数据服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

var _ ports.RecordingStorePort = (*Service)(nil)

func (s *Service) Save(ctx context.Context, rec ports.RecordingMeta) error {
	if rec.ID == "" {
		rec.ID = uuid.New().String()
	}
	row := models.Recording{
		ID: rec.ID, CallID: rec.CallID, FilePath: rec.FilePath, MediaType: rec.MediaType,
		StartedAt: rec.StartedAt, EndedAt: rec.EndedAt, RetainUntil: rec.RetainUntil, FileSize: rec.FileSize,
		CreatedAt: time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"file_path", "media_type", "ended_at", "retain_until", "file_size"}),
	}).Create(&row).Error
}

// List 按通话过滤。
func (s *Service) List(ctx context.Context, page, pageSize int, callID string) (ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := s.db.WithContext(ctx).Model(&models.Recording{})
	if callID != "" {
		q = q.Where("call_id = ?", callID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return ListResult{}, err
	}
	var rows []models.Recording
	if err := q.Order("started_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return ListResult{}, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, Item{ID: r.ID, CallID: r.CallID, FilePath: r.FilePath, MediaType: r.MediaType, StartedAt: r.StartedAt, EndedAt: r.EndedAt, FileSize: r.FileSize})
	}
	return ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// Get 读取单条。
func (s *Service) Get(ctx context.Context, id string) (models.Recording, error) {
	var row models.Recording
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Recording{}, errs.NotFound("录音不存在")
		}
		return models.Recording{}, err
	}
	return row, nil
}

// IDsByCall 通话关联录音。
func (s *Service) IDsByCall(ctx context.Context, callID string) []string {
	var ids []string
	_ = s.db.WithContext(ctx).Model(&models.Recording{}).Where("call_id = ?", callID).Pluck("id", &ids)
	return ids
}

// PurgeExpired 删除过期文件与记录。
func (s *Service) PurgeExpired(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	var rows []models.Recording
	if err := s.db.WithContext(ctx).Where("retain_until IS NOT NULL AND retain_until < ?", now).Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		_ = os.Remove(r.FilePath)
		if err := s.db.WithContext(ctx).Delete(&models.Recording{}, "id = ?", r.ID).Error; err == nil {
			n++
		}
	}
	return n, nil
}

// SaveWrapUp 提交小结。
func (s *Service) SaveWrapUp(ctx context.Context, callID, agentID, notes string) (WrapUpDTO, error) {
	if notes == "" {
		return WrapUpDTO{}, errs.InvalidRequest("小结不能为空")
	}
	row := models.CallWrapUp{ID: uuid.New().String(), CallID: callID, AgentID: agentID, Notes: notes, CreatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "call_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"agent_id", "notes", "created_at"}),
	}).Create(&row).Error; err != nil {
		return WrapUpDTO{}, err
	}
	return WrapUpDTO{ID: row.ID, CallID: callID, AgentID: agentID, Notes: notes, CreatedAt: row.CreatedAt}, nil
}

// ListWrapUps 按通话或全部列出小结。
func (s *Service) ListWrapUps(ctx context.Context, callID string) ([]WrapUpDTO, error) {
	q := s.db.WithContext(ctx).Model(&models.CallWrapUp{})
	if callID != "" {
		q = q.Where("call_id = ?", callID)
	}
	var rows []models.CallWrapUp
	if err := q.Order("created_at DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]WrapUpDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, WrapUpDTO{ID: r.ID, CallID: r.CallID, AgentID: r.AgentID, Notes: r.Notes, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// AddQAMark 质检标记。
func (s *Service) AddQAMark(ctx context.Context, callID, userID string, offset int, label string) (QAMarkDTO, error) {
	if label == "" {
		return QAMarkDTO{}, errs.InvalidRequest("标记说明必填")
	}
	row := models.QAMark{ID: uuid.New().String(), CallID: callID, OffsetSec: offset, Label: label, CreatedBy: userID, CreatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return QAMarkDTO{}, err
	}
	return QAMarkDTO{ID: row.ID, CallID: callID, OffsetSec: offset, Label: label, CreatedBy: userID, CreatedAt: row.CreatedAt}, nil
}

// ListQAMarks 列出标记。
func (s *Service) ListQAMarks(ctx context.Context, callID string) ([]QAMarkDTO, error) {
	var rows []models.QAMark
	if err := s.db.WithContext(ctx).Where("call_id = ?", callID).Order("offset_sec").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]QAMarkDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, QAMarkDTO{ID: r.ID, CallID: r.CallID, OffsetSec: r.OffsetSec, Label: r.Label, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt})
	}
	return out, nil
}
