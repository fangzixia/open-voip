// 本文件验证migrate的关键行为。
package store

import (
	"os"
	"testing"

	"gorm.io/gorm/logger"

	"open-call/internal/store/migrate"
)

func TestMigrateIntegration(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 OPEN_VOIP_TEST_DSN，跳过集成迁移测试")
	}
	db, err := Open(dsn, logger.Warn)
	if err != nil {
		t.Fatal("已配置测试数据库但连接失败: ", err)
	}
	if err := migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Migrate(db); err != nil {
		t.Fatal("重复 Migrate 应幂等", err)
	}
}
