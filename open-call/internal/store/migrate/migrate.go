package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// 每个服务独立记录迁移版本，即使共用同一个 PostgreSQL schema。
// 两个服务的 SQL 版本号可能重复，共用版本表会导致迁移被跳过。
const migrationsTable = "oc_schema_migrations"

// migrationLockID 对使用同一数据库的进程首次启动与升级进行串行化，
// 避免两个进程同时创建迁移版本表时产生 PostgreSQL 并发冲突。

const migrationLockID int64 = 0x6f635f6d696772 // "oc_migr"

// ensureMigrationsTableSQL 启动时确保业务服务的版本表存在。
const ensureMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS oc_schema_migrations (
  version BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  checksum TEXT NOT NULL DEFAULT '',
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE oc_schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT NOT NULL DEFAULT '';
ALTER TABLE oc_schema_migrations ADD COLUMN IF NOT EXISTS execution_patch TEXT NOT NULL DEFAULT '';
ALTER TABLE oc_schema_migrations ADD COLUMN IF NOT EXISTS execution_checksum TEXT NOT NULL DEFAULT '';
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
		raw, err := sqlFiles.ReadFile(path.Join("sql", name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		sum := checksumSQL(string(raw))
		skip, err := migrationApplied(db, version, name, sum)
		if err != nil {
			return err
		}
		if skip {
			continue
		}
		if err := applyFile(db, version, name, string(raw), sum); err != nil {
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

func checksumSQL(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// migrationApplied 已执行且校验和一致则跳过；已执行但文件被改写则拒绝启动。
func migrationApplied(db *gorm.DB, version int64, name, sum string) (bool, error) {
	var n int64
	if err := db.Raw("SELECT COUNT(1) FROM oc_schema_migrations WHERE version = ?", version).Scan(&n).Error; err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	var row struct {
		Name     string
		Checksum string
	}
	if err := db.Raw("SELECT name, checksum FROM oc_schema_migrations WHERE version = ?", version).Scan(&row).Error; err != nil {
		return false, err
	}
	if row.Checksum == "" {
		return true, nil
	}
	if row.Name != name || row.Checksum != sum {
		return false, fmt.Errorf("迁移 %d (%s) 已应用，但嵌入文件校验和不一致；请追加新迁移，勿修改已发布 SQL", version, name)
	}
	return true, nil
}

// applyFile 在单事务内执行 SQL 文件并记录版本。
func applyFile(db *gorm.DB, version int64, name, content, sum string) error {
	executed, patch, e := executionSQL(name, content)
	if e != nil {
		return fmt.Errorf("%s: %w", name, e)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range splitSQL(executed) {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		if err := tx.Exec(
			"INSERT INTO oc_schema_migrations (version, name, checksum, execution_patch, execution_checksum) VALUES (?, ?, ?, ?, ?)",
			version, name, sum, patch, checksumSQL(executed),
		).Error; err != nil {
			return err
		}
		return nil
	})
}

// splitSQL 按分号切分语句，忽略引号内分号及 -- 行注释。
func splitSQL(content string) []string {
	// Execution strips a UTF-8 BOM; the historical checksum remains unchanged.
	content = strings.TrimPrefix(content, "\ufeff")
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
