// Package app 是 open-call 组合根（L4 + BFF + Platform API）。
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

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm/logger"

	apphttp "open-call/internal/app/http"
	"open-call/internal/app/platform"
	"open-call/internal/app/ws"
	"open-call/internal/config"
	"open-call/internal/integration/switchapi"
	"open-call/internal/layers/biz/agent"
	"open-call/internal/layers/biz/audit"
	"open-call/internal/layers/biz/auth"
	"open-call/internal/layers/biz/authz"
	"open-call/internal/layers/biz/cdr"
	"open-call/internal/layers/biz/configio"
	"open-call/internal/layers/biz/configpub"
	"open-call/internal/layers/biz/guest"
	"open-call/internal/layers/biz/ivr"
	"open-call/internal/layers/biz/oidcauth"
	"open-call/internal/layers/biz/queue"
	"open-call/internal/layers/biz/recmeta"
	"open-call/internal/layers/biz/report"
	"open-call/internal/layers/biz/skill"
	"open-call/internal/layers/biz/user"
	"open-call/internal/layers/biz/webhook"
	"open-call/internal/store"
	"open-call/internal/store/migrate"
)

// Run 启动 open-call。
func Run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log, logFiles, err := newLogger(cfg.Log)
	if err != nil {
		return err
	}
	defer logFiles.Close()
	slog.SetDefault(log)

	gormLog := logger.Warn
	if cfg.Log.Level == "debug" {
		gormLog = logger.Info
	}

	db, err := store.Open(cfg.Database.DSN, gormLog)
	if err != nil {
		return err
	}
	if err := migrate.Migrate(db); err != nil {
		return fmt.Errorf("数据库迁移: %w", err)
	}
	if err := store.Ping(db); err != nil {
		return fmt.Errorf("数据库 Ping: %w", err)
	}
	if cfg.Bootstrap.Enabled {
		if err := store.SeedIfEmpty(db, cfg.Bootstrap, log); err != nil {
			return err
		}
		if err := store.SeedDefaultDID(db, log); err != nil {
			return err
		}
	}

	switchClient := switchapi.NewClient(cfg.Integration)
	wsHub := ws.NewHub(log, cfg.Security.AllowedOrigins...)
	queueSvc := queue.NewService(db, wsHub, queue.PolicyDefaults{
		Mode:          "audio",
		NotifyMessage: cfg.Recordings.NotifyMessage,
		RetainDays:    cfg.Recordings.RetainDays,
	})
	agentSvc := agent.NewService(db, wsHub)
	authSvc := auth.NewService(db, cfg.JWT)
	authSvc.ConfigureOIDC(cfg.OIDC.Enabled, cfg.OIDC.EmergencyAdmin)
	if err := authSvc.ValidateEmergencyAdmin(context.Background()); err != nil {
		return err
	}
	authzSvc := authz.NewService(db)
	oidcSvc, err := oidcauth.NewService(context.Background(), cfg.OIDC, db, authzSvc)
	if err != nil {
		return err
	}
	if oidcSvc != nil {
		authSvc.SetOIDCRefresher(oidcSvc.Refresh)
	}
	userSvc := user.NewService(db)
	configSnap := configpub.NewSnapshotService(db)
	cdrRecorder := cdr.NewRecorderService(db)
	recMeta := recmeta.NewService(db)
	recMeta.SetFiles(switchClient)
	ivrSvc := ivr.NewService(db)
	skillSvc := skill.NewService(db)
	reportSvc := report.NewService(db, switchClient)
	hookSvc := webhook.NewService(db, cfg.Webhook)
	auditSvc := audit.NewService(db)
	cfgIO := configio.NewService(db)
	guestSvc := guest.NewService(db, switchClient, cfg.Public.GuestBaseURL)

	wsHub.Configure(authSvc, switchClient, agentSvc, hookSvc)

	// 后台任务使用独立上下文，停机时先停止领取新任务。
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go monitorLogDisk(workerCtx, log, cfg.Log.Dir)
	go hookSvc.RunWorker(workerCtx, log)
	go runMaintenance(workerCtx, log, authSvc, guestSvc, recMeta)

	platformHandler := platform.NewRouter(platform.Deps{
		Secret:     cfg.Integration.Secret,
		Queues:     queueSvc,
		Agents:     agentSvc,
		Snapshots:  configSnap,
		CDR:        cdrRecorder,
		Recordings: recMeta,
		Hub:        wsHub,
	})

	apiRouter := apphttp.NewRouter(apphttp.RouterDeps{
		Config:        *cfg,
		Auth:          authSvc,
		Authorization: authzSvc,
		OIDC:          oidcSvc,
		Users:         userSvc,
		Agents:        agentSvc,
		Queues:        queueSvc,
		Guests:        guestSvc,
		CDR:           cdrRecorder,
		IVR:           ivrSvc,
		Skills:        skillSvc,
		Recordings:    recMeta,
		Reports:       reportSvc,
		Webhooks:      hookSvc,
		Audit:         auditSvc,
		ConfigIO:      cfgIO,
		Snapshots:     configSnap,
		Hub:           wsHub,
		Calls:         switchClient,
		Status: apphttp.StatusProvider{
			DB:            db,
			Runtime:       switchClient,
			WSConnections: wsHub.ConnectionCount,
			Webhooks:      hookSvc,
			Audit:         auditSvc,
		},
	})

	root := chi.NewRouter()
	root.Mount("/platform/v1", platformHandler)
	root.Mount("/", apphttp.WrapSwitchBFF(cfg.Integration, authSvc, apiRouter, cfg.Security.AllowedOrigins))

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		log.Info("open-call 启动", "listen", cfg.Server.Listen)
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
	stopWorkers()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	return srv.Shutdown(shutdownCtx)
}

// runMaintenance 定期清理过期认证记录、访客会话和到期录音。
func runMaintenance(ctx context.Context, log *slog.Logger, authSvc *auth.Service, guestSvc *guest.Service, recordings *recmeta.Service) {
	run := func() {
		if err := authSvc.CleanupExpired(ctx); err != nil && ctx.Err() == nil {
			log.Error("清理过期认证记录失败", "err", err)
		}
		if err := guestSvc.Cleanup(ctx); err != nil && ctx.Err() == nil {
			log.Error("清理过期访客会话失败", "err", err)
		}
		if _, err := recordings.PurgeExpired(ctx); err != nil && ctx.Err() == nil {
			log.Error("自动清理录音失败", "err", err)
		}
	}
	run()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
