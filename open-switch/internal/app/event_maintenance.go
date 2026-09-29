package app

import (
	"context"
	"log/slog"
	"time"

	"open-switch/internal/store"
)

// runEventRetention 按已登记 integrator 的 event_retention_days 定期清理过期呼叫事件。
func runEventRetention(ctx context.Context, events store.CallEvents, registry *store.ApplicationRegistry, log *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		purgeExpiredEvents(ctx, events, registry, log)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func purgeExpiredEvents(ctx context.Context, events store.CallEvents, registry *store.ApplicationRegistry, log *slog.Logger) {
	now := time.Now().UTC()
	for _, app := range registry.List() {
		days := app.EventRetentionDays
		if days < 1 {
			days = 14
		}
		cutoff := now.AddDate(0, 0, -days)
		n, err := events.PurgeBefore(ctx, app.ID, cutoff)
		if err != nil {
			log.Warn("事件保留清理失败", "application_id", app.ID, "err", err)
			continue
		}
		if n > 0 {
			log.Info("已清理过期 Switch 事件", "application_id", app.ID, "deleted", n, "before", cutoff.Format(time.RFC3339))
		}
	}
}
