package recmeta

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-call/internal/errs"
	"open-call/internal/ports"
	"open-call/internal/store/models"
)

// Item 录音列表项。
type Item struct {
	ID        string     `json:"id"`
	CallID    string     `json:"call_id"`
	MediaType string     `json:"media_type"`
	Format    string     `json:"format"`
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
	ID              string     `json:"id"`
	CallID          string     `json:"call_id"`
	AgentID         string     `json:"agent_id"`
	Notes           string     `json:"notes"`
	DispositionCode string     `json:"disposition_code"`
	Tags            []string   `json:"tags"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// QAMarkDTO 质检标记。
type QAMarkDTO struct {
	ID        string    `json:"id"`
	CallID    string    `json:"call_id"`
	OffsetSec int       `json:"offset_sec"`
	Label     string    `json:"label"`
	Score     *int      `json:"score,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Service 录音元数据、小结与质检。
type Service struct {
	db    *gorm.DB
	files interface {
		OpenRecordingAs(context.Context, string, string, string, string) (io.ReadCloser, error)
		DeleteRecording(context.Context, string, string, string) error
	}
}

// NewService 创建录音元数据服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }
func (s *Service) SetFiles(files interface {
	OpenRecordingAs(context.Context, string, string, string, string) (io.ReadCloser, error)
	DeleteRecording(context.Context, string, string, string) error
}) {
	s.files = files
}
func (s *Service) Open(ctx context.Context, row models.Recording, format string) (io.ReadCloser, error) {
	if s.files == nil {
		return nil, errs.NotImplemented("录音文件服务未配置")
	}
	return s.files.OpenRecordingAs(ctx, row.CallID, row.ID, row.FilePath, format)
}

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
		items = append(items, Item{ID: r.ID, CallID: r.CallID, MediaType: r.MediaType,
			Format:    strings.TrimPrefix(strings.ToLower(filepath.Ext(r.FilePath)), "."),
			StartedAt: r.StartedAt, EndedAt: r.EndedAt, FileSize: r.FileSize})
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
	if err := s.db.WithContext(ctx).Where("retain_until IS NOT NULL AND retain_until < ? AND (purge_retry_at IS NULL OR purge_retry_at <= ?)", now, now).Limit(200).Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if s.files == nil {
			return n, errs.NotImplemented("录音文件服务未配置")
		}
		if err := s.files.DeleteRecording(ctx, r.CallID, r.ID, r.FilePath); err != nil {
			retryAt := now.Add(time.Hour)
			_ = s.db.WithContext(ctx).Model(&models.Recording{}).Where("id = ?", r.ID).Updates(map[string]any{"purge_error": err.Error(), "purge_retry_at": retryAt}).Error
			continue
		}
		if err := s.db.WithContext(ctx).Delete(&models.Recording{}, "id = ?", r.ID).Error; err == nil {
			n++
		}
	}
	return n, nil
}

// SaveWrapUp 提交小结。
func (s *Service) SaveWrapUp(ctx context.Context, callID, agentID, notes, disposition string, tags []string, complete bool) (WrapUpDTO, error) {
	notes, disposition = strings.TrimSpace(notes), strings.TrimSpace(disposition)
	if notes == "" && disposition == "" {
		return WrapUpDTO{}, errs.InvalidRequest("小结或处置码至少填写一项")
	}
	if len(disposition) > 64 || len(tags) > 20 {
		return WrapUpDTO{}, errs.InvalidRequest("处置码或标签数量超出限制")
	}
	cleanTags := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || len(tag) > 64 {
			continue
		}
		if !seen[tag] {
			seen[tag] = true
			cleanTags = append(cleanTags, tag)
		}
	}
	tagsJSON, _ := json.Marshal(cleanTags)
	var completedAt *time.Time
	if complete {
		now := time.Now().UTC()
		completedAt = &now
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.CDR{}).Where("call_id = ? AND agent_id = ? AND ended_at IS NOT NULL", callID, agentID).Count(&count).Error; err != nil {
		return WrapUpDTO{}, err
	}
	if count == 0 {
		return WrapUpDTO{}, errs.Forbidden("仅通话坐席可填写已结束通话的小结，话单可能仍在同步")
	}
	row := models.CallWrapUp{ID: uuid.New().String(), CallID: callID, AgentID: agentID, Notes: notes, DispositionCode: disposition, TagsJSON: string(tagsJSON), CompletedAt: completedAt, CreatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "call_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"agent_id", "notes", "disposition_code", "tags_json", "completed_at", "created_at"}),
	}).Create(&row).Error; err != nil {
		return WrapUpDTO{}, err
	}
	return WrapUpDTO{ID: row.ID, CallID: callID, AgentID: agentID, Notes: notes, DispositionCode: disposition, Tags: cleanTags, CompletedAt: completedAt, CreatedAt: row.CreatedAt}, nil
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
		var tags []string
		_ = json.Unmarshal([]byte(r.TagsJSON), &tags)
		if tags == nil {
			tags = []string{}
		}
		out = append(out, WrapUpDTO{ID: r.ID, CallID: r.CallID, AgentID: r.AgentID, Notes: r.Notes, DispositionCode: r.DispositionCode, Tags: tags, CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// AddQAMark 质检标记。
func (s *Service) AddQAMark(ctx context.Context, callID, userID string, offset int, label string, score *int) (QAMarkDTO, error) {
	if label == "" {
		return QAMarkDTO{}, errs.InvalidRequest("标记说明必填")
	}
	if err := s.validateQAMark(ctx, callID, offset, score); err != nil {
		return QAMarkDTO{}, err
	}
	row := models.QAMark{ID: uuid.New().String(), CallID: callID, OffsetSec: offset, Label: label, Score: score, CreatedBy: userID, CreatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return QAMarkDTO{}, err
	}
	return QAMarkDTO{ID: row.ID, CallID: callID, OffsetSec: offset, Label: label, Score: score, CreatedBy: userID, CreatedAt: row.CreatedAt}, nil
}

// ListQAMarks 列出标记。
func (s *Service) ListQAMarks(ctx context.Context, callID string) ([]QAMarkDTO, error) {
	var rows []models.QAMark
	if err := s.db.WithContext(ctx).Where("call_id = ?", callID).Order("offset_sec").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]QAMarkDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, QAMarkDTO{ID: r.ID, CallID: r.CallID, OffsetSec: r.OffsetSec, Label: r.Label, Score: r.Score, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Service) validateQAMark(ctx context.Context, callID string, offset int, score *int) error {
	if offset < 0 || (score != nil && (*score < 0 || *score > 100)) {
		return errs.InvalidRequest("时间偏移或评分超出范围")
	}
	var row models.CDR
	if err := s.db.WithContext(ctx).First(&row, "call_id = ?", callID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.NotFound("话单不存在")
		}
		return err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Recording{}).Where("call_id = ?", callID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return errs.InvalidRequest("该通话没有录音")
	}
	if row.DurationSec > 0 && offset > row.DurationSec {
		return errs.InvalidRequest("标记时间超出通话时长")
	}
	return nil
}

func (s *Service) UpdateQAMark(ctx context.Context, id, callID string, offset int, label string, score *int) (QAMarkDTO, error) {
	if strings.TrimSpace(label) == "" {
		return QAMarkDTO{}, errs.InvalidRequest("标记说明必填")
	}
	if err := s.validateQAMark(ctx, callID, offset, score); err != nil {
		return QAMarkDTO{}, err
	}
	var row models.QAMark
	if err := s.db.WithContext(ctx).First(&row, "id = ? AND call_id = ?", id, callID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return QAMarkDTO{}, errs.NotFound("质检标记不存在")
		}
		return QAMarkDTO{}, err
	}
	row.OffsetSec, row.Label, row.Score = offset, strings.TrimSpace(label), score
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return QAMarkDTO{}, err
	}
	return QAMarkDTO{ID: row.ID, CallID: row.CallID, OffsetSec: row.OffsetSec, Label: row.Label, Score: row.Score, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt}, nil
}

func (s *Service) DeleteQAMark(ctx context.Context, id, callID string) error {
	res := s.db.WithContext(ctx).Delete(&models.QAMark{}, "id = ? AND call_id = ?", id, callID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("质检标记不存在")
	}
	return nil
}
