package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/scope"
)

type commandRow struct {
	ID             string
	ApplicationID  string
	CallID         *string
	IdempotencyKey string
	RequestHash    string
	Type           string
	Status         string
	Result         string
	ErrorCode      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (commandRow) TableName() string { return "os_commands" }

// Commands 实现 ports.CommandStore。
type Commands struct{ DB *gorm.DB }

func (s Commands) Accept(ctx context.Context, callID, idempotencyKey, requestHash, typ string, result map[string]any) (ports.CommandView, bool, error) {
	appID := scope.Application(ctx)
	if appID == "" {
		return ports.CommandView{}, false, errs.Forbidden("缺少应用作用域")
	}
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	var existing commandRow
	err := s.DB.WithContext(ctx).Where("application_id = ? AND idempotency_key = ?", appID, idempotencyKey).First(&existing).Error
	if err == nil {
		if existing.RequestHash != requestHash {
			return ports.CommandView{}, false, errs.Conflict("Idempotency-Key 已用于不同请求体", "")
		}
		return rowToView(existing), true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ports.CommandView{}, false, err
	}
	raw, _ := json.Marshal(result)
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	row := commandRow{
		ID: id, ApplicationID: appID, CallID: strPtr(callID), IdempotencyKey: idempotencyKey,
		RequestHash: requestHash, Type: typ, Status: "accepted", Result: string(raw),
		CreatedAt: now, UpdatedAt: now,
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		ev := ports.CallEvent{ApplicationID: appID, CallID: callID, CommandID: id, Type: "command.accepted", Payload: map[string]any{"command_id": id, "type": typ, "call_id": callID}}
		return (CallEvents{DB: tx}).PublishCallEvent(ctx, ev)
	})
	if err != nil {
		return ports.CommandView{}, false, err
	}
	return rowToView(row), false, nil
}

func (s Commands) MarkRunning(ctx context.Context, id string) error {
	q := s.DB.WithContext(ctx).Model(&commandRow{}).Where("id = ? AND status = 'accepted'", id)
	if app := scope.Application(ctx); app != "" {
		q = q.Where("application_id = ?", app)
	}
	res := q.Updates(map[string]any{"status": "running", "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("命令不存在或已结束")
	}
	return nil
}

func (s Commands) Complete(ctx context.Context, id, status, errCode string, result map[string]any) error {
	raw, _ := json.Marshal(result)
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	q := s.DB.WithContext(ctx).Model(&commandRow{}).Where("id = ? AND status IN ('accepted','running','unknown')", id)
	if app := scope.Application(ctx); app != "" {
		q = q.Where("application_id = ?", app)
	}
	res := q.Updates(map[string]any{"status": status, "error_code": errCode, "result": string(raw), "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("命令不存在或已结束")
	}
	var row commandRow
	if err := s.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return err
	}
	kind := "command.succeeded"
	if status == "failed" {
		kind = "command.failed"
	} else if status == "unknown" {
		kind = "command.failed"
	}
	callID := ""
	if row.CallID != nil {
		callID = *row.CallID
	}
	payload := map[string]any{"command_id": id, "type": row.Type, "status": status, "error_code": errCode}
	for k, v := range result {
		payload[k] = v
	}
	return (CallEvents{DB: s.DB}).PublishCallEvent(ctx, ports.CallEvent{
		ApplicationID: row.ApplicationID, CallID: callID, CommandID: id, Type: kind, Payload: payload,
	})
}

func (s Commands) Get(ctx context.Context, id string) (ports.CommandView, error) {
	var row commandRow
	q := s.DB.WithContext(ctx).Where("id = ?", id)
	if app := scope.Application(ctx); app != "" {
		q = q.Where("application_id = ?", app)
	}
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.CommandView{}, errs.NotFound("命令不存在")
		}
		return ports.CommandView{}, err
	}
	return rowToView(row), nil
}

func (s Commands) ReconcileStale(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().UTC().Add(-olderThan)
	var rows []commandRow
	if err := s.DB.WithContext(ctx).Where("status IN ('accepted','running') AND updated_at < ?", cutoff).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		ctxApp := scope.WithApplication(ctx, row.ApplicationID)
		callID := ""
		if row.CallID != nil {
			callID = *row.CallID
		}
		_ = s.Complete(ctxApp, row.ID, "unknown", "stale_command", map[string]any{"call_id": callID})
	}
	return nil
}

func rowToView(row commandRow) ports.CommandView {
	out := ports.CommandView{
		ID: row.ID, ApplicationID: row.ApplicationID, Type: row.Type, Status: row.Status,
		ErrorCode: row.ErrorCode, IdempotencyKey: row.IdempotencyKey, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.CallID != nil {
		out.CallID = *row.CallID
	}
	_ = json.Unmarshal([]byte(row.Result), &out.Result)
	if out.Result == nil {
		out.Result = map[string]any{}
	}
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
