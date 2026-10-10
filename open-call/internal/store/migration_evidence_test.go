package store

import (
	"errors"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"open-call/internal/store/migrate"
	"uuid"
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
		schema := "migration_" + strings.ReplaceAll(uuid.New().String(), "-", "")
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
		if e := tx.Table("oc_schema_migrations").Order("version").Find(&before).Error; e != nil {
			return e
		}
		if len(before) != 5 || before[0].Checksum != "a9bcf2d8ac2a3ec999246a981437076cb2184d40d8236e3ef45687a9ecac9f55" || before[0].ExecutionPatch != "baseline_comment_and_seed_quotes_v1" || len(before[0].ExecutionChecksum) != 64 || before[0].ExecutionChecksum == before[0].Checksum {
			t.Fatalf("migration evidence absent: %+v", before)
		}
		if e := migrate.Migrate(tx); e != nil {
			return e
		}
		if e := tx.Table("oc_schema_migrations").Order("version").Find(&after).Error; e != nil {
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
