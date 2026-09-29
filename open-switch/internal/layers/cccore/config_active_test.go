package cccore

import (
	"context"
	"os"
	"testing"

	"gorm.io/gorm/logger"
	"uuid"

	"open-switch/internal/ports"
	"open-switch/internal/scope"
	"open-switch/internal/store"
	"open-switch/internal/store/migrate"
)

func TestGetActiveConfigurationAfterActivate(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	db, err := store.Open(dsn, logger.Silent)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	svc := New(db, store.CallEvents{DB: db}, Options{})
	app := "active-read-" + uuid.New().String()
	ctx := scope.WithApplication(context.Background(), app)
	q := uuid.New().String()
	bundle := ports.ConfigBundle{
		Queues: []ports.QueueConfig{{ID: q, Name: "voice"}},
	}
	v, err := svc.StoreConfig(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ActivateConfig(ctx, v.Version); err != nil {
		t.Fatal(err)
	}
	active, err := svc.GetActiveConfiguration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active.Version.Version != v.Version {
		t.Fatalf("version %d != %d", active.Version.Version, v.Version)
	}
	if len(active.Bundle.Queues) != 1 || active.Bundle.Queues[0].ID != q {
		t.Fatalf("bundle queues %+v", active.Bundle.Queues)
	}
	summary, err := svc.GetActiveConfigurationSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Version != v.Version {
		t.Fatalf("summary version %d", summary.Version)
	}
}
