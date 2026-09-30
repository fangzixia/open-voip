package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"open-switch/internal/datetime"
	"open-switch/internal/store"
)

// Dispatcher 将持久化事件 POST 到配置的 events_callback_url。
type Dispatcher struct {
	DB          *gorm.DB
	CallbackURL string
	HTTP        *http.Client
	Log         *slog.Logger

	once sync.Once
}

func (d *Dispatcher) Start(ctx context.Context) {
	d.once.Do(func() {
		if d.HTTP == nil {
			d.HTTP = &http.Client{Timeout: 10 * time.Second}
		}
		if d.Log == nil {
			d.Log = slog.Default()
		}
		go d.retryLoop(ctx)
	})
}

func (d *Dispatcher) Enqueue(ctx context.Context, row store.CallEventRow) {
	if strings.TrimSpace(d.CallbackURL) == "" {
		return
	}
	_ = d.DB.WithContext(ctx).Exec(`
INSERT INTO os_integrator_event_deliveries (event_id, status, attempts, next_retry_at, last_error, updated_at)
VALUES (?, 'pending', 0, NOW(), '', NOW())
ON CONFLICT (event_id) DO NOTHING`, row.ID).Error
	go d.deliverOne(context.Background(), row)
}

func (d *Dispatcher) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		d.flushPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) flushPending(ctx context.Context) {
	var eventIDs []int64
	if err := d.DB.WithContext(ctx).Raw(`
SELECT event_id FROM os_integrator_event_deliveries
WHERE status = 'pending' AND next_retry_at <= NOW()
ORDER BY event_id LIMIT 50`).Scan(&eventIDs).Error; err != nil {
		d.Log.Warn("读取 integrator 投递队列失败", "err", err)
		return
	}
	for _, eventID := range eventIDs {
		var row store.CallEventRow
		if err := d.DB.WithContext(ctx).First(&row, "id = ?", eventID).Error; err != nil {
			continue
		}
		d.deliverOne(ctx, row)
	}
}

func (d *Dispatcher) deliverOne(ctx context.Context, row store.CallEventRow) {
	if strings.TrimSpace(d.CallbackURL) == "" {
		return
	}
	body, err := datetime.Marshal(row)
	if err != nil {
		d.markFailed(ctx, row.ID, err.Error(), true)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.CallbackURL, bytes.NewReader(body))
	if err != nil {
		d.markFailed(ctx, row.ID, err.Error(), true)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTP.Do(req)
	if err != nil {
		d.markFailed(ctx, row.ID, err.Error(), true)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		d.markFailed(ctx, row.ID, strings.TrimSpace(string(respBody)), true)
		return
	}
	var ack struct {
		Accepted bool  `json:"accepted"`
		EventID  int64 `json:"event_id"`
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(respBody, &envelope) == nil && len(envelope.Data) > 0 {
		_ = json.Unmarshal(envelope.Data, &ack)
	} else {
		_ = json.Unmarshal(respBody, &ack)
	}
	if !ack.Accepted || (ack.EventID != 0 && ack.EventID != row.ID) {
		d.markFailed(ctx, row.ID, "callback 未确认 accepted", true)
		return
	}
	_ = d.DB.WithContext(ctx).Exec(`
UPDATE os_integrator_event_deliveries SET status='success', updated_at=NOW(), last_error=''
WHERE event_id=?`, row.ID).Error
}

func (d *Dispatcher) markFailed(ctx context.Context, eventID int64, msg string, scheduleRetry bool) {
	var attempts int
	_ = d.DB.WithContext(ctx).Raw(`
SELECT attempts FROM os_integrator_event_deliveries WHERE event_id=?`,
		eventID).Scan(&attempts).Error
	attempts++
	next := time.Now().UTC()
	status := "pending"
	if !scheduleRetry || attempts >= 12 {
		status = "failed"
	} else {
		delay := time.Duration(1<<min(attempts, 6)) * time.Second
		next = next.Add(delay)
	}
	_ = d.DB.WithContext(ctx).Exec(`
UPDATE os_integrator_event_deliveries
SET status=?, attempts=?, next_retry_at=?, last_error=?, updated_at=NOW()
WHERE event_id=?`,
		status, attempts, next, truncateErr(msg), eventID).Error
}

func truncateErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
