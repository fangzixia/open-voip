package app

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"open-switch/internal/config"
	"open-switch/internal/datetime"
	"open-switch/internal/observability"
)

// newLogger 根据配置创建全局 slog 默认 logger。
func newLogger(cfg config.LogConfig) (*slog.Logger, func() error, error) {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	appFile, err := observability.NewRotatingWriter(filepath.Join(cfg.Dir, "app.jsonl"), cfg.MaxSizeMB)
	if err != nil {
		return nil, nil, fmt.Errorf("打开应用日志: %w", err)
	}
	traceFile, err := observability.NewRotatingWriter(filepath.Join(cfg.Dir, "trace.jsonl"), cfg.MaxSizeMB)
	if err != nil {
		_ = appFile.Close()
		return nil, nil, fmt.Errorf("打开追踪日志: %w", err)
	}

	var console slog.Handler
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
		if attr.Value.Kind() == slog.KindTime {
			return slog.String(attr.Key, datetime.Format(attr.Value.Time()))
		}
		return attr
	}}
	if strings.ToLower(cfg.Format) == "text" {
		console = slog.NewTextHandler(os.Stdout, opts)
	} else {
		console = slog.NewJSONHandler(os.Stdout, opts)
	}
	app := slog.NewJSONHandler(io.Writer(appFile), opts)
	trace := observability.NewTraceOnlyHandler(slog.NewJSONHandler(io.Writer(traceFile), opts))
	handler := observability.NewRedactingHandler(observability.NewMultiHandler(console, app, trace))
	closeFn := func() error {
		if err := appFile.Close(); err != nil {
			_ = traceFile.Close()
			return err
		}
		return traceFile.Close()
	}
	return slog.New(handler), closeFn, nil
}
