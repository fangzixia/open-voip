package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"github.com/google/uuid"

	"open-voip/internal/errs"
	"open-voip/internal/layers/biz/auth"
	"open-voip/internal/store/models"
)

// DTO 用户对外表示。
type DTO struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"display_name,omitempty"`
	Role         string    `json:"role"`
	Disabled     bool      `json:"disabled"`
	AgentID      string    `json:"agent_id,omitempty"`
	Extension    string    `json:"extension,omitempty"`
	VideoCapable bool      `json:"video_capable,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateInput 创建用户。
type CreateInput struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	Role         string `json:"role"`
	DisplayName  string `json:"display_name"`
	Extension    string `json:"extension"`
	VideoCapable bool   `json:"video_capable"`
}

// UpdateInput 更新用户。
type UpdateInput struct {
	Role         *string `json:"role"`
	Disabled     *bool   `json:"disabled"`
	DisplayName  *string `json:"display_name"`
	VideoCapable *bool   `json:"video_capable"`
}

// ListResult 分页用户。
type ListResult struct {
	Items    []DTO `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// Service 用户与坐席账号 CRUD。
type Service struct {
	db *gorm.DB
}

// NewService 创建用户服务。
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// List 分页列出用户。
func (s *Service) List(ctx context.Context, page, pageSize int) (ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&models.User{}).Count(&total).Error; err != nil {
		return ListResult{}, err
	}
	var users []models.User
	if err := s.db.WithContext(ctx).Order("created_at").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		return ListResult{}, err
	}
	items := make([]DTO, 0, len(users))
	for _, u := range users {
		dto, err := s.toDTO(ctx, u)
		if err != nil {
			return ListResult{}, err
		}
		items = append(items, dto)
	}
	return ListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

// Get 按 ID 获取。
func (s *Service) Get(ctx context.Context, id string) (DTO, error) {
	var u models.User
	if err := s.db.WithContext(ctx).First(&u, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DTO{}, errs.NotFound("用户不存在")
		}
		return DTO{}, err
	}
	return s.toDTO(ctx, u)
}

// Create 创建用户；role=agent 时同时创建坐席。
func (s *Service) Create(ctx context.Context, in CreateInput) (DTO, error) {
	in.Username = strings.TrimSpace(in.Username)
	in.Role = strings.TrimSpace(in.Role)
	if in.Username == "" || in.Password == "" {
		return DTO{}, errs.InvalidRequest("用户名与密码必填")
	}
	if in.Role != "admin" && in.Role != "supervisor" && in.Role != "agent" {
		return DTO{}, errs.InvalidRequest("角色无效")
	}
	if in.Role == "agent" && strings.TrimSpace(in.Extension) == "" {
		return DTO{}, errs.InvalidRequest("坐席必须设置分机号")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return DTO{}, err
	}
	now := time.Now().UTC()
	u := models.User{
		ID:           uuid.New().String(),
		Username:     in.Username,
		PasswordHash: hash,
		Role:         in.Role,
		DisplayName:  in.DisplayName,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		if in.Role == "agent" || (in.Role == "supervisor" && in.Extension != "") {
			ag := models.Agent{
				ID:           uuid.New().String(),
				UserID:       u.ID,
				Extension:    strings.TrimSpace(in.Extension),
				VideoCapable: in.VideoCapable,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			return tx.Create(&ag).Error
		}
		return nil
	})
	if err != nil {
		if isUnique(err) {
			return DTO{}, errs.Conflict("用户名或分机号已存在", "")
		}
		return DTO{}, err
	}
	return s.Get(ctx, u.ID)
}

// Update 更新角色/禁用/展示名。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (DTO, error) {
	var u models.User
	if err := s.db.WithContext(ctx).First(&u, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DTO{}, errs.NotFound("用户不存在")
		}
		return DTO{}, err
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if in.Role != nil {
		updates["role"] = *in.Role
	}
	if in.Disabled != nil {
		updates["disabled"] = *in.Disabled
	}
	if in.DisplayName != nil {
		updates["display_name"] = *in.DisplayName
	}
	if err := s.db.WithContext(ctx).Model(&u).Updates(updates).Error; err != nil {
		return DTO{}, err
	}
	if in.VideoCapable != nil {
		s.db.WithContext(ctx).Model(&models.Agent{}).Where("user_id = ?", id).Update("video_capable", *in.VideoCapable)
	}
	return s.Get(ctx, id)
}

// Delete 删除用户及关联坐席。
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ag models.Agent
		if err := tx.Where("user_id = ?", id).First(&ag).Error; err == nil {
			tx.Where("agent_id = ?", ag.ID).Delete(&models.QueueAgent{})
			tx.Where("agent_id = ?", ag.ID).Delete(&models.AgentSessionQueue{})
			tx.Where("agent_id = ?", ag.ID).Delete(&models.AgentSession{})
			tx.Delete(&ag)
		}
		res := tx.Delete(&models.User{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.NotFound("用户不存在")
		}
		return nil
	})
}

// ResetPassword 生成临时密码。
func (s *Service) ResetPassword(ctx context.Context, id string) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	plain := hex.EncodeToString(buf)
	hash, err := auth.HashPassword(plain)
	if err != nil {
		return "", err
	}
	res := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Updates(map[string]any{
		"password_hash": hash,
		"updated_at":    time.Now().UTC(),
	})
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", errs.NotFound("用户不存在")
	}
	return plain, nil
}

func (s *Service) toDTO(ctx context.Context, u models.User) (DTO, error) {
	out := DTO{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Role:        u.Role,
		Disabled:    u.Disabled,
		CreatedAt:   u.CreatedAt,
	}
	var ag models.Agent
	if err := s.db.WithContext(ctx).Where("user_id = ?", u.ID).First(&ag).Error; err == nil {
		out.AgentID = ag.ID
		out.Extension = ag.Extension
		out.VideoCapable = ag.VideoCapable
	}
	return out, nil
}

func isUnique(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
}
