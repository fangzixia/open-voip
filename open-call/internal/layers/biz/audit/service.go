package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/store/models"
)

// Item 审计日志。
type Item struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id,omitempty"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource,omitempty"`
	DetailJSON string    `json:"detail_json,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	RemoteIP   string    `json:"remote_ip,omitempty"`
	Outcome    string    `json:"outcome"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListResult 分页。
type ListResult struct {
	Items    []Item `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Total    int64  `json:"total"`
}

// Service 操作审计。
type Service struct {
	db            *gorm.DB
	writeFailures atomic.Uint64
}

// NewService 创建审计服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// Write 写入一条审计；调用方可以决定审计失败是否阻止敏感操作。
func (s *Service) Write(ctx context.Context, userID, action, resource string, detail any) error {
	if s == nil || s.db == nil {
		return nil
	}
	raw := ""
	if detail != nil {
		b, _ := json.Marshal(detail)
		raw = string(b)
	}
	var uid *string
	if userID != "" {
		uid = &userID
	}
	outcome, requestID, remoteIP := "success", "", ""
	if values, ok := detail.(map[string]string); ok {
		if values["outcome"] != "" {
			outcome = values["outcome"]
		}
		requestID, remoteIP = values["request_id"], values["remote_ip"]
	}
	err := s.db.WithContext(ctx).Create(&models.AuditLog{
		ID: uuid.New().String(), UserID: uid, Action: action, Resource: resource,
		DetailJSON: raw, RequestID: requestID, RemoteIP: remoteIP, Outcome: outcome, CreatedAt: time.Now().UTC(),
	}).Error
	if err != nil {
		s.writeFailures.Add(1)
	}
	return err
}

func (s *Service) WriteFailures() uint64 {
	if s == nil {
		return 0
	}
	return s.writeFailures.Load()
}

// List 按人、时间检索。
// extra 依次为 resource、outcome、from、to，保留旧调用兼容性。
func (s *Service) List(ctx context.Context, page, pageSize int, userID, action string, extra ...string) (ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := s.db.WithContext(ctx).Model(&models.AuditLog{})
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if action != "" {
		q = q.Where("action = ?", action)
	}
	value := func(index int) string {
		if len(extra) > index {
			return extra[index]
		}
		return ""
	}
	if resource := value(0); resource != "" {
		q = q.Where("resource = ?", resource)
	}
	if outcome := value(1); outcome != "" {
		q = q.Where("outcome = ?", outcome)
	}
	if from := value(2); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return ListResult{}, fmt.Errorf("from 必须为 RFC3339: %w", err)
		}
		q = q.Where("created_at >= ?", t)
	}
	if to := value(3); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return ListResult{}, fmt.Errorf("to 必须为 RFC3339: %w", err)
		}
		q = q.Where("created_at <= ?", t)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return ListResult{}, err
	}
	var rows []models.AuditLog
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return ListResult{}, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		it := Item{ID: r.ID, Action: r.Action, Resource: r.Resource, DetailJSON: r.DetailJSON,
			RequestID: r.RequestID, RemoteIP: r.RemoteIP, Outcome: r.Outcome, CreatedAt: r.CreatedAt}
		if r.UserID != nil {
			it.UserID = *r.UserID
		}
		items = append(items, it)
	}
	return ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}
