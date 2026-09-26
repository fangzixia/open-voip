package configio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-call/internal/errs"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/layers/biz/authz"
	"open-call/internal/store/models"
)

const schemaVersion = 3

// Bundle 是可校验、可事务恢复的组织业务配置。
type Bundle struct {
	SchemaVersion int                           `json:"schema_version"`
	ExportedAt    time.Time                     `json:"exported_at"`
	Users         []UserDump                    `json:"users"`
	Roles         []authz.RoleDTO               `json:"roles,omitempty"`
	GroupMappings []authz.GroupRole             `json:"group_mappings,omitempty"`
	Agents        []models.Agent                `json:"agents"`
	Queues        []models.Queue                `json:"queues"`
	Skills        []models.Skill                `json:"skills"`
	AgentSkills   []models.AgentSkill           `json:"agent_skills"`
	QueueSkills   []models.QueueSkill           `json:"queue_skills"`
	QueueAgents   []models.QueueAgent           `json:"queue_agents"`
	IVRFlows      []models.IVRFlow              `json:"ivr_flows"`
	IVRVersions   []models.IVRPublishedSnapshot `json:"ivr_versions"`
	DIDs          []models.DIDRoute             `json:"did_routes"`
	Webhooks      []models.WebhookSubscription  `json:"webhook_subscriptions"`
}

// UserDump 不包含密码哈希；新恢复的账号会禁用并要求管理员重置密码。
type UserDump struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Email       string   `json:"email,omitempty"`
	Role        string   `json:"role"`
	Roles       []string `json:"roles,omitempty"`
	DisplayName string   `json:"display_name"`
	Disabled    bool     `json:"disabled"`
}

// ImportOptions 控制配置导入行为。
type ImportOptions struct {
	DryRun bool
	// Mode 支持 merge 或 replace-bindings；后者会以文件内容覆盖所有绑定关系。
	Mode string
}

// ImportReport 描述预检查或导入结果。
type ImportReport struct {
	SchemaVersion int            `json:"schema_version"`
	DryRun        bool           `json:"dry_run"`
	Mode          string         `json:"mode"`
	Counts        map[string]int `json:"counts"`
	Warnings      []string       `json:"warnings,omitempty"`
}

// Service 提供业务配置的完整导出、预检查和事务导入。
type Service struct{ db *gorm.DB }

// NewService 创建配置导入导出服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// Export 导出用户、坐席、路由、绑定、IVR 版本及 Webhook 配置。
func (s *Service) Export(ctx context.Context) (Bundle, error) {
	out := Bundle{SchemaVersion: schemaVersion, ExportedAt: time.Now().UTC()}
	var users []models.User
	if err := s.db.WithContext(ctx).Order("created_at").Find(&users).Error; err != nil {
		return out, err
	}
	for _, user := range users {
		ids, err := authz.NewService(s.db).UserRoles(ctx, user.ID)
		if err != nil {
			return out, err
		}
		out.Users = append(out.Users, UserDump{ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role, Roles: ids, DisplayName: user.DisplayName, Disabled: user.Disabled})
	}
	var err error
	out.Roles, err = authz.NewService(s.db).ListRoles(ctx)
	if err != nil {
		return out, err
	}
	out.GroupMappings, err = authz.NewService(s.db).ListMappings(ctx)
	if err != nil {
		return out, err
	}
	queries := []struct {
		target any
		order  string
	}{
		{&out.Agents, "id"}, {&out.Skills, "id"}, {&out.IVRFlows, "id"}, {&out.IVRVersions, "flow_id, version"},
		{&out.Queues, "id"}, {&out.AgentSkills, "agent_id, skill_id"}, {&out.QueueSkills, "queue_id, skill_id"},
		{&out.QueueAgents, "queue_id, agent_id"}, {&out.DIDs, "d_id"}, {&out.Webhooks, "id"},
	}
	for _, query := range queries {
		if err := s.db.WithContext(ctx).Order(query.order).Find(query.target).Error; err != nil {
			return out, err
		}
	}
	return out, nil
}

// Import 保留旧接口，默认使用合并模式执行事务导入。
func (s *Service) Import(ctx context.Context, bundle Bundle) error {
	_, err := s.ImportWithOptions(ctx, bundle, ImportOptions{Mode: "merge"})
	return err
}

// ImportWithOptions 先完成全量引用校验，再在单个事务中恢复配置。
func (s *Service) ImportWithOptions(ctx context.Context, bundle Bundle, options ImportOptions) (ImportReport, error) {
	if options.Mode == "" {
		options.Mode = "merge"
	}
	if options.Mode != "merge" && options.Mode != "replace-bindings" {
		return ImportReport{}, errs.InvalidRequest("mode 必须为 merge 或 replace-bindings")
	}
	report, err := validate(bundle, options)
	if err != nil {
		return report, err
	}
	if options.DryRun {
		report.DryRun = true
		return report, nil
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return importTransaction(tx, bundle, options, &report) })
	return report, err
}

func validate(bundle Bundle, options ImportOptions) (ImportReport, error) {
	report := ImportReport{SchemaVersion: bundle.SchemaVersion, Mode: options.Mode, Counts: map[string]int{
		"users": len(bundle.Users), "agents": len(bundle.Agents), "skills": len(bundle.Skills), "queues": len(bundle.Queues),
		"ivr_flows": len(bundle.IVRFlows), "ivr_versions": len(bundle.IVRVersions), "dids": len(bundle.DIDs), "webhooks": len(bundle.Webhooks),
	}}
	if bundle.SchemaVersion != schemaVersion && bundle.SchemaVersion != 2 {
		return report, errs.InvalidRequest(fmt.Sprintf("不支持的 schema_version %d，当前为 %d", bundle.SchemaVersion, schemaVersion))
	}
	users, agents, skills, queues, flows := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, user := range bundle.Users {
		if user.ID == "" || strings.TrimSpace(user.Username) == "" {
			return report, errs.InvalidRequest("用户 id 与 username 必填")
		}
		if user.Role != "admin" && user.Role != "supervisor" && user.Role != "agent" {
			return report, errs.InvalidRequest("用户角色无效: " + user.Username)
		}
		users[user.ID] = true
	}
	for _, agent := range bundle.Agents {
		if !users[agent.UserID] {
			return report, errs.InvalidRequest("坐席引用了不存在的用户: " + agent.ID)
		}
		agents[agent.ID] = true
	}
	for _, skill := range bundle.Skills {
		skills[skill.ID] = true
	}
	for _, flow := range bundle.IVRFlows {
		flows[flow.ID] = true
	}
	for _, queue := range bundle.Queues {
		queues[queue.ID] = true
	}
	for _, queue := range bundle.Queues {
		if queue.OverflowQueueID != nil && !queues[*queue.OverflowQueueID] {
			return report, errs.InvalidRequest("队列引用了不存在的溢出队列: " + queue.ID)
		}
		if queue.IVRFlowID != nil && !flows[*queue.IVRFlowID] {
			return report, errs.InvalidRequest("队列引用了不存在的 IVR: " + queue.ID)
		}
	}
	for _, row := range bundle.AgentSkills {
		if !agents[row.AgentID] || !skills[row.SkillID] {
			return report, errs.InvalidRequest("agent_skills 包含无效引用")
		}
	}
	for _, row := range bundle.QueueSkills {
		if !queues[row.QueueID] || !skills[row.SkillID] {
			return report, errs.InvalidRequest("queue_skills 包含无效引用")
		}
	}
	for _, row := range bundle.QueueAgents {
		if !queues[row.QueueID] || !agents[row.AgentID] {
			return report, errs.InvalidRequest("queue_agents 包含无效引用")
		}
	}
	for _, row := range bundle.DIDs {
		if !queues[row.QueueID] {
			return report, errs.InvalidRequest("DID 引用了不存在的队列: " + row.DID)
		}
	}
	for _, row := range bundle.IVRVersions {
		if !flows[row.FlowID] {
			return report, errs.InvalidRequest("IVR 版本引用了不存在的流程")
		}
	}
	if len(bundle.Users) > 0 {
		report.Warnings = append(report.Warnings, "导入文件不包含密码；新账号会被禁用并要求管理员重置密码")
	}
	return report, nil
}

func importTransaction(tx *gorm.DB, bundle Bundle, options ImportOptions, report *ImportReport) error {
	for _, dump := range bundle.Roles {
		if dump.BuiltIn {
			continue
		}
		role := authz.Role{ID: dump.ID, Name: dump.Name, BuiltIn: false, CreatedAt: time.Now().UTC()}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"name"})}).Create(&role).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", dump.ID).Delete(&authz.RolePermission{}).Error; err != nil {
			return err
		}
		for _, code := range dump.Permissions {
			if err := tx.Create(&authz.RolePermission{RoleID: dump.ID, PermissionCode: code}).Error; err != nil {
				return err
			}
		}
	}
	for _, dump := range bundle.Users {
		var existing models.User
		err := tx.First(&existing, "id = ?", dump.ID).Error
		if errorsIsNotFound(err) {
			temporary := "RestoreA9-" + strings.ReplaceAll(uuid.New().String(), "-", "")
			hash, hashErr := auth.HashPassword(temporary)
			if hashErr != nil {
				return hashErr
			}
			row := models.User{ID: dump.ID, Username: dump.Username, Email: dump.Email, PasswordHash: hash, Role: dump.Role, DisplayName: dump.DisplayName,
				Disabled: true, AuthVersion: 1, MustChangePassword: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if err := tx.Model(&existing).Updates(map[string]any{"username": dump.Username, "email": dump.Email, "role": dump.Role, "display_name": dump.DisplayName,
				"disabled": dump.Disabled, "auth_version": gorm.Expr("auth_version + 1"), "updated_at": time.Now().UTC()}).Error; err != nil {
				return err
			}
		}
		roles := dump.Roles
		if len(roles) == 0 {
			roles = []string{dump.Role}
		}
		if err := tx.Where("user_id = ?", dump.ID).Delete(&authz.UserRole{}).Error; err != nil {
			return err
		}
		for _, id := range roles {
			if err := tx.Create(&authz.UserRole{UserID: dump.ID, RoleID: id}).Error; err != nil {
				return err
			}
		}
	}
	for _, mapping := range bundle.GroupMappings {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&mapping).Error; err != nil {
			return err
		}
	}
	for i := range bundle.Skills {
		if err := upsert(tx, &bundle.Skills[i]); err != nil {
			return err
		}
	}
	for i := range bundle.IVRFlows {
		if err := upsert(tx, &bundle.IVRFlows[i]); err != nil {
			return err
		}
	}
	for i := range bundle.Agents {
		if err := upsert(tx, &bundle.Agents[i]); err != nil {
			return err
		}
	}
	// 队列自引用需要两阶段写入：先清空引用创建，再恢复目标关系。
	for i := range bundle.Queues {
		row := bundle.Queues[i]
		overflow, flow := row.OverflowQueueID, row.IVRFlowID
		row.OverflowQueueID, row.IVRFlowID = nil, nil
		if err := upsert(tx, &row); err != nil {
			return err
		}
		bundle.Queues[i].OverflowQueueID, bundle.Queues[i].IVRFlowID = overflow, flow
	}
	for i := range bundle.Queues {
		if err := tx.Model(&models.Queue{}).Where("id = ?", bundle.Queues[i].ID).Updates(map[string]any{
			"overflow_queue_id": bundle.Queues[i].OverflowQueueID, "ivr_flow_id": bundle.Queues[i].IVRFlowID}).Error; err != nil {
			return err
		}
	}
	for i := range bundle.IVRVersions {
		if err := upsert(tx, &bundle.IVRVersions[i]); err != nil {
			return err
		}
	}
	if options.Mode == "replace-bindings" {
		for _, model := range []any{&models.AgentSkill{}, &models.QueueSkill{}, &models.QueueAgent{}} {
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(model).Error; err != nil {
				return err
			}
		}
	}
	for i := range bundle.AgentSkills {
		if err := upsert(tx, &bundle.AgentSkills[i]); err != nil {
			return err
		}
	}
	for i := range bundle.QueueSkills {
		if err := upsert(tx, &bundle.QueueSkills[i]); err != nil {
			return err
		}
	}
	for i := range bundle.QueueAgents {
		if err := upsert(tx, &bundle.QueueAgents[i]); err != nil {
			return err
		}
	}
	for i := range bundle.DIDs {
		if err := upsert(tx, &bundle.DIDs[i]); err != nil {
			return err
		}
	}
	for i := range bundle.Webhooks {
		if err := upsert(tx, &bundle.Webhooks[i]); err != nil {
			return err
		}
	}
	return nil
}

func upsert(tx *gorm.DB, value any) error {
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(value).Error
}

func errorsIsNotFound(err error) bool { return err == gorm.ErrRecordNotFound }
