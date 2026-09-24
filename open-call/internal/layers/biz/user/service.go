package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/store/models"
)

// DTO 用户对外表示。
type DTO struct {
	TerminalType       string    `json:"terminal_type,omitempty"`
	SIPUsername        string    `json:"sip_username,omitempty"`
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	DisplayName        string    `json:"display_name,omitempty"`
	Role               string    `json:"role"`
	Disabled           bool      `json:"disabled"`
	AgentID            string    `json:"agent_id,omitempty"`
	Extension          string    `json:"extension,omitempty"`
	VideoCapable       bool      `json:"video_capable,omitempty"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
}

// CreateInput 创建用户。
type CreateInput struct {
	TerminalType string `json:"terminal_type"`
	SIPUsername  string `json:"sip_username"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Role         string `json:"role"`
	DisplayName  string `json:"display_name"`
	Extension    string `json:"extension"`
	VideoCapable bool   `json:"video_capable"`
}

// UpdateInput 更新用户。
type UpdateInput struct {
	TerminalType *string `json:"terminal_type"`
	SIPUsername  *string `json:"sip_username"`
	Role         *string `json:"role"`
	Disabled     *bool   `json:"disabled"`
	DisplayName  *string `json:"display_name"`
	VideoCapable *bool   `json:"video_capable"`
	Extension    *string `json:"extension"`
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
	if err := auth.ValidatePassword(in.Password); err != nil {
		return DTO{}, err
	}
	if in.Role != "admin" && in.Role != "supervisor" && in.Role != "agent" {
		return DTO{}, errs.InvalidRequest("角色无效")
	}
	if in.Role == "agent" && strings.TrimSpace(in.Extension) == "" {
		return DTO{}, errs.InvalidRequest("坐席必须设置分机号")
	}
	if in.TerminalType == "" {
		in.TerminalType = "webrtc"
	}
	if err := validateTerminal(in.TerminalType, in.SIPUsername, in.Extension, in.VideoCapable); err != nil {
		return DTO{}, err
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
		AuthVersion:  1,
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
				TerminalType: in.TerminalType, SIPUsername: strings.TrimSpace(in.SIPUsername),
				CreatedAt: now,
				UpdatedAt: now,
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
	targetRole := u.Role
	authChanged := false
	if in.Role != nil {
		if *in.Role != "agent" && *in.Role != "admin" && *in.Role != "supervisor" {
			return DTO{}, errs.InvalidRequest("角色无效")
		}
		targetRole = *in.Role
		updates["role"] = targetRole
		authChanged = targetRole != u.Role
	}
	if in.Disabled != nil {
		updates["disabled"] = *in.Disabled
		authChanged = authChanged || *in.Disabled != u.Disabled
	}
	if in.DisplayName != nil {
		updates["display_name"] = *in.DisplayName
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ag models.Agent
		agErr := tx.Where("user_id = ?", id).First(&ag).Error
		hasAgent := agErr == nil
		if agErr != nil && !errors.Is(agErr, gorm.ErrRecordNotFound) {
			return agErr
		}
		if hasAgent {
			var active int64
			if err := tx.Model(&models.AgentSession{}).Where("agent_id = ? AND state <> ?", ag.ID, "offline").Count(&active).Error; err != nil {
				return err
			}
			terminalChange := in.TerminalType != nil || in.SIPUsername != nil || in.VideoCapable != nil || in.Extension != nil || targetRole == "admin"
			if terminalChange && active > 0 {
				return errs.Conflict("请先签出再修改坐席角色或终端", "")
			}
			if in.Disabled != nil && *in.Disabled {
				var calls int64
				if err := tx.Model(&models.AgentSession{}).Where("agent_id = ? AND state IN ?", ag.ID, []string{"ringing", "on_call"}).Count(&calls).Error; err != nil {
					return err
				}
				if calls > 0 {
					return errs.Conflict("请先结束坐席当前通话再禁用账号", "")
				}
				if err := tx.Where("agent_id = ?", ag.ID).Delete(&models.AgentSession{}).Error; err != nil {
					return err
				}
			}
		}
		needsAgent := targetRole == "agent" || (targetRole == "supervisor" && (hasAgent || (in.Extension != nil && strings.TrimSpace(*in.Extension) != "")))
		if needsAgent && !hasAgent {
			ext := ""
			if in.Extension != nil {
				ext = strings.TrimSpace(*in.Extension)
			}
			if targetRole == "agent" && ext == "" {
				return errs.InvalidRequest("转换为坐席角色时必须提供 extension")
			}
			kind, sipName, video := "webrtc", "", false
			if in.TerminalType != nil {
				kind = *in.TerminalType
			}
			if in.SIPUsername != nil {
				sipName = strings.TrimSpace(*in.SIPUsername)
			}
			if in.VideoCapable != nil {
				video = *in.VideoCapable
			}
			if err := validateTerminal(kind, sipName, ext, video); err != nil {
				return err
			}
			now := time.Now().UTC()
			ag = models.Agent{ID: uuid.New().String(), UserID: id, Extension: ext, TerminalType: kind, SIPUsername: sipName, VideoCapable: video, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&ag).Error; err != nil {
				return err
			}
			hasAgent = true
		}
		if hasAgent && targetRole == "admin" {
			if err := tx.Delete(&ag).Error; err != nil {
				return err
			}
			hasAgent = false
		}
		if hasAgent && targetRole != "admin" {
			if in.TerminalType != nil {
				ag.TerminalType = *in.TerminalType
			}
			if in.SIPUsername != nil {
				ag.SIPUsername = strings.TrimSpace(*in.SIPUsername)
			}
			if in.VideoCapable != nil {
				ag.VideoCapable = *in.VideoCapable
			}
			if in.Extension != nil {
				ag.Extension = strings.TrimSpace(*in.Extension)
			}
			if err := validateTerminal(ag.TerminalType, ag.SIPUsername, ag.Extension, ag.VideoCapable); err != nil {
				return err
			}
			ag.UpdatedAt = time.Now().UTC()
			if err := tx.Save(&ag).Error; err != nil {
				return err
			}
		}
		if authChanged {
			updates["auth_version"] = gorm.Expr("auth_version + 1")
			now := time.Now().UTC()
			if err := tx.Model(&models.AuthSession{}).Where("user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", now).Error; err != nil {
				return err
			}
		}
		return tx.Model(&u).Updates(updates).Error
	}); err != nil {
		return DTO{}, err
	}
	return s.Get(ctx, id)
}

// Delete 删除用户及关联坐席。
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ag models.Agent
		if err := tx.Where("user_id = ?", id).First(&ag).Error; err == nil {
			var active int64
			if err := tx.Model(&models.AgentSession{}).Where("agent_id = ? AND state IN ?", ag.ID, []string{"ringing", "on_call"}).Count(&active).Error; err != nil {
				return err
			}
			if active > 0 {
				return errs.Conflict("请先结束坐席通话", "")
			}
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
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	plain := "TmpA9-" + hex.EncodeToString(buf)
	hash, err := auth.HashPassword(plain)
	if err != nil {
		return "", err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.User{}).Where("id = ?", id).Updates(map[string]any{
			"password_hash": hash, "must_change_password": true, "auth_version": gorm.Expr("auth_version + 1"), "updated_at": time.Now().UTC(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.NotFound("用户不存在")
		}
		now := time.Now().UTC()
		return tx.Model(&models.AuthSession{}).Where("user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", now).Error
	})
	if err != nil {
		return "", err
	}
	return plain, nil
}

func (s *Service) toDTO(ctx context.Context, u models.User) (DTO, error) {
	out := DTO{
		ID:                 u.ID,
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		Role:               u.Role,
		Disabled:           u.Disabled,
		MustChangePassword: u.MustChangePassword,
		CreatedAt:          u.CreatedAt,
	}
	var ag models.Agent
	if err := s.db.WithContext(ctx).Where("user_id = ?", u.ID).First(&ag).Error; err == nil {
		out.AgentID = ag.ID
		out.Extension = ag.Extension
		out.VideoCapable = ag.VideoCapable
		out.TerminalType = ag.TerminalType
		out.SIPUsername = ag.SIPUsername
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

func validateTerminal(kind, username, extension string, video bool) error {
	if kind != "webrtc" && kind != "sip" {
		return errs.InvalidRequest("terminal_type 必须为 webrtc 或 sip")
	}
	if kind == "sip" && (username == "" || username != extension || video) {
		return errs.InvalidRequest("SIP 用户名须等于分机号，且只支持语音坐席")
	}
	if kind == "webrtc" && username != "" {
		return errs.InvalidRequest("WebRTC 坐席不能设置 SIP 用户名")
	}
	return nil
}
