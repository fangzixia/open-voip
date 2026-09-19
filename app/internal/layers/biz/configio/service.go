package configio

import (
	"context"
	"time"

	"gorm.io/gorm"

	"open-voip/internal/store/models"
)

// Bundle 可导出/导入的组织配置（不含密码哈希明文还原为占位）。
type Bundle struct {
	ExportedAt time.Time      `json:"exported_at"`
	Users      []UserDump     `json:"users"`
	Queues     []models.Queue `json:"queues"`
	Skills     []models.Skill `json:"skills"`
	IVR        []models.IVRFlow `json:"ivr_flows"`
	DIDs       []models.DIDRoute `json:"did_routes"`
}

// UserDump 不含密码。
type UserDump struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Disabled    bool   `json:"disabled"`
	Extension   string `json:"extension,omitempty"`
	VideoCapable bool  `json:"video_capable,omitempty"`
}

// Service 配置导入导出。
type Service struct {
	db *gorm.DB
}

// NewService 创建配置导入导出服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// Export 导出用户/队列/技能/IVR/DID。
func (s *Service) Export(ctx context.Context) (Bundle, error) {
	out := Bundle{ExportedAt: time.Now().UTC()}
	var users []models.User
	if err := s.db.WithContext(ctx).Find(&users).Error; err != nil {
		return out, err
	}
	for _, u := range users {
		d := UserDump{ID: u.ID, Username: u.Username, Role: u.Role, DisplayName: u.DisplayName, Disabled: u.Disabled}
		var ag models.Agent
		if err := s.db.WithContext(ctx).Where("user_id = ?", u.ID).First(&ag).Error; err == nil {
			d.Extension = ag.Extension
			d.VideoCapable = ag.VideoCapable
		}
		out.Users = append(out.Users, d)
	}
	_ = s.db.WithContext(ctx).Find(&out.Queues).Error
	_ = s.db.WithContext(ctx).Find(&out.Skills).Error
	_ = s.db.WithContext(ctx).Find(&out.IVR).Error
	_ = s.db.WithContext(ctx).Find(&out.DIDs).Error
	return out, nil
}

// Import 以 upsert 方式写入队列/技能/DID（用户需已存在，避免覆盖密码）。
func (s *Service) Import(ctx context.Context, b Bundle) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range b.Skills {
			if b.Skills[i].ID == "" {
				continue
			}
			if err := tx.Where("id = ?", b.Skills[i].ID).Assign(b.Skills[i]).FirstOrCreate(&b.Skills[i]).Error; err != nil {
				return err
			}
		}
		for i := range b.Queues {
			if b.Queues[i].ID == "" {
				continue
			}
			if err := tx.Where("id = ?", b.Queues[i].ID).Assign(b.Queues[i]).FirstOrCreate(&b.Queues[i]).Error; err != nil {
				return err
			}
		}
		for i := range b.DIDs {
			if b.DIDs[i].ID == "" {
				continue
			}
			if err := tx.Where("id = ?", b.DIDs[i].ID).Assign(b.DIDs[i]).FirstOrCreate(&b.DIDs[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
