package switchapi

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"open-call/internal/config"
	"testing"
	"time"
)

func TestPCMCreditClearAndStaleOutput(t *testing.T) {
	busy := make(chan struct{})
	cleared := make(chan struct{})
	newAudio := make(chan uint64, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("input_rate") != "16000" || r.URL.Query().Get("output_rate") != "24000" {
			t.Error("PCM formats lost")
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		first := true
		for {
			typ, b, err := c.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				g := binary.BigEndian.Uint64(b[:8])
				kind := "output.accepted"
				if first {
					first = false
					kind = "output.backpressure"
					close(busy)
					continue
				} else {
					select {
					case newAudio <- g:
					default:
					}
				}
				ack, _ := json.Marshal(outputAck{Type: kind, Generation: g})
				if c.Write(ctx, websocket.MessageText, ack) != nil {
					return
				}
			} else {
				var cmd outputAck
				_ = json.Unmarshal(b, &cmd)
				if cmd.Type == "output.clear" {
					ack, _ := json.Marshal(outputAck{Type: "output.backpressure", Generation: 1})
					_ = c.Write(ctx, websocket.MessageText, ack)
					close(cleared)
				}
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p, err := NewClient(config.IntegrationConfig{SwitchBaseURL: server.URL}).OpenPCM(ctx, "c", "a", 16000, 24000, "duplex")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	go func() { _, _ = p.Receive(ctx) }()
	done := make(chan error, 1)
	go func() { done <- p.Send(ctx, 1, []byte{1, 0}) }()
	select {
	case <-busy:
	case <-ctx.Done():
		t.Fatal("no backpressure")
	}
	if err = p.Clear(ctx, 2); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cleared:
	case <-ctx.Done():
		t.Fatal("clear blocked behind pending output")
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("stale sender stuck")
	}
	if err = p.Send(ctx, 1, []byte{2, 0}); err != nil {
		t.Fatal(err)
	} // discarded locally
	if err = p.Send(ctx, 2, []byte{3, 0}); err != nil {
		t.Fatal(err)
	}
	select {
	case g := <-newAudio:
		if g != 2 {
			t.Fatalf("old generation replayed: %d", g)
		}
	case <-ctx.Done():
		t.Fatal("new output missing")
	}
}
