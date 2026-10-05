package ws

import (
	"context"
	"os"
	"testing"

	"gorm.io/gorm/logger"

	"open-call/internal/store"
	"open-call/internal/store/migrate"
)

func TestAllocateSeqPostgres(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("OPEN_VOIP_TEST_DSN 未设置")
	}
	db, err := store.Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	h := NewHub(nil)
	h.ConfigureDB(db)
	a := h.allocateSeq(context.Background())
	b := h.allocateSeq(context.Background())
	if b <= a {
		t.Fatalf("expected monotonic seq, got %d then %d", a, b)
	}
}
