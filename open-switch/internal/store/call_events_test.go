package store

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/logger"
	"open-switch/internal/ports"
	"open-switch/internal/store/migrate"
)

func TestCallEventsConcurrentCursorIntegration(t *testing.T) {
	dsn := os.Getenv("OPEN_VOIP_TEST_DSN")
	if dsn == "" {
		t.Skip("需要独立测试数据库 OPEN_VOIP_TEST_DSN")
	}
	db, err := Open(dsn, logger.Warn)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Migrate(db); err != nil {
		t.Fatal(err)
	}
	stream := CallEvents{DB: db}
	ctx := context.Background()
	missing := ports.CallEvent{CallID: uuid.NewString(), Type: "recording.failed"}
	if err := stream.Append(ctx, &missing); err == nil || missing.ID != 0 {
		t.Fatal("event for absent call reused sequence zero")
	}
	calls := []string{uuid.New().String(), uuid.New().String()}
	for _, id := range calls {
		if e := db.Exec("INSERT INTO os_calls(id,direction,session_type,state,created_at,updated_at) VALUES (?,'inbound','audio','active',?,?)", id, time.Now(), time.Now()).Error; e != nil {
			t.Fatal(e)
		}
	}
	defer db.Exec("DELETE FROM os_calls WHERE id IN (?, ?)", calls[0], calls[1])
	defer db.Exec("DELETE FROM os_call_events WHERE call_id IN (?, ?)", calls[0], calls[1])
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ev := ports.CallEvent{CallID: calls[i%2], Type: "call.test", Payload: map[string]any{"n": i}}
			errs <- stream.Append(ctx, &ev)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, callID := range calls {
		rows, err := stream.List(ctx, 0, callID, 100)
		if err != nil || len(rows) != 10 {
			t.Fatalf("call %s rows=%d err=%v", callID, len(rows), err)
		}
		for i, row := range rows {
			if row.Seq != int64(i+1) || row.ID == 0 || row.Type != "call.test" {
				t.Fatalf("unexpected event: %+v", row)
			}
		}
	}
}
