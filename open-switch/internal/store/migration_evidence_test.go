package store

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"open-switch/internal/store/migrate"
)

func TestFreshMigrationKeepsSourceAndExecutionEvidence(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	db, err := Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	rollback := errors.New("test schema rollback")
	err = db.Transaction(func(tx *gorm.DB) error {
		schema := "migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if e := tx.Exec("CREATE SCHEMA " + schema).Error; e != nil {
			return e
		}
		if e := tx.Exec("SET LOCAL search_path TO " + schema).Error; e != nil {
			return e
		}
		if e := migrate.Migrate(tx); e != nil {
			return e
		}
		var before, after []struct {
			Version                                     int64
			Checksum, ExecutionPatch, ExecutionChecksum string
		}
		if e := tx.Table("os_schema_migrations").Order("version").Find(&before).Error; e != nil {
			return e
		}
		if len(before) != 7 || before[0].Checksum != "b966a58898a1bfcebad83a2e0445c9241cd6721768af2b212f7e20baef347ea4" || before[0].ExecutionPatch != "baseline_comment_quotes_v1" || len(before[0].ExecutionChecksum) != 64 || before[0].ExecutionChecksum == before[0].Checksum {
			t.Fatalf("migration evidence absent: %+v", before)
		}
		if before[3].ExecutionPatch != "uuid_empty_text_comparison_v1" {
			t.Fatal("UUID repair was not recorded")
		}
		if e := migrate.Migrate(tx); e != nil {
			return e
		}
		if e := tx.Table("os_schema_migrations").Order("version").Find(&after).Error; e != nil {
			return e
		}
		for i := range before {
			if before[i] != after[i] {
				t.Fatal("repeat migration rewrote installed history")
			}
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
