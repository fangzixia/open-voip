package cdr

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"github.com/google/uuid"

	"open-voip/internal/ports"
	"open-voip/internal/store/models"
)

// Item 话单列表项。
type Item struct {
	CallID       string     `json:"call_id"`
	Direction    string     `json:"direction"`
	Caller       string     `json:"caller"`
	Callee       string     `json:"callee"`
	QueueID      string     `json:"queue_id,omitempty"`
	AgentID      string     `json:"agent_id,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	AnsweredAt   *time.Time `json:"answered_at,omitempty"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	DurationSec  int        `json:"duration_sec"`
	WaitSec      int        `json:"wait_sec"`
	Result       string     `json:"result"`
	SessionType  string     `json:"session_type"`
	RecordingIDs []string   `json:"recording_ids"`
}

// ListResult 分页话单。
type ListResult struct {
	Items    []Item `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Total    int64  `json:"total"`
}

// RecorderService 实现 CDRRecorderPort 与查询。
type RecorderService struct {
	db *gorm.DB
}

// NewRecorderService 创建话单服务。
func NewRecorderService(db *gorm.DB) *RecorderService {
	return &RecorderService{db: db}
}

var _ ports.CDRRecorderPort = (*RecorderService)(nil)

func (r *RecorderService) Upsert(ctx context.Context, req ports.CDRWriteRequest) error {
	wait := 0
	if req.AnsweredAt != nil && !req.StartedAt.IsZero() {
		wait = int(req.AnsweredAt.Sub(req.StartedAt).Seconds())
		if wait < 0 {
			wait = 0
		}
	} else if req.EndedAt != nil && req.AnsweredAt == nil && !req.StartedAt.IsZero() {
		wait = int(req.EndedAt.Sub(req.StartedAt).Seconds())
		if wait < 0 {
			wait = 0
		}
	}
	dur := 0
	if req.AnsweredAt != nil && req.EndedAt != nil {
		dur = int(req.EndedAt.Sub(*req.AnsweredAt).Seconds())
		if dur < 0 {
			dur = 0
		}
	}
	var queueID *string
	if req.QueueID != "" {
		q := req.QueueID
		queueID = &q
	}
	var agentID *string
	if req.AgentID != "" {
		a := req.AgentID
		agentID = &a
	}
	row := models.CDR{
		ID:          uuid.New().String(),
		CallID:      req.CallID,
		Direction:   req.Direction,
		QueueID:     queueID,
		AgentID:     agentID,
		Caller:      req.Caller,
		Callee:      req.Callee,
		SessionType: string(req.SessionType),
		Result:      req.Result,
		StartedAt:   req.StartedAt,
		AnsweredAt:  req.AnsweredAt,
		EndedAt:     req.EndedAt,
		DurationSec: dur,
		WaitSec:          wait,
		VideoStartedAt:   req.VideoStartedAt,
		VideoUpgradeOk:   req.VideoUpgradeOk,
		ScreenShareCount: req.ScreenShareCount,
		CreatedAt:        time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "call_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"direction", "queue_id", "agent_id", "caller", "callee", "session_type",
			"result", "started_at", "answered_at", "ended_at", "duration_sec", "wait_sec",
			"video_started_at", "video_upgrade_ok", "screen_share_count",
		}),
	}).Create(&row).Error
}

// List 分页查询。
func (r *RecorderService) List(ctx context.Context, page, pageSize int, from, to, queueID string) (ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&models.CDR{})
	if queueID != "" {
		q = q.Where("queue_id = ?", queueID)
	}
	if from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			q = q.Where("started_at >= ?", t)
		}
	}
	if to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			q = q.Where("started_at <= ?", t)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return ListResult{}, err
	}
	var rows []models.CDR
	if err := q.Order("started_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return ListResult{}, err
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		it := Item{
			CallID:       row.CallID,
			Direction:    row.Direction,
			Caller:       row.Caller,
			Callee:       row.Callee,
			StartedAt:    row.StartedAt,
			AnsweredAt:   row.AnsweredAt,
			EndedAt:      row.EndedAt,
			DurationSec:  row.DurationSec,
			WaitSec:      row.WaitSec,
			Result:       row.Result,
			SessionType:  row.SessionType,
			RecordingIDs: []string{},
		}
		if row.QueueID != nil {
			it.QueueID = *row.QueueID
		}
		if row.AgentID != nil {
			it.AgentID = *row.AgentID
		}
		_ = r.db.WithContext(ctx).Model(&models.Recording{}).Where("call_id = ?", row.CallID).Pluck("id", &it.RecordingIDs)
		if it.RecordingIDs == nil {
			it.RecordingIDs = []string{}
		}
		items = append(items, it)
	}
	return ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// ExportCSV 导出时间范围内话单。
func (r *RecorderService) ExportCSV(ctx context.Context, from, to string) ([]Item, error) {
	if from == "" || to == "" {
		return nil, nil
	}
	res, err := r.List(ctx, 1, 10000, from, to, "")
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}
