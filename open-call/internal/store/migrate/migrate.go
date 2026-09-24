package migrate

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// Each service owns its migration ledger even when both share a PostgreSQL schema.
// Their SQL version numbers overlap, so a shared ledger would silently skip migrations.
const migrationsTable = "oc_schema_migrations"

// migrationLockID serializes first boot and upgrades across processes that use
// the same database. PostgreSQL can otherwise race while two processes both
// execute CREATE TABLE IF NOT EXISTS for the migration ledger.
const migrationLockID int64 = 0x6f635f6d696772 // "oc_migr"

// ensureMigrationsTableSQL 启动时确保业务服务的版本表存在。
const ensureMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS oc_schema_migrations (
  version BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

// Migrate 按 embed SQL 版本顺序执行未应用的迁移。
func Migrate(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", migrationLockID).Error; err != nil {
			return fmt.Errorf("lock %s: %w", migrationsTable, err)
		}
		return migrateLocked(tx)
	})
}

func migrateLocked(db *gorm.DB) error {
	if err := db.Exec(ensureMigrationsTableSQL).Error; err != nil {
		return fmt.Errorf("%s: %w", migrationsTable, err)
	}

	entries, err := sqlFiles.ReadDir("sql")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := parseVersion(name)
		if err != nil {
			return err
		}
		applied, err := isApplied(db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		raw, err := sqlFiles.ReadFile(path.Join("sql", name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := applyFile(db, version, name, string(raw)); err != nil {
			return err
		}
	}
	return nil
}

// parseVersion 从 NNNNNN_name.sql 文件名解析版本号。
func parseVersion(filename string) (int64, error) {
	base := strings.TrimSuffix(filename, ".sql")
	i := strings.Index(base, "_")
	if i <= 0 {
		return 0, fmt.Errorf("migration 文件名须为 NNNNNN_name.sql: %s", filename)
	}
	v, err := strconv.ParseInt(base[:i], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("migration 版本号无效: %s", filename)
	}
	return v, nil
}

// isApplied 查询指定版本是否已写入本服务的迁移记录。
func isApplied(db *gorm.DB, version int64) (bool, error) {
	var n int64
	err := db.Raw("SELECT COUNT(1) FROM oc_schema_migrations WHERE version = ?", version).Scan(&n).Error
	return n > 0, err
}

// applyFile 在单事务内执行 SQL 文件并记录版本。
func applyFile(db *gorm.DB, version int64, name, content string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range splitSQL(content) {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		if err := tx.Exec(
			"INSERT INTO oc_schema_migrations (version, name) VALUES (?, ?)",
			version, name,
		).Error; err != nil {
			return err
		}
		return nil
	})
}

// splitSQL 按分号切分语句，忽略引号内分号及 -- 行注释。
func splitSQL(content string) []string {
	var out []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if !inSingle && !inDouble && ch == '-' && i+1 < len(content) && content[i+1] == '-' {
			for i+1 < len(content) && content[i+1] != '\n' {
				i++
			}
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			b.WriteByte(ch)
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			b.WriteByte(ch)
			continue
		}
		if ch == ';' && !inSingle && !inDouble {
			if stmt := strings.TrimSpace(b.String()); stmt != "" {
				out = append(out, stmt)
			}
			b.Reset()
			continue
		}
		b.WriteByte(ch)
	}
	if stmt := strings.TrimSpace(b.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}
