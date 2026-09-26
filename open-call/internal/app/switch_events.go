package app

import (
	"context"
	"log/slog"
	"open-call/internal/integration/switchapi"
	"open-call/internal/ports"
	"time"

	"gorm.io/gorm"
)

// consumeSwitchEvents replays the Switch's durable event stream into the
// business WebSocket/webhook hub. The cursor advances only after delivery.
func consumeSwitchEvents(ctx context.Context, db *gorm.DB, client *switchapi.Client, sink ports.CallEventPublisher, log *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := consumeSwitchBatch(ctx, db, client, sink); err != nil && ctx.Err() == nil {
			log.Warn("Switch 事件待重试", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func consumeSwitchBatch(ctx context.Context, db *gorm.DB, client *switchapi.Client, sink ports.CallEventPublisher) error {
	var cursor int64
	if err := db.WithContext(ctx).Raw("SELECT last_event_id FROM oc_switch_event_cursor WHERE id = 1").Scan(&cursor).Error; err != nil {
		return err
	}
	events, err := client.ListEvents(ctx, cursor)
	if err != nil {
		return err
	}
	for _, ev := range events {
		callID := ev.CallID
		if ev.TargetOnly {
			callID = ""
		}
		payload := make(map[string]any, len(ev.Payload)+2)
		for key, value := range ev.Payload {
			payload[key] = value
		}
		payload["switch_event_id"] = ev.ID
		payload["switch_call_seq"] = ev.Seq
		if err := sink.PublishCallEvent(ctx, ports.CallEvent{Type: ev.Type, CallID: callID, AgentID: ev.AgentID, Payload: payload}); err != nil {
			return err
		}
		if err := db.WithContext(ctx).Exec("UPDATE oc_switch_event_cursor SET last_event_id = GREATEST(last_event_id, ?), updated_at = NOW() WHERE id = 1", ev.ID).Error; err != nil {
			return err
		}
	}
	return nil
}
