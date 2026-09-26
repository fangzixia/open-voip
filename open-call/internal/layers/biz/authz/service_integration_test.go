// 本文件验证service integration的关键行为。
package authz_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"open-call/internal/layers/biz/authz"
	"open-call/internal/layers/biz/user"
	"open-call/internal/store"
	"open-call/internal/store/migrate"
	"open-call/internal/store/models"
)

func TestRolesAndIdentityIntegration(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("OPEN_VOIP_TEST_DSN not set")
	}
	db, err := store.Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("rollback test data")
	err = db.Transaction(func(tx *gorm.DB) error {
		ctx := context.Background()
		svc := authz.NewService(tx)
		uid := uuid.NewString()
		name := "user-" + uid[:8]
		u := models.User{ID: uid, Username: name, Email: "existing-" + uid[:8] + "@example.test", PasswordHash: "test", Role: "agent", AuthVersion: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		if err := tx.Create(&authz.UserRole{UserID: uid, RoleID: "agent"}).Error; err != nil {
			return err
		}
		perms, err := svc.Permissions(ctx, uid)
		if err != nil {
			return err
		}
		if !contains(perms, "calls.operate") || contains(perms, "users.read") {
			t.Fatalf("agent permissions: %v", perms)
		}
		roleID := "report-" + uid[:8]
		if err := svc.SaveRole(ctx, roleID, "报表查看", []string{"reports.read"}); err != nil {
			return err
		}
		created, err := user.NewService(tx).Create(ctx, user.CreateInput{Username: "custom-" + uid[:8], Email: "custom-" + uid[:8] + "@example.test", Password: "StrongPassword123!", Roles: []string{roleID}})
		if err != nil {
			return err
		}
		if created.AgentID != "" || len(created.Roles) != 1 || created.Roles[0] != roleID {
			t.Fatalf("custom role creation failed: %+v", created)
		}
		extension := "88" + uid[:6]
		created, err = user.NewService(tx).Update(ctx, created.ID, user.UpdateInput{Extension: &extension})
		if err != nil || created.AgentID == "" || created.Extension != extension {
			t.Fatalf("agent profile creation failed: %+v %v", created, err)
		}
		adminRole := "admin"
		created, err = user.NewService(tx).Update(ctx, created.ID, user.UpdateInput{Role: &adminRole})
		if err != nil || created.AgentID == "" {
			t.Fatalf("legacy role change removed agent profile: %+v %v", created, err)
		}
		sid := uuid.NewString()
		session := models.AuthSession{ID: sid, UserID: uid, RefreshJTI: uuid.NewString(), CreatedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour)}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		if err := svc.SetUserRoles(ctx, uid, []string{roleID}); err != nil {
			return err
		}
		perms, err = svc.Permissions(ctx, uid)
		if err != nil {
			return err
		}
		if len(perms) != 1 || perms[0] != "reports.read" {
			t.Fatalf("custom permissions: %v", perms)
		}
		if err := tx.First(&session, "id = ?", sid).Error; err != nil {
			return err
		}
		if session.RevokedAt == nil {
			t.Fatal("role change did not revoke session")
		}
		if err := svc.SetMapping(ctx, "team-report", []string{roleID}); err != nil {
			return err
		}
		ids, err := svc.RolesForGroups(ctx, []string{"team-report"})
		if err != nil {
			return err
		}
		if len(ids) != 1 || ids[0] != roleID {
			t.Fatalf("mapped roles: %v", ids)
		}
		if _, err := svc.NewOIDCUser(ctx, "https://id.test", "sub-1", name, "", "Existing", ids); err == nil {
			t.Fatal("same username must not auto-link")
		}
		if _, err := svc.NewOIDCUser(ctx, "https://id.test", "sub-email", "different-"+uid[:8], u.Email, "Existing", ids); err == nil {
			t.Fatal("same email must not auto-link")
		}
		bindingSession := models.AuthSession{ID: uuid.NewString(), UserID: uid, RefreshJTI: uuid.NewString(), CreatedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour)}
		if err := tx.Create(&bindingSession).Error; err != nil {
			return err
		}
		if err := svc.BindIdentity(ctx, uid, "https://id.test", "sub-1"); err != nil {
			return err
		}
		if err := tx.First(&bindingSession, "id = ?", bindingSession.ID).Error; err != nil || bindingSession.RevokedAt == nil {
			t.Fatalf("identity change did not revoke session: %v", err)
		}
		bound, err := svc.NewOIDCUser(ctx, "https://id.test", "sub-1", name, "", "Existing", ids)
		if err != nil || bound.ID != uid {
			t.Fatalf("binding failed: %v %s", err, bound.ID)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
