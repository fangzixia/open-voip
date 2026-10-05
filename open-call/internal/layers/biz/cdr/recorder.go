package cdr

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"uuid"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-call/internal/datetime"
	"open-call/internal/errs"
	"open-call/internal/observability"
	"open-call/internal/ports"
	"open-call/internal/store/models"
)

// RecordingSummary 通话关联的录音元数据（嵌在话单列表项中）。
type RecordingSummary struct {
	ID        string     `json:"id"`
	CallID    string     `json:"call_id"`
	MediaType string     `json:"media_type"`
	Format    string     `json:"format"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	FileSize  int64      `json:"file_size"`
}

// Item 话单列表项。
type Item struct {
	CallID       string             `json:"call_id"`
	Direction    string             `json:"direction"`
	Caller       string             `json:"caller"`
	Callee       string             `json:"callee"`
	QueueID      string             `json:"queue_id,omitempty"`
	AgentID      string             `json:"agent_id,omitempty"`
	StartedAt    time.Time          `json:"started_at"`
	AnsweredAt   *time.Time         `json:"answered_at,omitempty"`
	EndedAt      *time.Time         `json:"ended_at,omitempty"`
	DurationSec  int                `json:"duration_sec"`
	WaitSec      int                `json:"wait_sec"`
	Result       string             `json:"result"`
	SessionType  string             `json:"session_type"`
	RecordingIDs []string           `json:"recording_ids,omitempty"`
	Recordings   []RecordingSummary `json:"recordings,omitempty"`
	CsatScore    *int               `json:"csat_score,omitempty"`
}

// ListResult 分页话单。
type ListResult struct {
	Items    []Item `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Total    int64  `json:"total"`
}

type Filter struct {
	From, To, QueueID, AgentID, Direction, Result string
	// Caller 主叫号码子串匹配。
	Caller string
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

// Upsert 以 callID 为键持续补全话单，保留通话从排队到结束的记录。
func (r *RecorderService) Upsert(ctx context.Context, req ports.CDRWriteRequest, switchEventVersion int64) error {
	ctx = observability.With(ctx, observability.Context{CallID: req.CallID, AgentID: req.AgentID, QueueID: req.QueueID})
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
		queueID = new(req.QueueID)
	}
	var agentID *string
	if req.AgentID != "" {
		agentID = new(req.AgentID)
	}
	row := models.CDR{
		ID:                 uuid.New().String(),
		CallID:             req.CallID,
		Direction:          req.Direction,
		QueueID:            queueID,
		AgentID:            agentID,
		Caller:             req.Caller,
		Callee:             req.Callee,
		SessionType:        string(req.SessionType),
		Result:             req.Result,
		StartedAt:          req.StartedAt,
		AnsweredAt:         req.AnsweredAt,
		EndedAt:            req.EndedAt,
		DurationSec:        dur,
		WaitSec:            wait,
		VideoStartedAt:     req.VideoStartedAt,
		VideoUpgradeOk:     req.VideoUpgradeOk,
		ScreenShareCount:   req.ScreenShareCount,
		SwitchEventVersion: switchEventVersion,
		CreatedAt:          time.Now().UTC(),
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "call_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"direction":            gorm.Expr("EXCLUDED.direction"),
			"queue_id":             gorm.Expr("EXCLUDED.queue_id"),
			"agent_id":             gorm.Expr("EXCLUDED.agent_id"),
			"caller":               gorm.Expr("EXCLUDED.caller"),
			"callee":               gorm.Expr("EXCLUDED.callee"),
			"session_type":         gorm.Expr("EXCLUDED.session_type"),
			"result":               gorm.Expr("EXCLUDED.result"),
			"started_at":           gorm.Expr("EXCLUDED.started_at"),
			"answered_at":          gorm.Expr("EXCLUDED.answered_at"),
			"ended_at":             gorm.Expr("EXCLUDED.ended_at"),
			"duration_sec":         gorm.Expr("EXCLUDED.duration_sec"),
			"wait_sec":             gorm.Expr("EXCLUDED.wait_sec"),
			"video_started_at":     gorm.Expr("EXCLUDED.video_started_at"),
			"video_upgrade_ok":     gorm.Expr("EXCLUDED.video_upgrade_ok"),
			"screen_share_count":   gorm.Expr("EXCLUDED.screen_share_count"),
			"switch_event_version": gorm.Expr("EXCLUDED.switch_event_version"),
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "oc_cdr.switch_event_version < ?", Vars: []any{switchEventVersion}},
		}},
	}).Create(&row).Error
	if err != nil {
		observability.Emit(ctx, "cdr.upsert.failed", map[string]any{"error": err.Error()})
		return err
	}
	observability.Emit(ctx, "cdr.upserted", map[string]any{
		"direction": req.Direction, "result": req.Result, "duration_sec": dur, "wait_sec": wait,
	})
	return nil
}

// List 分页查询。
func (r *RecorderService) List(ctx context.Context, page, pageSize int, from, to, queueID string) (ListResult, error) {
	return r.ListFiltered(ctx, page, pageSize, Filter{From: from, To: to, QueueID: queueID})
}

func (r *RecorderService) query(ctx context.Context, f Filter) (*gorm.DB, error) {
	q := r.db.WithContext(ctx).Model(&models.CDR{})
	if f.QueueID != "" {
		q = q.Where("queue_id = ?", f.QueueID)
	}
	if f.AgentID != "" {
		q = q.Where("agent_id = ?", f.AgentID)
	}
	if f.Direction != "" {
		q = q.Where("direction = ?", f.Direction)
	}
	if f.Result != "" {
		q = q.Where("result = ?", f.Result)
	}
	if f.Caller != "" {
		q = q.Where("caller LIKE ?", "%"+escapeLike(f.Caller)+"%")
	}
	if f.From != "" {
		t, err := datetime.Parse(f.From)
		if err != nil {
			return nil, errs.InvalidRequest("from 必须为 YYYY-MM-DD HH:MM:SS (本地时间)")
		}
		q = q.Where("started_at >= ?", t)
	}
	if f.To != "" {
		t, err := datetime.Parse(f.To)
		if err != nil {
			return nil, errs.InvalidRequest("to 必须为 YYYY-MM-DD HH:MM:SS (本地时间)")
		}
		q = q.Where("started_at <= ?", t)
	}
	return q, nil
}

func (r *RecorderService) ListFiltered(ctx context.Context, page, pageSize int, f Filter) (ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q, err := r.query(ctx, f)
	if err != nil {
		return ListResult{}, err
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return ListResult{}, err
	}
	var rows []models.CDR
	if err := q.Order("started_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return ListResult{}, err
	}
	callIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		callIDs = append(callIDs, row.CallID)
	}
	recordings := map[string][]RecordingSummary{}
	csats := map[string]int{}
	if len(callIDs) > 0 {
		var recs []models.Recording
		if err := r.db.WithContext(ctx).Where("call_id IN ?", callIDs).Order("started_at").Find(&recs).Error; err != nil {
			return ListResult{}, err
		}
		for _, rec := range recs {
			recordings[rec.CallID] = append(recordings[rec.CallID], recordingSummary(rec))
		}
		var scores []models.CallCsat
		if err := r.db.WithContext(ctx).Where("call_id IN ?", callIDs).Find(&scores).Error; err != nil {
			return ListResult{}, err
		}
		for _, c := range scores {
			csats[c.CallID] = c.Score
		}
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
		if recs := recordings[row.CallID]; len(recs) > 0 {
			it.Recordings = recs
			it.RecordingIDs = make([]string, len(recs))
			for i, rec := range recs {
				it.RecordingIDs[i] = rec.ID
			}
		}
		if score, ok := csats[row.CallID]; ok {
			it.CsatScore = &score
		}
		items = append(items, it)
	}
	return ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func recordingSummary(r models.Recording) RecordingSummary {
	return RecordingSummary{
		ID:        r.ID,
		CallID:    r.CallID,
		MediaType: r.MediaType,
		Format:    strings.TrimPrefix(strings.ToLower(filepath.Ext(r.FilePath)), "."),
		StartedAt: r.StartedAt,
		EndedAt:   r.EndedAt,
		FileSize:  r.FileSize,
	}
}

// Stream 逐行读取指定时间范围内的话单，避免导出时一次加载全部记录。
func (r *RecorderService) Stream(ctx context.Context, f Filter, emit func(Item) error) error {
	if f.From == "" || f.To == "" {
		return errs.InvalidRequest("导出必须指定 from 和 to")
	}
	q, err := r.query(ctx, f)
	if err != nil {
		return err
	}
	rows, err := q.Order("started_at, id").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row models.CDR
		if err := r.db.ScanRows(rows, &row); err != nil {
			return err
		}
		it := Item{CallID: row.CallID, Direction: row.Direction, Caller: row.Caller, Callee: row.Callee, StartedAt: row.StartedAt, AnsweredAt: row.AnsweredAt, EndedAt: row.EndedAt, DurationSec: row.DurationSec, WaitSec: row.WaitSec, Result: row.Result, SessionType: row.SessionType, RecordingIDs: []string{}}
		if row.QueueID != nil {
			it.QueueID = *row.QueueID
		}
		if row.AgentID != nil {
			it.AgentID = *row.AgentID
		}
		if err := emit(it); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return ctx.Err()
}
