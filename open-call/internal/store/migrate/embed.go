package migrate

import "embed"

// sqlFiles 嵌入 sql/ 目录下的版本化迁移脚本。
//
//go:embed sql/*.sql
var sqlFiles embed.FS
