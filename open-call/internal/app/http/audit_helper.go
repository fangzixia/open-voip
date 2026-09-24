package http

import (
	"context"
	"log/slog"
)

func (d RouterDeps) writeAudit(ctx context.Context, userID, action, resource string, detail any) {
	if d.Audit == nil {
		return
	}
	if err := d.Audit.Write(ctx, userID, action, resource, detail); err != nil {
		slog.ErrorContext(ctx, "审计日志写入失败", "action", action, "resource", resource, "err", err)
	}
}
