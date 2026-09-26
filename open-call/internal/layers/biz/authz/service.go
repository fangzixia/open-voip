package authz

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

type Role struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	Name      string    `gorm:"not null;size:128" json:"name"`
	BuiltIn   bool      `gorm:"not null" json:"built_in"`
	CreatedAt time.Time `json:"created_at"`
}

func (Role) TableName() string { return "oc_roles" }

type Permission struct {
	Code        string `gorm:"primaryKey;size:96" json:"code"`
	Description string `json:"description"`
}

func (Permission) TableName() string { return "oc_permissions" }

type RolePermission struct {
	RoleID         string `gorm:"primaryKey;column:role_id"`
	PermissionCode string `gorm:"primaryKey;column:permission_code"`
}

func (RolePermission) TableName() string { return "oc_role_permissions" }

type UserRole struct {
	UserID string `gorm:"primaryKey;column:user_id"`
	RoleID string `gorm:"primaryKey;column:role_id"`
}

func (UserRole) TableName() string { return "oc_user_roles" }

type GroupRole struct {
	GroupName string `gorm:"primaryKey;column:group_name" json:"group_name"`
	RoleID    string `gorm:"primaryKey;column:role_id" json:"role_id"`
}

func (GroupRole) TableName() string { return "oc_oidc_group_roles" }

type Identity struct {
	Issuer  string `gorm:"primaryKey;column:issuer" json:"issuer"`
	Subject string `gorm:"primaryKey;column:subject" json:"subject"`
	UserID  string `gorm:"column:user_id" json:"user_id"`
}

func (Identity) TableName() string { return "oc_oidc_identities" }

type RoleDTO struct {
	Role
	Permissions []string `json:"permissions"`
}

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Permissions(ctx context.Context, userID string) ([]string, error) {
	var codes []string
	err := s.db.WithContext(ctx).Table("oc_role_permissions rp").
		Joins("JOIN oc_user_roles ur ON ur.role_id = rp.role_id").
		Where("ur.user_id = ?", userID).Distinct().Order("rp.permission_code").Pluck("rp.permission_code", &codes).Error
	return codes, err
}

func (s *Service) UserRoles(ctx context.Context, userID string) ([]string, error) {
	var ids []string
	err := s.db.WithContext(ctx).Model(&UserRole{}).Where("user_id = ?", userID).Order("role_id").Pluck("role_id", &ids).Error
	return ids, err
}

func (s *Service) ListPermissions(ctx context.Context) ([]Permission, error) {
	var out []Permission
	err := s.db.WithContext(ctx).Order("code").Find(&out).Error
	return out, err
}

func (s *Service) ListRoles(ctx context.Context) ([]RoleDTO, error) {
	var rows []Role
	if err := s.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RoleDTO, 0, len(rows))
	for _, r := range rows {
		var codes []string
		if err := s.db.WithContext(ctx).Model(&RolePermission{}).Where("role_id = ?", r.ID).Order("permission_code").Pluck("permission_code", &codes).Error; err != nil {
			return nil, err
		}
		out = append(out, RoleDTO{Role: r, Permissions: codes})
	}
	return out, nil
}

func (s *Service) SaveRole(ctx context.Context, id, name string, codes []string) error {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" || name == "" || len(id) > 64 || len(name) > 128 {
		return errs.InvalidRequest("角色编号和名称无效")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var role Role
		err := tx.First(&role, "id = ?", id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			role = Role{ID: id, Name: name, CreatedAt: time.Now().UTC()}
			if err = tx.Create(&role).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if role.BuiltIn {
			return errs.Forbidden("内置角色不可修改")
		}
		if err := tx.Model(&role).Update("name", name).Error; err != nil {
			return err
		}
		if len(codes) > 0 {
			var n int64
			if err := tx.Model(&Permission{}).Where("code IN ?", codes).Count(&n).Error; err != nil {
				return err
			}
			if int(n) != len(unique(codes)) {
				return errs.InvalidRequest("包含未知权限")
			}
		}
		if err := tx.Where("role_id = ?", id).Delete(&RolePermission{}).Error; err != nil {
			return err
		}
		for _, code := range unique(codes) {
			if err := tx.Create(&RolePermission{RoleID: id, PermissionCode: code}).Error; err != nil {
				return err
			}
		}
		return revokeRoleUsers(tx, id)
	})
}

func (s *Service) DeleteRole(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var r Role
		if err := tx.First(&r, "id = ?", id).Error; err != nil {
			return err
		}
		if r.BuiltIn {
			return errs.Forbidden("内置角色不可删除")
		}
		if err := revokeRoleUsers(tx, id); err != nil {
			return err
		}
		return tx.Delete(&r).Error
	})
}

func (s *Service) SetUserRoles(ctx context.Context, userID string, ids []string) error {
	ids = unique(ids)
	if len(ids) == 0 {
		return errs.InvalidRequest("至少需要一个角色")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&Role{}).Where("id IN ?", ids).Count(&n).Error; err != nil {
			return err
		}
		if int(n) != len(ids) {
			return errs.InvalidRequest("包含未知角色")
		}
		if err := tx.Where("user_id = ?", userID).Delete(&UserRole{}).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := tx.Create(&UserRole{UserID: userID, RoleID: id}).Error; err != nil {
				return err
			}
		}
		return revokeUser(tx, userID)
	})
}

func (s *Service) ListMappings(ctx context.Context) ([]GroupRole, error) {
	var out []GroupRole
	err := s.db.WithContext(ctx).Order("group_name,role_id").Find(&out).Error
	return out, err
}

func (s *Service) SetMapping(ctx context.Context, group string, ids []string) error {
	group = strings.TrimSpace(group)
	ids = unique(ids)
	if group == "" || len(group) > 255 {
		return errs.InvalidRequest("组名无效")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(ids) > 0 {
			var count int64
			if err := tx.Model(&Role{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
				return err
			}
			if int(count) != len(ids) {
				return errs.InvalidRequest("映射包含未知角色")
			}
		}
		if err := tx.Where("group_name = ?", group).Delete(&GroupRole{}).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := tx.Create(&GroupRole{GroupName: group, RoleID: id}).Error; err != nil {
				return err
			}
		}
		// 外部角色映射变更后，现有外部会话必须重新登录以获取最新组。
		return tx.Model(&models.AuthSession{}).Where("provider = ? AND revoked_at IS NULL", "oidc").Update("revoked_at", time.Now().UTC()).Error
	})
}

func (s *Service) RolesForGroups(ctx context.Context, groups []string) ([]string, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	var ids []string
	err := s.db.WithContext(ctx).Model(&GroupRole{}).Where("group_name IN ?", groups).Distinct().Order("role_id").Pluck("role_id", &ids).Error
	return ids, err
}

func (s *Service) BindIdentity(ctx context.Context, userID, issuer, subject string) error {
	if issuer == "" || subject == "" || userID == "" {
		return errs.InvalidRequest("身份绑定信息不完整")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.First(&user, "id = ?", userID).Error; err != nil {
			return errs.NotFound("用户不存在")
		}
		if err := tx.Create(&Identity{Issuer: issuer, Subject: subject, UserID: userID}).Error; err != nil {
			return errs.Conflict("外部身份已绑定", "")
		}
		return revokeUser(tx, userID)
	})
}

func (s *Service) ListIdentities(ctx context.Context, userID string) ([]Identity, error) {
	var out []Identity
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&out).Error
	return out, err
}
func (s *Service) DeleteIdentity(ctx context.Context, userID, issuer, subject string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("user_id = ? AND issuer = ? AND subject = ?", userID, issuer, subject).Delete(&Identity{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errs.NotFound("外部身份绑定不存在")
		}
		return revokeUser(tx, userID)
	})
}

func revokeRoleUsers(tx *gorm.DB, id string) error {
	var ids []string
	if err := tx.Model(&UserRole{}).Where("role_id = ?", id).Pluck("user_id", &ids).Error; err != nil {
		return err
	}
	for _, uid := range ids {
		if err := revokeUser(tx, uid); err != nil {
			return err
		}
	}
	return nil
}
func revokeUser(tx *gorm.DB, id string) error {
	if err := tx.Model(&models.User{}).Where("id = ?", id).Update("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
		return err
	}
	return tx.Model(&models.AuthSession{}).Where("user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", time.Now().UTC()).Error
}
func unique(in []string) []string {
	m := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !m[v] {
			m[v] = true
			out = append(out, v)
		}
	}
	return out
}

// NewOIDCUser 仅在稳定外部身份尚未绑定时创建用户。
func (s *Service) NewOIDCUser(ctx context.Context, issuer, subject, loginName, username, employeeNo string, roles []string) (models.User, error) {
	loginName = strings.TrimSpace(loginName)
	username = strings.TrimSpace(username)
	employeeNo = strings.TrimSpace(employeeNo)
	var u models.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identity Identity
		if err := tx.Where("issuer = ? AND subject = ?", issuer, subject).First(&identity).Error; err == nil {
			return tx.First(&u, "id = ?", identity.UserID).Error
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		query := tx.Model(&models.User{}).Where("LOWER(username) = LOWER(?) OR employee_no = ?", loginName, employeeNo)
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errs.Conflict("同登录名或工号用户已存在，请管理员先绑定外部身份", "")
		}
		u = models.User{ID: uuid.NewString(), Username: loginName, EmployeeNo: employeeNo, PasswordHash: "!oidc-only", Role: "agent", DisplayName: username, AuthVersion: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		if err := tx.Create(&Identity{Issuer: issuer, Subject: subject, UserID: u.ID}).Error; err != nil {
			return err
		}
		for _, id := range unique(roles) {
			if err := tx.Create(&UserRole{UserID: u.ID, RoleID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return u, err
}
