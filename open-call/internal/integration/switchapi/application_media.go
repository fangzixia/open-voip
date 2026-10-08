package switchapi

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type PlaybackStatus struct {
	ID    string `json:"playback_id"`
	LegID string `json:"leg_id"`
	State string `json:"state"`
}

func (c *Client) CreateApplicationCall(ctx context.Context, id, ref string) error {
	return c.do(ctx, http.MethodPost, "/switch/v1/calls", map[string]any{"call_id": id, "business_ref": ref}, nil)
}
func (c *Client) DialApplicationSIP(ctx context.Context, id, destination, trunk string) (string, error) {
	var out struct {
		LegID string `json:"leg_id"`
	}
	err := c.do(WithMutation(ctx, Mutation{IdempotencyKey: "dial:" + id}), http.MethodPost, "/switch/v1/calls/"+id+"/legs/sip", map[string]string{"destination": destination, "trunk_id": trunk}, &out)
	return out.LegID, err
}
func (c *Client) PlayAsset(ctx context.Context, callID, legID, asset, key string) (string, error) {
	var out struct {
		ID string `json:"playback_id"`
	}
	err := c.do(WithMutation(ctx, Mutation{IdempotencyKey: key}), http.MethodPost, "/switch/v1/calls/"+callID+"/legs/"+legID+"/playbacks", map[string]string{"asset_id": asset}, &out)
	return out.ID, err
}
func (c *Client) Playback(ctx context.Context, callID, legID, id string) (PlaybackStatus, error) {
	var out PlaybackStatus
	err := c.do(ctx, http.MethodGet, "/switch/v1/calls/"+callID+"/legs/"+legID+"/playbacks/"+id, nil, &out)
	return out, err
}

// PCMStream is the application SDK: provider PCM crosses the service boundary
// without phone codecs, resampling, WebRTC negotiation or a local pacing loop.
type PCMStream struct {
	conn       *websocket.Conn
	mu         sync.Mutex
	sendMu     sync.Mutex
	ack        chan outputAck
	generation uint64
}

func (c *Client) OpenPCM(ctx context.Context, callID, legID string, inputRate, outputRate int, direction string) (*PCMStream, error) {
	u := strings.Replace(strings.Replace(c.base, "https://", "wss://", 1), "http://", "ws://", 1)
	q := url.Values{"input_rate": {strconv.Itoa(inputRate)}, "output_rate": {strconv.Itoa(outputRate)}, "direction": {direction}}
	conn, _, err := websocket.Dial(ctx, u+"/switch/v1/calls/"+callID+"/legs/"+legID+"/media?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(96000)
	return &PCMStream{conn: conn, generation: 1, ack: make(chan outputAck, 1)}, nil
}
func (p *PCMStream) Close() { _ = p.conn.CloseNow() }
func (p *PCMStream) write(ctx context.Context, t websocket.MessageType, b []byte) error {
	wc, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return p.conn.Write(wc, t, b)
}

type outputAck struct {
	Type       string `json:"type"`
	Generation uint64 `json:"generation"`
}

// Send waits for bounded-queue credit, while Clear remains immediately writable.
func (p *PCMStream) Send(ctx context.Context, g uint64, pcm []byte) error {
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	b := make([]byte, len(pcm)+8)
	binary.BigEndian.PutUint64(b, g)
	copy(b[8:], pcm)
	for {
		p.mu.Lock()
		if g != p.generation {
			p.mu.Unlock()
			return nil
		}
		err := p.write(ctx, websocket.MessageBinary, b)
		p.mu.Unlock()
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case a := <-p.ack:
			if a.Type != "output.backpressure" {
				return nil
			}
			timer := time.NewTimer(20 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
}
func (p *PCMStream) Clear(ctx context.Context, g uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if g <= p.generation {
		return errors.New("generation must increase")
	}
	p.generation = g
	b, _ := json.Marshal(map[string]any{"type": "output.clear", "generation": g})
	return p.write(ctx, websocket.MessageText, b)
}
func (p *PCMStream) Finish(ctx context.Context, g uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if g != p.generation {
		return nil
	}
	b, _ := json.Marshal(map[string]any{"type": "output.finish", "generation": g})
	return p.write(ctx, websocket.MessageText, b)
}
func (p *PCMStream) Receive(ctx context.Context) ([]byte, error) {
	for {
		t, b, err := p.conn.Read(ctx)
		if err != nil {
			return nil, err
		}
		if t == websocket.MessageBinary {
			return b, nil
		}
		var a outputAck
		if json.Unmarshal(b, &a) == nil && (a.Type == "output.accepted" || a.Type == "output.backpressure" || a.Type == "output.discarded") {
			select {
			case p.ack <- a:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
}
