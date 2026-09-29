package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"open-call/internal/datetime"
	"open-call/internal/integration/switchapi"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
	"open-call/internal/store"
	"open-call/internal/store/migrate"
	"open-call/internal/store/models"
)

func TestSwitchEventProjectionReplay(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	db, err := store.Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	started := time.Now().UTC().Truncate(time.Second)
	answered, ended := started.Add(5*time.Second), started.Add(35*time.Second)
	callID, recordingID, agentID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	payload := func(v any) map[string]any {
		t.Helper()
		raw, err := datetime.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err = json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	events := []switchapi.Event{
		{ID: time.Now().UnixNano(), Type: "agent.routing_state_changed", AgentID: agentID, CreatedAt: datetime.Format(started), Payload: map[string]any{"state": "idle", "reason": "check_in"}},
		{Type: "cdr.updated", CallID: callID, Payload: payload(ports.CDRWriteRequest{CallID: callID, Direction: "inbound", SessionType: dto.SessionTypeAudio, Result: "answered", StartedAt: started, AnsweredAt: &answered, EndedAt: &ended})},
		{Type: "recording.saved", CallID: callID, Payload: payload(ports.RecordingMeta{ID: recordingID, CallID: callID, FilePath: "test.wav", MediaType: "audio", StartedAt: answered, EndedAt: &ended, FileSize: 32000})},
	}
	for i := 0; i < 2; i++ {
		for _, ev := range events {
			if err := projectSwitchEvent(ctx, db, ev); err != nil {
				t.Fatalf("%s replay %d: %v", ev.Type, i, err)
			}
		}
	}
	var cdr models.CDR
	if err := db.First(&cdr, "call_id = ?", callID).Error; err != nil {
		t.Fatal(err)
	}
	if cdr.DurationSec != 30 || cdr.WaitSec != 5 || !cdr.StartedAt.Equal(started) {
		t.Fatalf("incorrect projection: %+v", cdr)
	}
	var count int64
	if err := db.Model(&models.AgentStateProjection{}).Where("agent_id = ?", agentID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("duplicate agent event: %d %v", count, err)
	}
	if err := db.Model(&models.Recording{}).Where("id = ?", recordingID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("duplicate recording: %d %v", count, err)
	}
}

type projectionSink struct {
	fail      bool
	delivered []ports.CallEvent
}

func (s *projectionSink) PublishCallEvent(_ context.Context, ev ports.CallEvent) error {
	if s.fail {
		return errors.New("delivery unavailable")
	}
	s.delivered = append(s.delivered, ev)
	return nil
}

func TestEventInboxProjectionCursorAndDelivery(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	db, err := store.Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("test transaction rollback")
	err = db.Transaction(func(tx *gorm.DB) error {
		ctx := context.Background()
		var cursor int64
		if err := tx.Raw("SELECT last_event_id FROM oc_switch_event_cursor WHERE id=1").Scan(&cursor).Error; err != nil {
			t.Fatal(err)
		}
		ev := switchapi.Event{ID: cursor + 1, ApplicationID: "projection-test", AgentID: uuid.NewString(), Type: "agent.routing_state_changed", CreatedAt: "invalid", Payload: map[string]any{"state": "idle"}}
		if err := commitSwitchEvent(ctx, tx, ev); err == nil {
			t.Fatal("invalid projection committed")
		}
		var after int64
		tx.Raw("SELECT last_event_id FROM oc_switch_event_cursor WHERE id=1").Scan(&after)
		if after != cursor {
			t.Fatal("cursor advanced after projection failure")
		}
		var count int64
		tx.Model(&models.SwitchEventInbox{}).Where("event_id=?", ev.ID).Count(&count)
		if count != 0 {
			t.Fatal("failed event left inbox record")
		}
		ev.CreatedAt = datetime.Format(time.Now())
		for i := 0; i < 2; i++ {
			if err := commitSwitchEvent(ctx, tx, ev); err != nil {
				t.Fatal(err)
			}
		}
		tx.Model(&models.AgentStateProjection{}).Where("id=?", ev.ID).Count(&count)
		if count != 1 {
			t.Fatal("projection duplicated")
		}
		sink := &projectionSink{fail: true}
		if err := deliverSwitchEvents(ctx, tx, sink); err == nil {
			t.Fatal("delivery failure ignored")
		}
		var row models.SwitchEventOutbox
		if err := tx.First(&row, "event_id=?", ev.ID).Error; err != nil || row.DeliveredAt != nil {
			t.Fatal("failed delivery was acknowledged")
		}
		sink.fail = false
		if err := deliverSwitchEvents(ctx, tx, sink); err != nil {
			t.Fatal(err)
		}
		if err := tx.First(&row, "event_id=?", ev.ID).Error; err != nil || row.DeliveredAt == nil {
			t.Fatal("successful delivery not acknowledged")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
