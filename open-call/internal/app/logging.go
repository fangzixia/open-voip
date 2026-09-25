package app

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"open-call/internal/config"
	"open-call/internal/datetime"
	"open-call/internal/observability"
)

type multiHandler []slog.Handler

func (h multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}
func (h multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}
func (h multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(multiHandler, len(h))
	for i := range h {
		out[i] = h[i].WithAttrs(attrs)
	}
	return out
}
func (h multiHandler) WithGroup(name string) slog.Handler {
	out := make(multiHandler, len(h))
	for i := range h {
		out[i] = h[i].WithGroup(name)
	}
	return out
}

type rotatingWriter struct {
	mu         sync.Mutex
	path       string
	maxBytes   int64
	maxAgeDays int
	file       *os.File
	size       int64
}

func newRotatingWriter(path string, maxBytes int64, maxAgeDays int) (*rotatingWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	w := &rotatingWriter{path: path, maxBytes: maxBytes, maxAgeDays: maxAgeDays}
	if err := w.open(); err != nil {
		return nil, err
	}
	if maxAgeDays > 0 {
		_ = w.cleanup()
	}
	return w, nil
}
func (w *rotatingWriter) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file, w.size = file, info.Size()
	return nil
}
func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.maxBytes > 0 && w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}
func (w *rotatingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	rotated := fmt.Sprintf("%s.%s", w.path, time.Now().UTC().Format("20060102T150405.000000000"))
	if err := os.Rename(w.path, rotated); err != nil {
		_ = w.open()
		return err
	}
	if err := gzipFile(rotated); err != nil {
		_ = w.open()
		return err
	}
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
	gz := gzip.NewWriter(dst)
	_, copyErr := io.Copy(gz, src)
	srcErr := src.Close()
	closeErr := gz.Close()
	fileErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if srcErr != nil {
		return srcErr
	}
	if closeErr != nil {
		return closeErr
	}
	if fileErr != nil {
		return fileErr
	}
	return os.Remove(path)
}
func (w *rotatingWriter) cleanup() error {
	cutoff := time.Now().Add(-time.Duration(w.maxAgeDays) * 24 * time.Hour)
	matches, err := filepath.Glob(w.path + ".*.gz")
	if err != nil {
		return err
	}
	for _, path := range matches {
		if info, statErr := os.Stat(path); statErr == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
	}
	return nil
}
func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

type logResources struct{ app, trace *rotatingWriter }

func (r *logResources) Close() error {
	var first error
	if r.app != nil {
		first = r.app.Close()
	}
	if r.trace != nil {
		if err := r.trace.Close(); first == nil {
			first = err
		}
	}
	return first
}

// newLogger creates stdout + application JSONL + trace JSONL sinks.
func newLogger(cfg config.LogConfig) (*slog.Logger, io.Closer, error) {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
		if attr.Value.Kind() == slog.KindTime {
			return slog.String(attr.Key, datetime.Format(attr.Value.Time()))
		}
		cleaned := observability.SanitizeMap(map[string]any{attr.Key: attr.Value.Any()})[attr.Key]
		if err, ok := cleaned.(error); ok {
			cleaned = observability.SanitizeString(err.Error())
		}
		return slog.Any(attr.Key, cleaned)
	}}
	maxBytes := cfg.MaxSizeMB * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 100 * 1024 * 1024
	}
	dir := strings.TrimSpace(cfg.Dir)
	if dir == "" {
		dir = "logs/open-call"
	}
	appWriter, err := newRotatingWriter(filepath.Join(dir, "app.jsonl"), maxBytes, cfg.MaxAgeDays)
	if err != nil {
		return nil, nil, fmt.Errorf("创建应用日志: %w", err)
	}
	traceWriter, err := newRotatingWriter(filepath.Join(dir, "trace.jsonl"), maxBytes, cfg.MaxAgeDays)
	if err != nil {
		_ = appWriter.Close()
		return nil, nil, fmt.Errorf("创建追踪日志: %w", err)
	}

	var stdout slog.Handler
	if strings.ToLower(cfg.Format) == "text" {
		stdout = slog.NewTextHandler(os.Stdout, opts)
	} else {
		stdout = slog.NewJSONHandler(os.Stdout, opts)
	}
	logger := slog.New(multiHandler{stdout, slog.NewJSONHandler(appWriter, opts)})
	observability.SetRecorder(observability.NewRecorder(traceWriter))
	return logger, &logResources{app: appWriter, trace: traceWriter}, nil
}
