// 本文件验证boundary integration的关键行为。
package queue_test

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/gorm/logger"
	"open-call/internal/layers/biz/agent"
	"open-call/internal/layers/biz/queue"
	"open-call/internal/ports/dto"
	"open-call/internal/store"
	"open-call/internal/store/migrate"
	"open-call/internal/store/models"
	"os"
	"testing"
	"time"
)

func TestACDAllSkillsIdempotencyAndStaleRelease(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL OPEN_VOIP_TEST_DSN")
	}
	db, err := store.Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	if err = migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	qid, s1, s2 := uuid.NewString(), uuid.NewString(), uuid.NewString()
	rows := []any{&models.Queue{ID: qid, Name: "test", DispatchStrategy: "longest_idle"}, &models.Skill{ID: s1, Name: s1}, &models.Skill{ID: s2, Name: s2}, &models.QueueSkill{QueueID: qid, SkillID: s1}, &models.QueueSkill{QueueID: qid, SkillID: s2}}
	full := ""
	for i := 0; i < 2; i++ {
		uid, aid := uuid.NewString(), uuid.NewString()
		rows = append(rows, &models.User{ID: uid, Username: uid, PasswordHash: "test", Role: "agent"}, &models.Agent{ID: aid, UserID: uid, Extension: aid[:12]}, &models.AgentSession{ID: uuid.NewString(), AgentID: aid, State: "idle", UpdatedAt: time.Now().Add(time.Duration(i) * time.Second)}, &models.AgentSessionQueue{AgentID: aid, QueueID: qid}, &models.AgentSkill{AgentID: aid, SkillID: s1})
		if i == 1 {
			full = aid
			rows = append(rows, &models.AgentSkill{AgentID: aid, SkillID: s2})
		}
	}
	for _, row := range rows {
		if err := tx.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := queue.NewService(tx, nil, queue.PolicyDefaults{})
	request := dto.DispatchRequest{CallID: uuid.NewString(), QueueID: qid}
	for i := 0; i < 2; i++ {
		got, err := svc.RequestAgent(context.Background(), request)
		if err != nil || got.AgentID != full {
			t.Fatalf("dispatch %d: %+v %v", i, got, err)
		}
	}
	agents := agent.NewService(tx, nil)
	if err := agents.SetCallState(context.Background(), "old-call", full, "ringing", "idle", "late-release"); err != nil {
		t.Fatal(err)
	}
	var session models.AgentSession
	if err := tx.Where("agent_id = ?", full).First(&session).Error; err != nil {
		t.Fatal(err)
	}
	if session.State != "ringing" || session.CurrentCallID != request.CallID {
		t.Fatalf("stale callback released new call: %+v", session)
	}
}
