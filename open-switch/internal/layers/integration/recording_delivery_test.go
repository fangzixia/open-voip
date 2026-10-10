package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"open-switch/internal/layers/cccore"
	"open-switch/internal/ports"
	"open-switch/internal/store"
	"open-switch/internal/store/migrate"
)

func TestRecordingMetadataEventAndDeliveryAreOneTransaction(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rollback := errors.New("test schema rollback")
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		schema := "delivery_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if e := tx.Exec("CREATE SCHEMA " + schema).Error; e != nil {
			return e
		}
		if e := tx.Exec("SET LOCAL search_path TO " + schema).Error; e != nil {
			return e
		}
		if e := migrate.Migrate(tx); e != nil {
			return e
		}
		id, now := uuid.NewString(), time.Now().UTC()
		if e := tx.Exec("INSERT INTO os_calls(id,direction,session_type,state,created_at,updated_at) VALUES (?,'inbound','audio','active',?,?)", id, now, now).Error; e != nil {
			return e
		}
		// DB intentionally points to the pool, not tx. Stage must use the
		// supplied transaction, and Notify must never attempt a second insert.
		d := &Dispatcher{DB: db, CallbackURL: "http://127.0.0.1/unused"}
		publisher := store.CallEvents{DB: tx, BeforeCommit: d.Stage, AfterAppend: d.Notify}
		facts := cccore.New(tx, publisher, cccore.Options{})
		meta := ports.RecordingMeta{ID: uuid.NewString(), CallID: id, FilePath: "partial.wav", MediaType: "audio", StartedAt: now, RecordingSemantics: "conversation_mono_v1", SampleRateHz: 8000, Channels: 1, DurationSamples: 160, Status: "failed", FailureReason: "disk full"}
		if e := facts.Save(ctx, meta); e != nil {
			return e
		}
		var count int64
		if e := tx.Raw("SELECT count(*) FROM os_integrator_event_deliveries d JOIN os_call_events e ON d.event_id=e.id WHERE e.call_id=? AND e.type='recording.saved' AND d.status='pending'", id).Scan(&count).Error; e != nil {
			return e
		}
		if count != 1 {
			t.Fatal("saved metadata was not durably staged for delivery")
		}
		failure := errors.New("injected outbox failure")
		broken := store.CallEvents{DB: tx, BeforeCommit: func(ctx context.Context, inner *gorm.DB, eventID int64) error {
			if e := d.Stage(ctx, inner, eventID); e != nil {
				return e
			}
			return failure
		}}
		event := ports.CallEvent{CallID: id, Type: "recording.failed"}
		if e := broken.Append(ctx, &event); !errors.Is(e, failure) || event.ID != 0 {
			t.Fatal("outbox failure was hidden", e)
		}
		if e := tx.Raw("SELECT count(*) FROM os_call_events WHERE call_id=?", id).Scan(&count).Error; e != nil {
			return e
		}
		if count != 1 {
			t.Fatal("event committed without its delivery")
		}
		if e := tx.Raw("SELECT count(*) FROM os_integrator_event_deliveries").Scan(&count).Error; e != nil {
			return e
		}
		if count != 1 {
			t.Fatal("failed event left an orphan delivery")
		}
		var seq int64
		if e := tx.Raw("SELECT event_seq FROM os_calls WHERE id=?", id).Scan(&seq).Error; e != nil {
			return e
		}
		if seq != 1 {
			t.Fatal("rolled-back outbox consumed a call sequence")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
