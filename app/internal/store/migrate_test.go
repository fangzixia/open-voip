package store

import (
	"os"
	"testing"

	"gorm.io/gorm/logger"
)

func TestAutoMigrateIntegration(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 OPEN_VOIP_TEST_DSN，跳过集成迁移测试")
	}
	db, err := Open(dsn, logger.Warn)
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal("重复 AutoMigrate 应幂等", err)
	}
}
