package http

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"open-switch/internal/errs"
	"open-switch/internal/ports"
)

func (d SwitchRouterDeps) handleMediaStream(w http.ResponseWriter, r *http.Request) {
	p, ok := d.CallControl.(ports.ApplicationControlPort)
	if !ok {
		writeErr(w, errs.NotImplemented("媒体流未配置"))
		return
	}
	in, _ := strconv.Atoi(r.URL.Query().Get("input_rate"))
	out, _ := strconv.Atoi(r.URL.Query().Get("output_rate"))
	a, err := p.OpenApplicationStream(r.Context(), chi.URLParam(r, "callId"), chi.URLParam(r, "legId"), ports.MediaStreamOptions{Input: ports.PCMFormat{SampleRate: in}, Output: ports.PCMFormat{SampleRate: out}, Direction: r.URL.Query().Get("direction")})
	if err != nil {
		writeErr(w, err)
		return
	}
	defer a.Close()
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(96008)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	readErr := make(chan error, 1)
	go func() { readErr <- readMediaOutput(ctx, c, a) }()
	write := func(typ websocket.MessageType, b []byte) error {
		wc, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		return c.Write(wc, typ, b)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.Done():
			_ = c.Close(websocket.StatusGoingAway, "media session closed")
			return
		case err := <-readErr:
			if err != nil {
				_ = c.Close(websocket.StatusPolicyViolation, "invalid audio or producer exceeded queue limit")
			}
			return
		case raw := <-a.Input():
			if write(websocket.MessageBinary, raw) != nil {
				return
			}
		case ev := <-a.Events():
			b, _ := json.Marshal(ev)
			if write(websocket.MessageText, b) != nil {
				return
			}
		}
	}
}

func readMediaOutput(ctx context.Context, c *websocket.Conn, a ports.ApplicationStream) error {
	for {
		typ, b, err := c.Read(ctx)
		if err != nil {
			return err
		}
		if typ == websocket.MessageBinary {
			if len(b) < 10 {
				return errors.New("missing generation or PCM")
			}
			g := binary.BigEndian.Uint64(b[:8])
			err = a.Write(g, b[8:])
			kind := "output.accepted"
			if errors.Is(err, ports.ErrAudioBackpressure) {
				kind = "output.backpressure"
			} else if errors.Is(err, ports.ErrStaleGeneration) {
				kind = "output.discarded"
			} else if err != nil {
				return err
			}
			ack, _ := json.Marshal(ports.MediaOutputEvent{Type: kind, Generation: g})
			wc, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = c.Write(wc, websocket.MessageText, ack)
			cancel()
			if err != nil {
				return err
			}
			continue
		}
		var cmd struct {
			Type       string `json:"type"`
			Generation uint64 `json:"generation"`
		}
		if json.Unmarshal(b, &cmd) != nil {
			return errors.New("invalid command")
		}
		switch cmd.Type {
		case "output.clear":
			err = a.Clear(cmd.Generation)
		case "output.finish":
			err = a.Finish(cmd.Generation)
		default:
			return errors.New("unknown media command")
		}
		if err != nil {
			return err
		}
	}
}

func (d SwitchRouterDeps) handleLegPlaybackGet(w http.ResponseWriter, r *http.Request) {
	p, ok := d.CallControl.(ports.ApplicationControlPort)
	if !ok {
		writeErr(w, errs.NotImplemented("播放查询未配置"))
		return
	}
	out, err := p.GetLegPlayback(r.Context(), chi.URLParam(r, "callId"), chi.URLParam(r, "legId"), chi.URLParam(r, "playbackId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
