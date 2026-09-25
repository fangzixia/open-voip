package platform

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"log/slog"
	"net/http"
	"open-switch/internal/datetime"
	"open-switch/internal/observability"
	"time"
)

type delivery struct {
	ID        int64
	Path      string
	Payload   string
	Attempts  int
	LastError string
	TraceID   string
	RequestID string
	CreatedAt time.Time
}

func (delivery) TableName() string { return "os_platform_outbox" }

// EnableOutbox 须在对外提供服务前调用。控制面命令仍同步执行；
// 最终元数据与坐席释放仅在本地 outbox 持久化写入后才视为已确认。
func (c *Client) EnableOutbox(db *gorm.DB) { c.outbox = db }

// enqueue 持久化跨服务请求，允许目标服务暂时不可用时稍后重试。
func (c *Client) enqueue(ctx context.Context, path string, body any) error {
	if c.outbox == nil {
		return c.doJSON(ctx, http.MethodPost, path, body, nil)
	}
	raw, err := datetime.Marshal(body)
	if err != nil {
		return err
	}
	f := observability.FromContext(ctx)
	started := time.Now()
	err = c.outbox.WithContext(ctx).Create(&delivery{
		Path: path, Payload: string(raw), TraceID: f.TraceID, RequestID: f.RequestID, CreatedAt: time.Now().UTC(),
	}).Error
	result := "ok"
	if err != nil {
		result = "error"
	}
	observability.Event(ctx, "platform_outbox", "outbox.enqueue", "persist", result, "", started, "path", path)
	return err
}

func (c *Client) RunOutbox(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if err := c.deliverPending(ctx); err != nil {
			slog.Error("Platform 回调等待重试", "err", err)
		}
	}
}

// deliverPending 领取并投递待处理请求，按结果更新重试状态。
func (c *Client) deliverPending(ctx context.Context) error {
	if c.outbox == nil {
		return nil
	}
	var rows []delivery
	if err := c.outbox.WithContext(ctx).Order("id").Limit(100).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		rowCtx := observability.WithFields(ctx, observability.Fields{TraceID: row.TraceID, RequestID: row.RequestID})
		started := time.Now()
		err := c.doJSON(rowCtx, http.MethodPost, row.Path, json.RawMessage(row.Payload), nil)
		if err != nil {
			_ = c.outbox.WithContext(ctx).Model(&row).Updates(map[string]any{"attempts": row.Attempts + 1, "last_error": err.Error()}).Error
			observability.Event(rowCtx, "platform_outbox", "outbox.deliver", "retry", "error", "delivery_failed", started, "path", row.Path, "attempt", row.Attempts+1)
			return err // preserve ordering: an older CDR can never overwrite a final CDR.
		}
		if err := c.outbox.WithContext(ctx).Delete(&row).Error; err != nil {
			return err
		}
		observability.Event(rowCtx, "platform_outbox", "outbox.deliver", "complete", "ok", "", started, "path", row.Path, "attempt", row.Attempts+1)
	}
	return nil
}

func (c *Client) SetCallState(ctx context.Context, callID, agentID, from, to, reason string) error {
	body := map[string]string{"call_id": callID, "from_state": from, "to_state": to, "reason": reason}
	path := "/platform/v1/agents/" + agentID + "/state"
	if to != "ringing" && to != "on_call" {
		return c.enqueue(ctx, path, body)
	}
	return c.doJSON(ctx, http.MethodPost, path, body, nil)
}
