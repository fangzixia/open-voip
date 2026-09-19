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

	apphttp "open-voip/internal/app/http"
	"open-voip/internal/app/ws"
	"open-voip/internal/config"
	"open-voip/internal/layers/biz/agent"
	"open-voip/internal/layers/biz/audit"
	"open-voip/internal/layers/biz/auth"
	"open-voip/internal/layers/biz/cdr"
	"open-voip/internal/layers/biz/configio"
	"open-voip/internal/layers/biz/configpub"
	"open-voip/internal/layers/biz/guest"
	"open-voip/internal/layers/biz/ivr"
	"open-voip/internal/layers/biz/queue"
	"open-voip/internal/layers/biz/recmeta"
	"open-voip/internal/layers/biz/report"
	"open-voip/internal/layers/biz/skill"
	"open-voip/internal/layers/biz/user"
	"open-voip/internal/layers/biz/webhook"
	"open-voip/internal/layers/control"
	"open-voip/internal/layers/media"
	"open-voip/internal/ports/dto"
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
	if err := store.SeedIfEmpty(db, cfg.Bootstrap, log); err != nil {
		return err
	}
	if err := store.SeedDefaultDID(db, log); err != nil {
		return err
	}

	mediaSvc, err := media.NewService(media.Options{
		ICE: cfg.ICE, TURN: cfg.TURN, RecordingsDir: cfg.Recordings.Dir, SIP: cfg.SIP,
	})
	if err != nil {
		return fmt.Errorf("媒体层: %w", err)
	}
	wsHub := ws.NewHub(log)
	queueSvc := queue.NewService(db, wsHub, queue.PolicyDefaults{
		Mode:          "audio",
		NotifyMessage: cfg.Recordings.NotifyMessage,
		RetainDays:    cfg.Recordings.RetainDays,
	})
	agentSvc := agent.NewService(db, wsHub)
	authSvc := auth.NewService(db, cfg.JWT)
	userSvc := user.NewService(db)
	configSnap := configpub.NewSnapshotService(db)
	cdrRecorder := cdr.NewRecorderService(db)
	callStore := store.NewCallStore(db)
	recMeta := recmeta.NewService(db)
	ivrSvc := ivr.NewService(db)
	skillSvc := skill.NewService(db)
	reportSvc := report.NewService(db)
	hookSvc := webhook.NewService(db, cfg.Webhook)
	auditSvc := audit.NewService(db)
	cfgIO := configio.NewService(db)

	callControl := control.NewService(control.Deps{
		Media:           mediaSvc,
		ACD:             queueSvc,
		Config:          configSnap,
		Agents:          agentSvc,
		RecordingPolicy: queueSvc,
		CDR:             cdrRecorder,
		Calls:           callStore,
		CallEvents:      wsHub,
		Recordings:      recMeta,
	})
	wsHub.Configure(authSvc, callControl, agentSvc, hookSvc)
	guestSvc := guest.NewService(db, callControl, cfg.Public.GuestBaseURL)

	mediaSvc.SetSIPHangupHandler(func(ctx context.Context, callID string) {
		_ = callControl.Hangup(ctx, callID, dto.HangupReasonNormal)
	})
	mediaSvc.SetInboundHandler(func(ctx context.Context, did, from, callID string) (string, string, error) {
		qid, err := configSnap.ResolveDID(ctx, did)
		if err != nil {
			return "", "", err
		}
		id, err := callControl.StartInbound(ctx, dto.InboundRequest{
			CallID: callID, QueueID: qid, Caller: from, SessionType: dto.SessionTypeAudio,
		})
		if err != nil {
			return "", "", err
		}
		return id, "", nil
	})

	router := apphttp.NewRouter(apphttp.RouterDeps{
		Config:      *cfg,
		Auth:        authSvc,
		Users:       userSvc,
		Agents:      agentSvc,
		Queues:      queueSvc,
		Guests:      guestSvc,
		CDR:         cdrRecorder,
		CallControl: callControl,
		Signaling:   callControl,
		IVR:         ivrSvc,
		Skills:      skillSvc,
		Recordings:  recMeta,
		Reports:     reportSvc,
		Webhooks:    hookSvc,
		Audit:       auditSvc,
		ConfigIO:    cfgIO,
		Snapshots:   configSnap,
		Hub:         wsHub,
		Status: apphttp.StatusProvider{
			DB:            db,
			ActiveCalls:   callControl.ActiveCalls,
			WSConnections: wsHub.ConnectionCount,
			RecordingsDir: cfg.Recordings.Dir,
		},
	})

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go callControl.Run(ctx)
	go func() {
		if err := mediaSvc.ServeSIP(ctx); err != nil {
			log.Warn("SIP 监听结束", "err", err)
		}
	}()

	go func() {
		log.Info("open-voip 启动", "listen", cfg.Server.Listen)
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
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	return srv.Shutdown(shutdownCtx)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o750)
}
