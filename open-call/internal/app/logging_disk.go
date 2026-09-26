// 本文件负责磁盘日志文件配置。
package app

import (
	"context"
	"log/slog"
	"time"
)

const diskWarningBytes = uint64(1 << 30)

func monitorLogDisk(ctx context.Context, log *slog.Logger, path string) {
	check := func() {
		free, total, err := diskSpace(path)
		if err != nil {
			log.Warn("日志磁盘空间检查失败", "path", path, "err", err)
			return
		}
		if free < diskWarningBytes || total > 0 && free*100/total < 10 {
			log.Warn("日志磁盘可用空间不足", "path", path, "free_bytes", free, "total_bytes", total)
		}
	}
	check()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
