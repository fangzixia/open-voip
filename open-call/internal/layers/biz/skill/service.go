package skill

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/store/models"
)

// DTO 技能组。
type DTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Service 技能 CRUD 与坐席绑定。
type Service struct {
	db *gorm.DB
}

// NewService 创建技能服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// List 列出技能。
func (s *Service) List(ctx context.Context) ([]DTO, error) {
	var rows []models.Skill
	if err := s.db.WithContext(ctx).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, DTO{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// Create 创建技能。
func (s *Service) Create(ctx context.Context, name string) (DTO, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return DTO{}, errs.InvalidRequest("技能名称必填")
	}
	row := models.Skill{ID: uuid.New().String(), Name: name, CreatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return DTO{}, errs.Conflict("技能名称已存在", "")
	}
	return DTO{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt}, nil
}

func (s *Service) Update(ctx context.Context, id, name string) (DTO, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return DTO{}, errs.InvalidRequest("技能名称必填")
	}
	var row models.Skill
	if err := s.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DTO{}, errs.NotFound("技能不存在")
		}
		return DTO{}, err
	}
	if err := s.db.WithContext(ctx).Model(&row).Update("name", name).Error; err != nil {
		return DTO{}, errs.Conflict("技能名称已存在", "")
	}
	row.Name = name
	return DTO{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt}, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	var n int64
	if err := s.db.WithContext(ctx).Model(&models.AgentSkill{}).Where("skill_id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return errs.Conflict("技能仍绑定坐席", "")
	}
	if err := s.db.WithContext(ctx).Model(&models.QueueSkill{}).Where("skill_id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return errs.Conflict("技能仍被队列引用", "")
	}
	res := s.db.WithContext(ctx).Delete(&models.Skill{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("技能不存在")
	}
	return nil
}

// BindAgent 覆盖坐席技能。
func (s *Service) BindAgent(ctx context.Context, agentID string, skillIDs []string) error {
	var ag models.Agent
	if err := s.db.WithContext(ctx).First(&ag, "id = ?", agentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.NotFound("坐席不存在")
		}
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		unique := map[string]struct{}{}
		for _, id := range skillIDs {
			if strings.TrimSpace(id) != "" {
				unique[id] = struct{}{}
			}
		}
		if len(unique) > 0 {
			ids := make([]string, 0, len(unique))
			for id := range unique {
				ids = append(ids, id)
			}
			var count int64
			if err := tx.Model(&models.Skill{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return errs.InvalidRequest("skill_ids 包含不存在的技能")
			}
		}
		if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentSkill{}).Error; err != nil {
			return err
		}
		for id := range unique {
			if err := tx.Create(&models.AgentSkill{AgentID: agentID, SkillID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// AgentSkills 坐席技能 ID。
func (s *Service) AgentSkills(ctx context.Context, agentID string) ([]string, error) {
	var ids []string
	err := s.db.WithContext(ctx).Model(&models.AgentSkill{}).Where("agent_id = ?", agentID).Pluck("skill_id", &ids).Error
	return ids, err
}
