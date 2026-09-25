package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/config"
	"open-call/internal/datetime"
	"open-call/internal/errs"
	"open-call/internal/observability"
	"open-call/internal/ports"
	"open-call/internal/store/models"
)

// SubDTO 是 Webhook 订阅的对外表示，签名密钥不会回传。
type SubDTO struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	EventTypes []string  `json:"event_types"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

// DeliveryDTO 是持久投递任务的对外表示。
type DeliveryDTO struct {
	ID             string     `json:"id"`
	EventID        string     `json:"event_id"`
	SubscriptionID string     `json:"subscription_id"`
	EventType      string     `json:"event_type"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	LastError      string     `json:"last_error,omitempty"`
	NextAttemptAt  time.Time  `json:"next_attempt_at"`
	DeadLetterAt   *time.Time `json:"dead_letter_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// DeliveryList 是分页后的投递任务列表。
type DeliveryList struct {
	Items    []DeliveryDTO `json:"items"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int64         `json:"total"`
}

// UpdateInput 表示订阅的部分更新。
type UpdateInput struct {
	URL          *string   `json:"url"`
	EventTypes   *[]string `json:"event_types"`
	Secret       *string   `json:"secret"`
	RotateSecret bool      `json:"rotate_secret"`
	Enabled      *bool     `json:"enabled"`
}

// Stats 是运维状态使用的 Webhook 积压摘要。
type Stats struct {
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	DeadLetter int64 `json:"dead_letter"`
}

// Service 管理订阅，并使用 PostgreSQL 持久任务完成最终投递。
type Service struct {
	db     *gorm.DB
	cfg    config.WebhookConfig
	client *http.Client
}

// NewService 创建 Webhook 服务。
func NewService(db *gorm.DB, cfg config.WebhookConfig) *Service {
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Service{db: db, cfg: cfg, client: &http.Client{Timeout: timeout}}
}

var _ ports.WebhookDispatcher = (*Service)(nil)

// List 按创建时间列出订阅。
func (s *Service) List(ctx context.Context) ([]SubDTO, error) {
	var rows []models.WebhookSubscription
	if err := s.db.WithContext(ctx).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SubDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSub(row))
	}
	return out, nil
}

// Create 创建订阅。
func (s *Service) Create(ctx context.Context, target string, types []string, secret string) (SubDTO, error) {
	if err := validateTarget(target); err != nil {
		return SubDTO{}, err
	}
	types = normalizeTypes(types)
	if len(types) == 0 {
		return SubDTO{}, errs.InvalidRequest("event_types 不能为空")
	}
	if secret == "" {
		var err error
		secret, err = randomSecret()
		if err != nil {
			return SubDTO{}, err
		}
	}
	raw, _ := json.Marshal(types)
	row := models.WebhookSubscription{ID: uuid.New().String(), URL: strings.TrimSpace(target), EventTypes: string(raw), Secret: secret, Enabled: true, CreatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return SubDTO{}, err
	}
	return toSub(row), nil
}

// Update 部分更新订阅；密钥只接受替换或轮换，不提供读取接口。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (SubDTO, string, error) {
	var row models.WebhookSubscription
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SubDTO{}, "", errs.NotFound("订阅不存在")
		}
		return SubDTO{}, "", err
	}
	updates := map[string]any{}
	if in.URL != nil {
		if err := validateTarget(*in.URL); err != nil {
			return SubDTO{}, "", err
		}
		updates["url"] = strings.TrimSpace(*in.URL)
	}
	if in.EventTypes != nil {
		types := normalizeTypes(*in.EventTypes)
		if len(types) == 0 {
			return SubDTO{}, "", errs.InvalidRequest("event_types 不能为空")
		}
		raw, _ := json.Marshal(types)
		updates["event_types"] = string(raw)
	}
	if in.Enabled != nil {
		updates["enabled"] = *in.Enabled
	}
	newSecret := ""
	if in.Secret != nil {
		newSecret = strings.TrimSpace(*in.Secret)
	}
	if in.RotateSecret {
		var err error
		newSecret, err = randomSecret()
		if err != nil {
			return SubDTO{}, "", err
		}
	}
	if newSecret != "" {
		updates["secret"] = newSecret
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
			return SubDTO{}, "", err
		}
	}
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return SubDTO{}, "", err
	}
	return toSub(row), newSecret, nil
}

// Delete 删除订阅，数据库外键会同时删除其投递记录。
func (s *Service) Delete(ctx context.Context, id string) error {
	res := s.db.WithContext(ctx).Delete(&models.WebhookSubscription{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("订阅不存在")
	}
	return nil
}

// Dispatch 先持久化所有匹配订阅的任务，不在业务请求线程中访问外部系统。
func (s *Service) Dispatch(ctx context.Context, eventType string, payload map[string]any) error {
	var subscriptions []models.WebhookSubscription
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&subscriptions).Error; err != nil {
		return err
	}
	eventID := uuid.New().String()
	ids := observability.From(ctx)
	body, err := datetime.Marshal(map[string]any{
		"id": eventID, "type": eventType, "ts": datetime.Format(time.Now()), "payload": payload,
		"trace_id": ids.TraceID, "request_id": ids.RequestID,
	})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, sub := range subscriptions {
			var types []string
			if json.Unmarshal([]byte(sub.EventTypes), &types) != nil || !matchType(types, eventType) {
				continue
			}
			row := models.WebhookDelivery{ID: uuid.New().String(), EventID: eventID, SubscriptionID: sub.ID,
				EventType: eventType, Payload: string(body), Status: "pending", NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RunWorker 持续领取到期任务；取消上下文后安全退出。
func (s *Service) RunWorker(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := s.ProcessBatch(ctx, 50); err != nil && ctx.Err() == nil {
			log.Error("Webhook 后台投递失败", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessBatch 使用行锁和跳过已锁任务的方式支持多个 open-call 实例并发工作。
func (s *Service) ProcessBatch(ctx context.Context, limit int) error {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var jobs []models.WebhookDelivery
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 超过租约的 processing 任务可由任一实例重新领取。
		if err := tx.Model(&models.WebhookDelivery{}).Where("status = ? AND locked_at < ?", "processing", now.Add(-2*time.Minute)).
			Updates(map[string]any{"status": "pending", "locked_at": nil, "next_attempt_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT * FROM oc_webhook_deliveries
			WHERE status = 'pending' AND next_attempt_at <= ?
			ORDER BY next_attempt_at, created_at FOR UPDATE SKIP LOCKED LIMIT ?`, now, limit).Scan(&jobs).Error; err != nil {
			return err
		}
		for i := range jobs {
			if err := tx.Model(&models.WebhookDelivery{}).Where("id = ?", jobs[i].ID).
				Updates(map[string]any{"status": "processing", "locked_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := range jobs {
		if err := s.processOne(ctx, jobs[i]); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

// Retry 将失败或死信任务重置为立即可领取状态。
func (s *Service) Retry(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res := s.db.WithContext(ctx).Model(&models.WebhookDelivery{}).Where("id = ?", id).Updates(map[string]any{
		"status": "pending", "attempts": 0, "last_error": "", "dead_letter_at": nil,
		"locked_at": nil, "next_attempt_at": now, "updated_at": now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("投递记录不存在")
	}
	return nil
}

// ListDeliveries 查询投递记录和死信。
func (s *Service) ListDeliveries(ctx context.Context, page, pageSize int, status, eventType string) (DeliveryList, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := s.db.WithContext(ctx).Model(&models.WebhookDelivery{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return DeliveryList{}, err
	}
	var rows []models.WebhookDelivery
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return DeliveryList{}, err
	}
	items := make([]DeliveryDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, toDelivery(row))
	}
	return DeliveryList{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// Stats 返回待投递、投递中和死信数量。
func (s *Service) Stats(ctx context.Context) (Stats, error) {
	var out Stats
	for status, target := range map[string]*int64{"pending": &out.Pending, "processing": &out.Processing, "dead_letter": &out.DeadLetter} {
		if err := s.db.WithContext(ctx).Model(&models.WebhookDelivery{}).Where("status = ?", status).Count(target).Error; err != nil {
			return out, err
		}
	}
	return out, nil
}

// processOne 仅处理已领取的任务，按订阅状态决定成功或失败结果。
func (s *Service) processOne(ctx context.Context, job models.WebhookDelivery) error {
	var sub models.WebhookSubscription
	if err := s.db.WithContext(ctx).First(&sub, "id = ?", job.SubscriptionID).Error; err != nil {
		return s.finishFailure(ctx, job, "订阅不存在或已删除")
	}
	if !sub.Enabled {
		return s.finishFailure(ctx, job, "订阅已停用")
	}
	err := s.postOnce(ctx, sub, job.EventID, []byte(job.Payload))
	if err == nil {
		now := time.Now().UTC()
		return s.db.WithContext(ctx).Model(&models.WebhookDelivery{}).Where("id = ? AND status = ?", job.ID, "processing").
			Updates(map[string]any{"status": "success", "attempts": job.Attempts + 1, "last_error": "", "locked_at": nil, "updated_at": now}).Error
	}
	return s.finishFailure(ctx, job, err.Error())
}

// finishFailure 累计尝试次数；达到上限转死信，否则按退避时间重新入队。
func (s *Service) finishFailure(ctx context.Context, job models.WebhookDelivery, message string) error {
	attempts := job.Attempts + 1
	maxRetries := s.cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 8
	}
	now := time.Now().UTC()
	updates := map[string]any{"attempts": attempts, "last_error": truncate(message, 2048), "locked_at": nil, "updated_at": now}
	if attempts >= maxRetries {
		updates["status"], updates["dead_letter_at"] = "dead_letter", now
	} else {
		updates["status"] = "pending"
		updates["next_attempt_at"] = now.Add(backoff(attempts))
	}
	return s.db.WithContext(ctx).Model(&models.WebhookDelivery{}).Where("id = ?", job.ID).Updates(updates).Error
}

// postOnce 发送一次带追踪头和 HMAC 签名的 Webhook 请求。
func (s *Service) postOnce(ctx context.Context, sub models.WebhookSubscription, eventID string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Open-VoIP-Event-ID", eventID)
	var envelope struct {
		TraceID   string `json:"trace_id"`
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		if id := observability.NormalizeID(envelope.TraceID); id != "" {
			req.Header.Set("X-Trace-ID", id)
		}
		if id := observability.NormalizeID(envelope.RequestID); id != "" {
			req.Header.Set("X-Request-ID", id)
		}
	}
	traceCtx := observability.With(ctx, observability.Context{
		TraceID: observability.NormalizeID(envelope.TraceID), RequestID: observability.NormalizeID(envelope.RequestID),
	})
	observability.Emit(traceCtx, "webhook.delivery.started", map[string]any{"event_id": eventID, "subscription_id": sub.ID})
	if sub.Secret != "" {
		mac := hmac.New(sha256.New, []byte(sub.Secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-Open-VoIP-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	response, err := s.client.Do(req)
	if err != nil {
		observability.Emit(traceCtx, "webhook.delivery.failed", map[string]any{"event_id": eventID, "subscription_id": sub.ID, "error": err.Error()})
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		observability.Emit(traceCtx, "webhook.delivery.failed", map[string]any{"event_id": eventID, "subscription_id": sub.ID, "status": response.StatusCode})
		return fmt.Errorf("HTTP %s", response.Status)
	}
	observability.Emit(traceCtx, "webhook.delivery.completed", map[string]any{"event_id": eventID, "subscription_id": sub.ID, "status": response.StatusCode})
	return nil
}

func validateTarget(value string) error {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errs.InvalidRequest("Webhook URL 无效")
	}
	if u.User != nil || u.Fragment != "" {
		return errs.InvalidRequest("Webhook URL 不能包含用户凭证或片段")
	}
	return nil
}

func normalizeTypes(values []string) []string {
	seen, out := map[string]bool{}, make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func randomSecret() (string, error) {
	raw, err := authRandomToken(32)
	return raw, err
}

// authRandomToken 放在本包内，避免 Webhook 业务依赖认证服务。
func authRandomToken(size int) (string, error) {
	value := uuid.New().String() + uuid.New().String()
	if size > 0 && len(value) > size {
		value = value[:size]
	}
	return value, nil
}

// backoff 计算下一次投递的指数退避时间。
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > time.Hour {
		return time.Hour
	}
	return d
}

func matchType(types []string, event string) bool {
	for _, value := range types {
		if value == "*" || value == event {
			return true
		}
	}
	return false
}

func toSub(row models.WebhookSubscription) SubDTO {
	var types []string
	_ = json.Unmarshal([]byte(row.EventTypes), &types)
	return SubDTO{ID: row.ID, URL: row.URL, EventTypes: types, Enabled: row.Enabled, CreatedAt: row.CreatedAt}
}

func toDelivery(row models.WebhookDelivery) DeliveryDTO {
	return DeliveryDTO{ID: row.ID, EventID: row.EventID, SubscriptionID: row.SubscriptionID, EventType: row.EventType,
		Status: row.Status, Attempts: row.Attempts, LastError: row.LastError, NextAttemptAt: row.NextAttemptAt,
		DeadLetterAt: row.DeadLetterAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func truncate(value string, size int) string {
	if len(value) > size {
		return value[:size]
	}
	return value
}
