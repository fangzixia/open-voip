package observability

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	in := "Authorization: Bearer abc.def\r\nCookie: sid=secret\r\nAuthorization: Digest username=\"u\", response=\"deadbeef\"\r\npassword=secret\r\na=ice-pwd:ice-secret\r\nturn_credential: turn-secret"
	got := Redact(in)
	for _, secret := range []string{"abc.def", "sid=secret", "deadbeef", "password=secret", "ice-secret", "turn-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q leaked in %q", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redaction marker missing: %q", got)
	}
}

func TestContextFieldsMerge(t *testing.T) {
	ctx := WithFields(context.Background(), Fields{TraceID: "trace", RequestID: "request"})
	ctx = WithFields(ctx, Fields{CallID: "call"})
	got := FromContext(ctx)
	if got.TraceID != "trace" || got.RequestID != "request" || got.CallID != "call" {
		t.Fatalf("unexpected fields: %+v", got)
	}
}

func TestRotatingWriterGzipArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.jsonl")
	w, err := NewRotatingWriter(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	block := strings.Repeat("x", 700*1024)
	if _, err = w.Write([]byte(block)); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write([]byte(block)); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(path + ".*.gz")
	if err != nil || len(matches) != 1 {
		t.Fatalf("archives=%v err=%v", matches, err)
	}
	if st, err := os.Stat(path); err != nil || st.Size() == 0 {
		t.Fatalf("active log missing: %v", err)
	}
}
