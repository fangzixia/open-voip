package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-voip/internal/config"
	"open-voip/internal/errs"
	"open-voip/internal/ports"
	"open-voip/internal/store/models"
)

// SubDTO 订阅。
type SubDTO struct {
	ID         string   `json:"id"`
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
	Enabled    bool     `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

// Service Webhook 订阅与投递。
type Service struct {
	db  *gorm.DB
	cfg config.WebhookConfig
}

// NewService 创建 Webhook 服务。
func NewService(db *gorm.DB, cfg config.WebhookConfig) *Service {
	return &Service{db: db, cfg: cfg}
}

var _ ports.WebhookDispatcher = (*Service)(nil)

// List 订阅列表。
func (s *Service) List(ctx context.Context) ([]SubDTO, error) {
	var rows []models.WebhookSubscription
	if err := s.db.WithContext(ctx).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SubDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSub(r))
	}
	return out, nil
}

// Create 创建订阅。
func (s *Service) Create(ctx context.Context, url string, types []string, secret string) (SubDTO, error) {
	if url == "" || len(types) == 0 {
		return SubDTO{}, errs.InvalidRequest("url 与 event_types 必填")
	}
	raw, _ := json.Marshal(types)
	row := models.WebhookSubscription{
		ID: uuid.New().String(), URL: url, EventTypes: string(raw), Secret: secret, Enabled: true, CreatedAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return SubDTO{}, err
	}
	return toSub(row), nil
}

func (s *Service) Dispatch(ctx context.Context, eventType string, payload map[string]any) error {
	var rows []models.WebhookSubscription
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"type": eventType, "ts": time.Now().UTC().Format(time.RFC3339), "payload": payload})
	for _, sub := range rows {
		var types []string
		_ = json.Unmarshal([]byte(sub.EventTypes), &types)
		if !matchType(types, eventType) {
			continue
		}
		s.deliver(ctx, sub, eventType, body)
	}
	return nil
}

// Retry 管理员重试失败投递。
func (s *Service) Retry(ctx context.Context, id string) error {
	var d models.WebhookDelivery
	if err := s.db.WithContext(ctx).First(&d, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.NotFound("投递记录不存在")
		}
		return err
	}
	var sub models.WebhookSubscription
	if err := s.db.WithContext(ctx).First(&sub, "id = ?", d.SubscriptionID).Error; err != nil {
		return errs.NotFound("订阅不存在")
	}
	s.postOnce(ctx, sub, &d, []byte(d.Payload))
	return s.db.WithContext(ctx).Save(&d).Error
}

func (s *Service) deliver(ctx context.Context, sub models.WebhookSubscription, eventType string, body []byte) {
	d := models.WebhookDelivery{
		ID: uuid.New().String(), SubscriptionID: sub.ID, EventType: eventType, Payload: string(body),
		Status: "pending", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	n := s.cfg.MaxRetries
	if n <= 0 {
		n = 3
	}
	for i := 0; i < n; i++ {
		if s.postOnce(ctx, sub, &d, body) {
			break
		}
	}
	_ = s.db.WithContext(ctx).Create(&d)
}

func (s *Service) postOnce(ctx context.Context, sub models.WebhookSubscription, d *models.WebhookDelivery, body []byte) bool {
	d.Attempts++
	d.UpdatedAt = time.Now().UTC()
	timeout := time.Duration(s.cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, sub.URL, bytes.NewReader(body))
	if err != nil {
		d.Status, d.LastError = "failed", err.Error()
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	if sub.Secret != "" {
		mac := hmac.New(sha256.New, []byte(sub.Secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-Open-VoIP-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		d.Status, d.LastError = "failed", err.Error()
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		d.Status, d.LastError = "success", ""
		return true
	}
	d.Status, d.LastError = "failed", resp.Status
	return false
}

func matchType(types []string, ev string) bool {
	for _, t := range types {
		if t == "*" || t == ev {
			return true
		}
	}
	return false
}

func toSub(r models.WebhookSubscription) SubDTO {
	var types []string
	_ = json.Unmarshal([]byte(r.EventTypes), &types)
	return SubDTO{ID: r.ID, URL: r.URL, EventTypes: types, Enabled: r.Enabled, CreatedAt: r.CreatedAt}
}
