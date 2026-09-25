package observability

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RotatingWriter rotates by size and gzip-compresses archives. Archives are never deleted.
type RotatingWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
	size     int64
}

func NewRotatingWriter(path string, maxMB int) (*RotatingWriter, error) {
	if maxMB <= 0 {
		maxMB = 100
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	w := &RotatingWriter{path: path, maxBytes: int64(maxMB) << 20}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file, w.size = f, st.Size()
	return nil
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	archive := fmt.Sprintf("%s.%s", w.path, stamp)
	if err := os.Rename(w.path, archive); err != nil {
		return err
	}
	if err := gzipFile(archive); err != nil {
		return err
	}
	w.size = 0
	return w.open()
}

func gzipFile(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(path+".gz", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		_ = src.Close()
		return err
	}
	ok := false
	defer func() {
		_ = dst.Close()
		if !ok {
			_ = os.Remove(path + ".gz")
		}
	}()
	gz := gzip.NewWriter(dst)
	if _, err = io.Copy(gz, src); err == nil {
		err = gz.Close()
	}
	if closeErr := src.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = dst.Sync()
	}
	if err != nil {
		return err
	}
	ok = true
	return os.Remove(path)
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
