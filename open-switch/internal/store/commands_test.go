package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/logger"
	"open-switch/internal/store/migrate"
)

func TestCommandIdempotency(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("requires OPEN_VOIP_TEST_DSN")
	}
	db, err := Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	cmds := Commands{DB: db, Events: CallEvents{DB: db}}
	callID := uuid.NewString()
	if e := db.Exec("INSERT INTO os_calls(id,direction,session_type,state,created_at,updated_at) VALUES (?,'inbound','audio','active',?,?)", callID, time.Now(), time.Now()).Error; e != nil {
		t.Fatal(e)
	}
	defer db.Exec("DELETE FROM os_calls WHERE id = ?", callID)
	defer db.Exec("DELETE FROM os_call_events WHERE call_id = ?", callID)
	defer db.Exec("DELETE FROM os_commands WHERE call_id = ?", callID)
	ctx := context.Background()
	hash := "abc123"
	first, reused, err := cmds.Accept(ctx, callID, "idem-"+callID, hash, "sip.dial", map[string]any{"leg_id": "leg-1"})
	if err != nil || reused {
		t.Fatalf("first accept: reused=%v err=%v", reused, err)
	}
	second, reused, err := cmds.Accept(ctx, callID, "idem-"+callID, hash, "sip.dial", map[string]any{"leg_id": "leg-1"})
	if err != nil || !reused || second.ID != first.ID {
		t.Fatalf("replay: %+v reused=%v err=%v", second, reused, err)
	}
	if err := cmds.MarkRunning(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := cmds.Complete(ctx, first.ID, "succeeded", "", map[string]any{"leg_id": "leg-1"}); err != nil {
		t.Fatal(err)
	}
	got, err := cmds.Get(ctx, first.ID)
	if err != nil || got.Status != "succeeded" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	_ = cmds.ReconcileStale(ctx, time.Nanosecond)
}
