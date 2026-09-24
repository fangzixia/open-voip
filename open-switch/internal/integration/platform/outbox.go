package platform

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"log/slog"
	"net/http"
	"time"
)

type delivery struct {
	ID        int64
	Path      string
	Payload   string
	Attempts  int
	LastError string
	CreatedAt time.Time
}

func (delivery) TableName() string { return "os_platform_outbox" }

// EnableOutbox 须在对外提供服务前调用。控制面命令仍同步执行；
// 最终元数据与坐席释放仅在本地 outbox 持久化写入后才视为已确认。
func (c *Client) EnableOutbox(db *gorm.DB) { c.outbox = db }

func (c *Client) enqueue(ctx context.Context, path string, body any) error {
	if c.outbox == nil {
		return c.doJSON(ctx, http.MethodPost, path, body, nil)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return c.outbox.WithContext(ctx).Create(&delivery{Path: path, Payload: string(raw), CreatedAt: time.Now().UTC()}).Error
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

func (c *Client) deliverPending(ctx context.Context) error {
	if c.outbox == nil {
		return nil
	}
	var rows []delivery
	if err := c.outbox.WithContext(ctx).Order("id").Limit(100).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		err := c.doJSON(ctx, http.MethodPost, row.Path, json.RawMessage(row.Payload), nil)
		if err != nil {
			_ = c.outbox.WithContext(ctx).Model(&row).Updates(map[string]any{"attempts": row.Attempts + 1, "last_error": err.Error()}).Error
			return err // preserve ordering: an older CDR can never overwrite a final CDR.
		}
		if err := c.outbox.WithContext(ctx).Delete(&row).Error; err != nil {
			return err
		}
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
