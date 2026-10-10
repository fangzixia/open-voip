package app

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"open-call/internal/datetime"
	"open-call/internal/integration/switchapi"
	"open-call/internal/layers/biz/recmeta"
	"open-call/internal/ports"
	"open-call/internal/store"
	"open-call/internal/store/models"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestConversationRecordingMigrationAndFailureProjection(t *testing.T) {
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
		schema := "quality_" + strings.ReplaceAll(uuid.New().String(), "-", "")
		if e := tx.Exec("CREATE SCHEMA " + schema).Error; e != nil {
			return e
		}
		if e := tx.Exec("SET LOCAL search_path TO " + schema).Error; e != nil {
			return e
		}
		raw, e := os.ReadFile("../store/migrate/sql/000001_baseline.sql")
		if e != nil {
			return e
		}
		text := string(raw)
		start := strings.Index(text, "CREATE TABLE oc_recordings (")
		end := strings.Index(text[start:], ");") + start + 2
		if e = tx.Exec(text[start:end]).Error; e != nil {
			return e
		}
		legacyID, callID := uuid.New().String(), uuid.New().String()
		started := time.Now().UTC().Truncate(time.Second)
		ended := started.Add(time.Second)
		if e = tx.Exec("INSERT INTO oc_recordings(id,call_id,file_path,media_type,started_at,ended_at) VALUES (?,?,?,'audio',?,?)", legacyID, callID, "historical.wav", started, ended).Error; e != nil {
			return e
		}
		migration, e := os.ReadFile("../store/migrate/sql/000005_conversation_recording.sql")
		if e != nil {
			return e
		}
		if e = tx.Exec(string(migration)).Error; e != nil {
			return e
		}
		var old models.Recording
		if e = tx.First(&old, "id = ?", legacyID).Error; e != nil {
			return e
		}
		if old.RecordingSemantics != "legacy" || old.FilePath != "historical.wav" {
			t.Fatal("migration rewrote historical content")
		}
		meta := ports.RecordingMeta{ID: uuid.New().String(), CallID: callID, FilePath: "partial.wav", MediaType: "audio", StartedAt: started, RecordingSemantics: "conversation_mono_v1", Channels: 1, SampleRateHz: 8000, DurationSamples: 320, Status: "failed", FailureReason: "disk full", LegPaths: map[string]string{"leg": "aligned.wav"}}
		event := func(kind string, m ports.RecordingMeta) error {
			raw, e := datetime.Marshal(m)
			if e != nil {
				return e
			}
			var payload map[string]any
			if e = json.Unmarshal(raw, &payload); e != nil {
				return e
			}
			return projectSwitchEvent(context.Background(), tx, switchapi.Event{Type: kind, CallID: callID, Payload: payload})
		}
		if e = event("recording.failed", meta); e != nil {
			return e
		}
		stale := meta
		stale.Status = "completed"
		stale.FailureReason = ""
		stale.DurationSamples = 160
		if e = event("recording.saved", stale); e != nil {
			return e
		}
		if e = event("recording.failed", meta); e != nil {
			return e
		}
		olderFailure := meta
		olderFailure.DurationSamples = 160
		olderFailure.LegPaths = nil
		olderFailure.FailureReason = "stale failure"
		if e = event("recording.failed", olderFailure); e != nil {
			return e
		}
		var saved models.Recording
		if e = tx.First(&saved, "id = ?", meta.ID).Error; e != nil {
			return e
		}
		if saved.Status != "failed" || saved.FailureReason != "disk full" || saved.DurationSamples != 320 || saved.LegPaths["leg"] != "aligned.wav" || saved.Channels != 1 || saved.SampleRateHz != 8000 {
			t.Fatalf("failure projection lost metadata: %+v", saved)
		}
		listed, e := recmeta.NewService(tx).List(context.Background(), 1, 20, callID)
		if e != nil {
			return e
		}
		if len(listed.Items) != 2 {
			t.Fatal("API projection omitted recording")
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal(e)
	}
}
