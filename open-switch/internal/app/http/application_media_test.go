package http

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/coder/websocket"
	"net/http/httptest"
	"open-switch/internal/ports"
	"strings"
	"sync"
	"testing"
	"time"
)

type streamProbe struct {
	input  chan []byte
	events chan ports.MediaOutputEvent
	done   chan struct{}
	once   sync.Once
	writes chan uint64
	clears chan uint64
}

func (p *streamProbe) Input() <-chan []byte                  { return p.input }
func (p *streamProbe) Events() <-chan ports.MediaOutputEvent { return p.events }
func (p *streamProbe) Done() <-chan struct{}                 { return p.done }
func (p *streamProbe) Err() error                            { return nil }
func (p *streamProbe) Write(g uint64, b []byte) error        { p.writes <- g; return nil }
func (p *streamProbe) Clear(g uint64) error                  { p.clears <- g; return nil }
func (p *streamProbe) Finish(g uint64) error {
	p.events <- ports.MediaOutputEvent{Type: "output.finished", Generation: g}
	return nil
}
func (p *streamProbe) Close() { p.once.Do(func() { close(p.done) }) }

type streamControl struct {
	ports.CallControlPort
	stream *streamProbe
	opts   chan ports.MediaStreamOptions
}

func (p *streamControl) GetCall(context.Context, string) (ports.CallView, error) {
	return ports.CallView{ID: "call", State: "active"}, nil
}
func (p *streamControl) OpenApplicationStream(_ context.Context, _, _ string, o ports.MediaStreamOptions) (ports.ApplicationStream, error) {
	p.opts <- o
	return p.stream, nil
}
func (p *streamControl) GetLegPlayback(context.Context, string, string, string) (ports.PlaybackStatus, error) {
	return ports.PlaybackStatus{}, nil
}

func TestApplicationWebSocketContractAndCleanup(t *testing.T) {
	p := &streamProbe{input: make(chan []byte, 1), events: make(chan ports.MediaOutputEvent, 1), done: make(chan struct{}), writes: make(chan uint64, 1), clears: make(chan uint64, 1)}
	control := &streamControl{stream: p, opts: make(chan ports.MediaStreamOptions, 1)}
	server := httptest.NewServer(NewSwitchRouter(SwitchRouterDeps{RouterDeps: RouterDeps{CallControl: control}}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(server.URL, "http://", "ws://", 1)+"/switch/v1/calls/call/legs/agent/media?direction=duplex&input_rate=16000&output_rate=24000", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if o := <-control.opts; o.Input.SampleRate != 16000 || o.Output.SampleRate != 24000 || o.Direction != "duplex" {
		t.Fatalf("format: %+v", o)
	}
	b := make([]byte, 10)
	binary.BigEndian.PutUint64(b, 1)
	if err = c.Write(ctx, websocket.MessageBinary, b); err != nil {
		t.Fatal(err)
	}
	_, raw, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ack ports.MediaOutputEvent
	_ = json.Unmarshal(raw, &ack)
	if ack.Type != "output.accepted" || ack.Generation != 1 {
		t.Fatalf("ack=%s", raw)
	}
	p.input <- []byte{10, 0}
	typ, raw, err := c.Read(ctx)
	if err != nil || typ != websocket.MessageBinary || len(raw) != 2 || raw[0] != 10 {
		t.Fatalf("input: %v %v", raw, err)
	}
	if err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"output.clear","generation":2}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case g := <-p.clears:
		if g != 2 {
			t.Fatal(g)
		}
	case <-ctx.Done():
		t.Fatal("clear not forwarded")
	}
	if err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"output.finish","generation":2}`)); err != nil {
		t.Fatal(err)
	}
	_, raw, err = c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &ack)
	if ack.Type != "output.finished" {
		t.Fatalf("completion=%s", raw)
	}
	_ = c.CloseNow()
	select {
	case <-p.Done():
	case <-ctx.Done():
		t.Fatal("disconnected socket leaked media session")
	}
}
