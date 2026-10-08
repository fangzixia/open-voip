package app

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/gorm/logger"
	"open-call/internal/integration/switchapi"
	"open-call/internal/store"
	"open-call/internal/store/migrate"
	"open-call/internal/store/models"
)

func TestProjectCsatScored(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	db, err := store.Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	ev := switchapi.Event{
		ID:        42,
		Type:      "ivr.input_collected",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Payload:   map[string]any{"call_id": "00000000-0000-4000-8000-000000000001", "input": "4", "result_key": "csat", "agent_id": "00000000-0000-4000-8000-000000000002"},
	}
	if err := projectSwitchEvent(context.Background(), db, ev); err != nil {
		t.Fatal(err)
	}
	var row models.CallCsat
	if err := db.First(&row, "call_id = ?", "00000000-0000-4000-8000-000000000001").Error; err != nil {
		t.Fatal(err)
	}
	if row.Score != 4 {
		t.Fatalf("score=%d", row.Score)
	}
}
