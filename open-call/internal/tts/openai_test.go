package tts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"open-call/internal/config"
)

func TestOpenAICompatibleSynthesize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte{0xff, 0xfb, 0x90, 0x00})
	}))
	defer srv.Close()

	p, err := newOpenAICompatible(config.TTSOpenAICompatibleConfig{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Model:   "test-model",
		Voice:   "alloy",
	})
	if err != nil {
		t.Fatal(err)
	}
	audio, ct, err := p.Synthesize(context.Background(), "你好", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(audio) != 4 || ct != "audio/mpeg" {
		t.Fatalf("got len=%d ct=%s", len(audio), ct)
	}
}
