package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"open-switch/internal/errs"
	"open-switch/internal/store/models"
)

// ApplicationRecord 运行时租户快照（含 callback 投递所需明文 secret）。
type ApplicationRecord struct {
	ID                 string
	Secret             string
	EventsCallbackURL  string
	EventRetentionDays int
	MaxConcurrentCalls int
	Enabled            bool
}

// ApplicationRegistry 加载并解析 integrator 凭据。
type ApplicationRegistry struct {
	DB *gorm.DB

	mu    sync.RWMutex
	byID  map[string]ApplicationRecord
	bySec map[string]string
}

func NewApplicationRegistry(db *gorm.DB) *ApplicationRegistry {
	return &ApplicationRegistry{DB: db, byID: map[string]ApplicationRecord{}, bySec: map[string]string{}}
}

func (r *ApplicationRegistry) Reload(ctx context.Context) error {
	var rows []models.Application
	if err := r.DB.WithContext(ctx).Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return err
	}
	byID := make(map[string]ApplicationRecord, len(rows))
	bySec := make(map[string]string, len(rows))
	for _, row := range rows {
		rec := ApplicationRecord{
			ID:                 row.ID,
			Secret:             row.Secret,
			EventsCallbackURL:  strings.TrimSpace(row.EventsCallbackURL),
			EventRetentionDays: row.EventRetentionDays,
			MaxConcurrentCalls: row.MaxConcurrentCalls,
			Enabled:            row.Enabled,
		}
		byID[row.ID] = rec
		bySec[row.Secret] = row.ID
	}
	r.mu.Lock()
	r.byID = byID
	r.bySec = bySec
	r.mu.Unlock()
	return nil
}

func (r *ApplicationRegistry) ResolveSecret(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.bySec[token]
	return id, ok
}

func (r *ApplicationRegistry) Get(id string) (ApplicationRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.byID[id]
	return rec, ok
}

func (r *ApplicationRegistry) List() []ApplicationRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ApplicationRecord, 0, len(r.byID))
	for _, rec := range r.byID {
		out = append(out, rec)
	}
	return out
}

func (r *ApplicationRegistry) Exists(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.byID[id]
	return ok
}

// SetTestRecord 仅供单元测试注入租户凭据。
func (r *ApplicationRegistry) SetTestRecord(rec ApplicationRecord) {
	r.mu.Lock()
	r.byID[rec.ID] = rec
	r.bySec[rec.Secret] = rec.ID
	r.mu.Unlock()
}

type RegisterApplicationInput struct {
	ApplicationID      string
	EventsCallbackURL  string
	EventRetentionDays int
	MaxConcurrentCalls int
}

type RegisterApplicationResult struct {
	ApplicationID string
	Secret        string
	Created       bool
}

// Register 创建或更新 integrator；新建时生成 secret 并返回明文一次。
func (r *ApplicationRegistry) Register(ctx context.Context, in RegisterApplicationInput) (RegisterApplicationResult, error) {
	id := strings.TrimSpace(in.ApplicationID)
	if id == "" || strings.ContainsAny(id, " /\\:@\r\n\t") {
		return RegisterApplicationResult{}, errs.InvalidRequest("application_id 无效")
	}
	url := strings.TrimSpace(in.EventsCallbackURL)
	if url == "" {
		return RegisterApplicationResult{}, errs.InvalidRequest("events_callback_url 不能为空")
	}
	retention := in.EventRetentionDays
	if retention < 1 {
		retention = 14
	}
	maxCalls := in.MaxConcurrentCalls
	if maxCalls < 1 {
		maxCalls = 100
	}

	var existing models.Application
	err := r.DB.WithContext(ctx).First(&existing, "id = ?", id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return RegisterApplicationResult{}, err
	}
	now := time.Now().UTC()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		secret, err := generateIntegratorSecret()
		if err != nil {
			return RegisterApplicationResult{}, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
		if err != nil {
			return RegisterApplicationResult{}, err
		}
		row := models.Application{
			ID: id, SecretHash: string(hash), Secret: secret, EventsCallbackURL: url,
			EventRetentionDays: retention, MaxConcurrentCalls: maxCalls, Enabled: true,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := r.DB.WithContext(ctx).Create(&row).Error; err != nil {
			return RegisterApplicationResult{}, err
		}
		if err := r.Reload(ctx); err != nil {
			return RegisterApplicationResult{}, err
		}
		return RegisterApplicationResult{ApplicationID: id, Secret: secret, Created: true}, nil
	}

	updates := map[string]any{
		"events_callback_url":  url,
		"event_retention_days": retention,
		"max_concurrent_calls": maxCalls,
		"updated_at":           now,
	}
	if err := r.DB.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
		return RegisterApplicationResult{}, err
	}
	if err := r.Reload(ctx); err != nil {
		return RegisterApplicationResult{}, err
	}
	return RegisterApplicationResult{ApplicationID: id, Created: false}, nil
}

func (r *ApplicationRegistry) RotateSecret(ctx context.Context, applicationID string) (string, error) {
	id := strings.TrimSpace(applicationID)
	var row models.Application
	if err := r.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errs.NotFound("应用不存在")
		}
		return "", err
	}
	secret, err := generateIntegratorSecret()
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if err := r.DB.WithContext(ctx).Model(&row).Updates(map[string]any{
		"secret":      secret,
		"secret_hash": string(hash),
		"updated_at":  time.Now().UTC(),
	}).Error; err != nil {
		return "", err
	}
	if err := r.Reload(ctx); err != nil {
		return "", err
	}
	return secret, nil
}

func generateIntegratorSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成 integrator secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
