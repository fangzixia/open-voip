package app

import (
	"context"
	"encoding/json"
	"gorm.io/gorm/clause"
	"log/slog"
	"open-call/internal/datetime"
	"open-call/internal/integration/switchapi"
	"open-call/internal/layers/biz/cdr"
	"open-call/internal/layers/biz/recmeta"
	"open-call/internal/ports"
	"open-call/internal/store/models"
	"time"

	"gorm.io/gorm"
)

// consumeSwitchEvents replays the Switch's durable event stream into the
// business WebSocket/webhook hub. The cursor advances with the durable projection and delivery intent.
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
	if err := deliverSwitchEvents(ctx, db, sink); err != nil {
		return err
	}
	var cursor int64
	if err := db.WithContext(ctx).Raw("SELECT last_event_id FROM oc_switch_event_cursor WHERE id=1").Scan(&cursor).Error; err != nil {
		return err
	}
	events, err := client.ListEvents(ctx, cursor)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if err := commitSwitchEvent(ctx, db, ev); err != nil {
			return err
		}
	}
	return deliverSwitchEvents(ctx, db, sink)
}

// commitSwitchEvent commits the inbox, projection, delivery intent and cursor together.
func commitSwitchEvent(ctx context.Context, db *gorm.DB, ev switchapi.Event) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cursor int64
		if err := tx.Raw("SELECT last_event_id FROM oc_switch_event_cursor WHERE id=1 FOR UPDATE").Scan(&cursor).Error; err != nil {
			return err
		}
		if ev.ID <= cursor {
			return nil
		}
		if err := tx.Create(&models.SwitchEventInbox{EventID: ev.ID, ApplicationID: ev.ApplicationID, ReceivedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
		if err := projectSwitchEvent(ctx, tx, ev); err != nil {
			return err
		}
		callID := ev.CallID
		if ev.TargetOnly {
			callID = ""
		}
		payload := make(map[string]any, len(ev.Payload)+4)
		for k, v := range ev.Payload {
			payload[k] = v
		}
		payload["switch_event_id"] = ev.ID
		payload["switch_call_seq"] = ev.Seq
		payload["application_id"] = ev.ApplicationID
		payload["version"] = ev.Version
		raw, err := json.Marshal(ports.CallEvent{Type: ev.Type, CallID: callID, AgentID: ev.AgentID, Payload: payload})
		if err != nil {
			return err
		}
		if err := tx.Create(&models.SwitchEventOutbox{EventID: ev.ID, Payload: string(raw)}).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE oc_switch_event_cursor SET last_event_id=?,updated_at=NOW() WHERE id=1", ev.ID).Error
	})
}

// Delivery is at least once: a crash after delivery can replay the stable event ID.
func deliverSwitchEvents(ctx context.Context, db *gorm.DB, sink ports.CallEventPublisher) error {
	var rows []models.SwitchEventOutbox
	if err := db.WithContext(ctx).Where("delivered_at IS NULL").Order("event_id").Limit(100).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var ev ports.CallEvent
		if err := json.Unmarshal([]byte(row.Payload), &ev); err != nil {
			return err
		}
		if err := sink.PublishCallEvent(ctx, ev); err != nil {
			return err
		}
		if err := db.WithContext(ctx).Model(&models.SwitchEventOutbox{}).Where("event_id=?", row.EventID).Update("delivered_at", time.Now().UTC()).Error; err != nil {
			return err
		}
	}
	return nil
}

// projectSwitchEvent is idempotent; replay after a delivery failure cannot duplicate history.
func projectSwitchEvent(ctx context.Context, db *gorm.DB, ev switchapi.Event) error {
	raw, err := json.Marshal(ev.Payload)
	if err != nil {
		return err
	}
	switch ev.Type {
	case "cdr.updated":
		var req ports.CDRWriteRequest
		if err := datetime.UnmarshalCurrent(raw, &req); err != nil {
			return err
		}
		return cdr.NewRecorderService(db).Upsert(ctx, req)
	case "recording.saved":
		var rec ports.RecordingMeta
		if err := datetime.UnmarshalCurrent(raw, &rec); err != nil {
			return err
		}
		return recmeta.NewService(db).Save(ctx, rec)
	case "agent.routing_state_changed":
		var payload struct {
			State  string
			Reason string
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
		at, err := datetime.Parse(ev.CreatedAt)
		if err != nil {
			return err
		}
		row := models.AgentStateProjection{ID: ev.ID, AgentID: ev.AgentID, ToState: payload.State, Reason: payload.Reason, CreatedAt: at}
		return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	}
	return nil
}
