package cccore

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"open-switch/internal/datetime"
	"open-switch/internal/ports"
	"open-switch/internal/store"
	"open-switch/internal/store/models"
	"os"
	"strings"
	"testing"
	"time"
)

func TestConversationRecordingMigrationAndSavedEvent(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	db, e := store.Open(dsn, logger.Silent)
	if e != nil {
		t.Fatal(e)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	rollback := errors.New("test schema rollback")
	e = db.Transaction(func(tx *gorm.DB) error {
		schema := "quality_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if e := tx.Exec("CREATE SCHEMA " + schema).Error; e != nil {
			return e
		}
		if e := tx.Exec("SET LOCAL search_path TO " + schema).Error; e != nil {
			return e
		}
		raw, e := os.ReadFile("../../store/migrate/sql/000001_baseline.sql")
		if e != nil {
			return e
		}
		// Only schema DDL is needed for this incremental migration gate; fresh
		// installation of the historical baseline is a separate integration test.
		text := strings.TrimPrefix(string(raw), "\ufeff")
		end := strings.Index(text, "COMMENT ON ")
		if end < 0 {
			return errors.New("baseline boundary missing")
		}
		if e = tx.Exec(text[:end]).Error; e != nil {
			return e
		}
		fixes, e := os.ReadFile("../../store/migrate/sql/000002_schema_fixes.sql")
		if e != nil {
			return e
		}
		if e = tx.Exec(string(fixes)).Error; e != nil {
			return e
		}
		if e = tx.Exec("ALTER TABLE os_calls ADD COLUMN event_seq BIGINT NOT NULL DEFAULT 0").Error; e != nil {
			return e
		}
		// Existing deployed schemas contain this later application column;
		// baseline completeness is independently covered by full migration tests.
		if e = tx.Exec("ALTER TABLE os_queues ADD COLUMN post_call_ivr_flow_id UUID").Error; e != nil {
			return e
		}
		for _, name := range []string{"000005_queue_audio_profile.sql", "000006_conversation_recording.sql"} {
			sql, e := os.ReadFile("../../store/migrate/sql/" + name)
			if e != nil {
				return e
			}
			if e = tx.Exec(string(sql)).Error; e != nil {
				return e
			}
		}
		now := time.Now().UTC().Truncate(time.Second)
		legacyID, callID := uuid.NewString(), uuid.NewString()
		if e = tx.Exec("INSERT INTO os_calls(id,direction,session_type,state,created_at,updated_at) VALUES (?,'inbound','audio','active',?,?)", callID, now, now).Error; e != nil {
			return e
		}
		// Legacy insert uses metadata defaults without changing its existing file.
		if e = tx.Exec("INSERT INTO os_recordings(id,call_id,file_path,media_type,started_at,created_at) VALUES (?,?,?,'audio',?,?)", legacyID, callID, "old.wav", now, now).Error; e != nil {
			return e
		}
		var legacy models.Recording
		if e = tx.First(&legacy, "id = ?", legacyID).Error; e != nil {
			return e
		}
		if legacy.RecordingSemantics != "legacy" || legacy.FilePath != "old.wav" {
			t.Fatal("historical recording changed")
		}
		svc := New(tx, store.CallEvents{DB: tx}, Options{})
		meta := ports.RecordingMeta{ID: uuid.NewString(), CallID: callID, FilePath: "partial.wav", MediaType: "audio", StartedAt: now, RecordingSemantics: "conversation_mono_v1", Channels: 1, SampleRateHz: 8000, DurationSamples: 640, Status: "failed", FailureReason: "queue full", LegPaths: map[string]string{"customer": "leg.wav"}}
		if e = svc.Save(context.Background(), meta); e != nil {
			return e
		}
		if e = svc.Save(context.Background(), meta); e != nil {
			return e
		}
		var row models.Recording
		if e = tx.First(&row, "id = ?", meta.ID).Error; e != nil {
			return e
		}
		if row.Status != "failed" || row.DurationSamples != 640 || row.LegPaths["customer"] != "leg.wav" {
			t.Fatalf("recording metadata lost: %+v", row)
		}
		stale := meta
		stale.DurationSamples, stale.LegPaths = 160, nil
		stale.FailureReason = "stale failure"
		if e = svc.Save(context.Background(), stale); e != nil {
			return e
		}
		if e = tx.First(&row, "id = ?", meta.ID).Error; e != nil {
			return e
		}
		if row.DurationSamples != 640 || row.LegPaths["customer"] != "leg.wav" || row.FailureReason != "queue full" {
			t.Fatal("stale failure snapshot erased partial recording metadata")
		}
		var event store.CallEventRow
		if e = tx.Where("type = ?", "recording.saved").Last(&event).Error; e != nil {
			return e
		}
		var payload ports.RecordingMeta
		if e = datetime.Unmarshal(event.Payload, &payload); e != nil {
			return e
		}
		if payload.Status != "failed" || payload.RecordingSemantics != "conversation_mono_v1" || payload.DurationSamples != 640 || payload.FailureReason != "queue full" || payload.LegPaths["customer"] != "leg.wav" {
			t.Fatal("saved event lost semantics")
		}
		// Emulate a pre-upgrade active wideband revision without passing the
		// new publisher validation. Migration must create immutable history.
		bundle := ports.ConfigBundle{Version: 1, Queues: []ports.QueueConfig{{ID: uuid.NewString(), Name: "legacy", AudioProfile: ports.AudioProfileWideband}}}
		normalizeBundle(&bundle)
		rawBundle, e := json.Marshal(bundle)
		if e != nil {
			return e
		}
		old := models.ConfigVersion{Version: 1, Status: "active", Checksum: "historical-checksum", Payload: string(rawBundle), CreatedAt: now}
		if e = tx.Create(&old).Error; e != nil {
			return e
		}
		if e = tx.Create(&models.ActiveConfig{ID: 1, Version: 1, ActivatedAt: now}).Error; e != nil {
			return e
		}
		if e = svc.MigrateNarrowbandConfiguration(context.Background()); e != nil {
			return e
		}
		var active models.ActiveConfig
		if e = tx.First(&active, "id = ?", 1).Error; e != nil {
			return e
		}
		if active.Version != 2 {
			t.Fatalf("no immutable migration revision: %d", active.Version)
		}
		var historical, current models.ConfigVersion
		if e = tx.First(&historical, "version = ?", 1).Error; e != nil {
			return e
		}
		if historical.Payload != string(rawBundle) || historical.Checksum != old.Checksum {
			t.Fatal("historical payload/checksum rewritten")
		}
		if e = tx.First(&current, "version = ?", 2).Error; e != nil {
			return e
		}
		var migrated ports.ConfigBundle
		if e = json.Unmarshal([]byte(current.Payload), &migrated); e != nil {
			return e
		}
		if migrated.Queues[0].AudioProfile != ports.AudioProfileNarrowband {
			t.Fatal("migration retained unsupported profile")
		}
		if e = svc.MigrateNarrowbandConfiguration(context.Background()); e != nil {
			return e
		}
		var count int64
		if e = tx.Model(&models.ConfigVersion{}).Count(&count).Error; e != nil {
			return e
		}
		if count != 2 {
			t.Fatal("repeated startup published another revision")
		}
		if _, e = svc.ActivateConfig(context.Background(), 1); e == nil {
			t.Fatal("reactivated unverified wideband revision")
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal(e)
	}
}
