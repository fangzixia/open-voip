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

// runSwitchOutboxDelivery 投递已投影的出站事件（Switch HTTP callback 为主路径）。
func runSwitchOutboxDelivery(ctx context.Context, db *gorm.DB, sink ports.CallEventPublisher, log *slog.Logger) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := DeliverSwitchEvents(ctx, db, sink); err != nil && ctx.Err() == nil {
			log.Warn("Switch 出站事件投递重试", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// CommitSwitchEvent 在同一事务中写入收件箱、业务投影、出站载荷并推进游标。
func CommitSwitchEvent(ctx context.Context, db *gorm.DB, ev switchapi.Event, _ ports.CallEventPublisher) error {
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
			// 仅面向坐席的事件不携带 call_id，避免客户侧 WebSocket 订阅收到。
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

// DeliverSwitchEvents 向 Hub 发布未投递的出站事件；至少投递一次，崩溃后可凭稳定 event_id 重放。
func DeliverSwitchEvents(ctx context.Context, db *gorm.DB, sink ports.CallEventPublisher) error {
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

// projectSwitchEvent 将技术类事件投影到业务库（话单、录音元数据、坐席状态历史）；幂等，投递失败后重放不会重复写入。
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
