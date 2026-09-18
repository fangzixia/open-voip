// Package app 是组合根：唯一允许串联 L2/L3/L4 具体实现并启动 HTTP 服务。
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/gorm/logger"

	"open-voip/internal/config"
	apphttp "open-voip/internal/app/http"
	"open-voip/internal/app/ws"
	"open-voip/internal/layers/biz/agent"
	"open-voip/internal/layers/biz/cdr"
	"open-voip/internal/layers/biz/configpub"
	"open-voip/internal/layers/biz/queue"
	"open-voip/internal/layers/control"
	"open-voip/internal/layers/media"
	"open-voip/internal/store"
)

// Run 加载配置、迁移数据库、组装各层并阻塞监听 HTTP(S)。
func Run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log := newLogger(cfg.Log)
	slog.SetDefault(log)

	if err := ensureDir(cfg.Recordings.Dir); err != nil {
		return fmt.Errorf("录音目录: %w", err)
	}
	if cfg.StaticServe && cfg.Static.Dir != "" {
		if err := ensureDir(cfg.Static.Dir); err != nil {
			return fmt.Errorf("静态资源目录: %w", err)
		}
	}

	gormLog := logger.Warn
	if cfg.Log.Level == "debug" {
		gormLog = logger.Info
	}

	db, err := store.Open(cfg.Database.DSN, gormLog)
	if err != nil {
		return err
	}
	if err := store.AutoMigrate(db); err != nil {
		return fmt.Errorf("AutoMigrate: %w", err)
	}
	if err := store.Ping(db); err != nil {
		return fmt.Errorf("数据库 Ping: %w", err)
	}

	// L2 → L3 → L4 Port 实现，依赖方向见 architecture §2.3
	mediaSvc := media.NewService()
	acdSvc := queue.NewACDService()
	recordingPolicy := queue.NewRecordingPolicyService()
	configSnap := configpub.NewSnapshotService()
	agentDir := agent.NewDirectoryService()
	cdrRecorder := cdr.NewRecorderService()
	wsHub := ws.NewHub(log, cfg.WebSocketOriginPatterns())

	callControl := control.NewService(control.Deps{
		Media:           mediaSvc,
		ACD:             acdSvc,
		Config:          configSnap,
		Agents:          agentDir,
		RecordingPolicy: recordingPolicy,
		CDR:             cdrRecorder,
		CallEvents:      wsHub,
	})

	router := apphttp.NewRouter(apphttp.RouterDeps{
		Config:      *cfg,
		CallControl: callControl,
		Hub:         wsHub,
		Status: apphttp.StatusProvider{
			DB:            db,
			ActiveCalls:   callControl.ActiveCalls,
			WSConnections: wsHub.ConnectionCount,
		},
	})

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("open-voip 启动",
			"listen", cfg.Server.Listen,
			"public_url", cfg.Server.PublicURL,
		)
		var serveErr error
		if cfg.TLS.Enabled {
			serveErr = srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		} else {
			serveErr = srv.ListenAndServe()
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error("HTTP 服务异常退出", "err", serveErr)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o750)
}
