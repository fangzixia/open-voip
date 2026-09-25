package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingWriterCompressesAndRetains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.jsonl")
	writer, err := newRotatingWriter(path, 8, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("second\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(path + ".*.gz")
	if err != nil || len(matches) != 1 {
		t.Fatalf("rotated files=%v err=%v", matches, err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("active log missing: info=%v err=%v", info, err)
	}
}
