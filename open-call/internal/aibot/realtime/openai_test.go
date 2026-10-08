package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProviderInterruptionDiscardsLateResponseAudio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		for _, ev := range []map[string]any{
			{"type": "response.created", "response": map[string]string{"id": "old"}},
			{"type": "response.audio.delta", "response_id": "old", "delta": base64.StdEncoding.EncodeToString([]byte{1, 0})},
			{"type": "input_audio_buffer.speech_started"},
			{"type": "response.audio.delta", "response_id": "old", "delta": base64.StdEncoding.EncodeToString([]byte{2, 0})},
			{"type": "response.created", "response": map[string]string{"id": "new"}},
			{"type": "response.audio.delta", "response_id": "new", "delta": base64.StdEncoding.EncodeToString([]byte{3, 0})},
			{"type": "response.audio.done", "response_id": "new"},
		} {
			b, _ := json.Marshal(ev)
			if c.Write(r.Context(), websocket.MessageText, b) != nil {
				return
			}
		}
		_ = c.Close(websocket.StatusNormalClosure, "done")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(server.URL, "http://", "ws://", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	s := &Session{conn: c, outRateHz: 24000}
	var values []byte
	var generations []uint64
	var finished uint64
	err = s.readLoop(ctx, OutputCallbacks{Audio: func(g uint64, b []byte) error { values = append(values, b[0]); return nil }, Clear: func(g uint64) error { generations = append(generations, g); return nil }, Finish: func(g uint64) error { finished = g; return nil }})
	if len(values) != 2 || values[0] != 1 || values[1] != 3 || len(generations) != 3 || finished != 4 {
		t.Fatalf("late response leaked: audio=%v generations=%v finished=%d err=%v", values, generations, finished, err)
	}
}
