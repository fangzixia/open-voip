// Package app 是 open-switch 组合根。
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

	apphttp "open-switch/internal/app/http"
	"open-switch/internal/config"
	"open-switch/internal/errs"
	"open-switch/internal/layers/cccore"
	"open-switch/internal/layers/control"
	"open-switch/internal/layers/integration"
	"open-switch/internal/layers/media"
	"open-switch/internal/ports/dto"
	"open-switch/internal/scope"
	"open-switch/internal/store"
	"open-switch/internal/store/migrate"
)

// Run 启动 open-switch（L2/L3 + Switch API）。
func Run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	log, closeLogs, err := newLogger(cfg.Log)
	if err != nil {
		return err
	}
	defer func() { _ = closeLogs() }()
	slog.SetDefault(log)

	if err := ensureDir(cfg.Recordings.AudioDirPath()); err != nil {
		return fmt.Errorf("音频录制目录: %w", err)
	}
	if err := ensureDir(cfg.Recordings.VideoDirPath()); err != nil {
		return fmt.Errorf("录像目录: %w", err)
	}

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

	mediaSvc, err := media.NewService(media.Options{
		ICE: cfg.ICE, TURN: cfg.TURN,
		AudioRecDir: cfg.Recordings.AudioDirPath(), VideoRecDir: cfg.Recordings.VideoDirPath(),
		VideoFormat: cfg.Recordings.Video.Format, FFmpegPath: cfg.Recordings.Video.FFmpegPath, SIP: cfg.SIP,
		Media: cfg.Media,
	})
	if err != nil {
		return fmt.Errorf("媒体层: %w", err)
	}

	integratorDispatch := &integration.Dispatcher{
		DB: db, CallbackURL: cfg.Integration.EventsCallbackURL, Log: log,
	}
	eventStore := store.CallEvents{DB: db, AfterAppend: integratorDispatch.Enqueue}
	commandStore := store.Commands{DB: db, Events: eventStore}
	routingStore := store.RoutingSessions{DB: db}
	ccCore := cccore.New(db, eventStore, cccore.Options{
		RecordingMode:        "off",
		AudioNotifyMessage:   cfg.Recordings.Audio.NotifyMessage,
		VideoNotifyMessage:   cfg.Recordings.Video.NotifyMessage,
	})
	controlDeps := control.Deps{
		BusinessActions: ccCore, Media: mediaSvc, ACD: ccCore, Config: ccCore, Agents: ccCore,
		RecordingPolicy: ccCore, CDR: ccCore, Recordings: ccCore,
		CallEvents: eventStore, Commands: commandStore,
		Bridges: store.Bridges{DB: db}, IVRSessions: store.IVRSessions{DB: db},
		Routing: routingStore,
	}
	callStore := store.NewCallStore(db, eventStore)
	controlDeps.Calls = callStore
	callControl := control.NewService(controlDeps)

	if err := callControl.Recover(context.Background()); err != nil {
		return fmt.Errorf("遗留通话恢复: %w", err)
	}
	if err := commandStore.ReconcileStale(context.Background(), 2*time.Minute); err != nil {
		return fmt.Errorf("异步命令恢复: %w", err)
	}
	if err := ccCore.RecoverReservations(context.Background()); err != nil {
		return fmt.Errorf("坐席预留恢复: %w", err)
	}
	mediaSvc.SetSIPHangupHandler(func(ctx context.Context, callID string) {
		_ = callControl.Hangup(ctx, callID, dto.HangupReasonNormal)
	})
	startInbound := func(ctx context.Context, trunkID, did, from, callID string) (string, string, error) {
		route, err := ccCore.ResolveDID(ctx, trunkID, did)
		if err != nil {
			return "", "", err
		}
		ctx = scope.WithConfigVersion(ctx, route.ConfigVersion)
		req := dto.InboundRequest{
			ConfigVersion: route.ConfigVersion,
			CallID:        callID, Caller: from, SessionType: dto.SessionTypeAudio,
		}
		switch route.TargetType {
		case "queue":
			req.QueueID = route.TargetID
		case "ivr":
			req.IVRFlowID = route.TargetID
		default:
			return "", "", fmt.Errorf("DID 路由目标类型无效: %s", route.TargetType)
		}
		id, err := callControl.StartInbound(ctx, dto.InboundRequest{
			ConfigVersion: req.ConfigVersion,
			CallID:        req.CallID, QueueID: req.QueueID, IVRFlowID: req.IVRFlowID,
			Caller: req.Caller, SessionType: req.SessionType,
		})
		if err != nil {
			return "", "", err
		}
		view, err := callControl.GetCall(ctx, id)
		if err != nil {
			return "", "", err
		}
		for _, leg := range view.Legs {
			if leg.Role == dto.LegRoleCustomer {
				return id, leg.ID, nil
			}
		}
		return id, "", nil
	}
	mediaSvc.SetSIPBindingStore(context.Background(), store.NewSIPBindingStore(db))
	mediaSvc.SetInboundHandler(startInbound)
	mediaSvc.SetDeviceHandler(func(ctx context.Context, _ string, destination, from, callID string) (string, string, error) {
		known := false
		for _, device := range cfg.SIP.Devices {
			if device.Username == from {
				known = true
				break
			}
		}
		if !known {
			return "", "", fmt.Errorf("未知 SIP 设备用户")
		}
		_, routeErr := ccCore.ResolveDID(ctx, "*", destination)
		if routeErr == nil {
			return startInbound(ctx, "*", destination, from, callID)
		}
		if !errors.Is(routeErr, errs.ErrNotFound) {
			return "", "", routeErr
		}
		return callControl.SIPSource(ctx, callID, from, destination)
	})

	log.Info("open-switch 以单实例模式启动（控制面/媒体不可双活共享库）", "listen", cfg.Server.Listen)

	deps := apphttp.RouterDeps{
		Config:      *cfg,
		CallControl: callControl,
		Signaling:   callControl,
	}
	switchDeps := apphttp.SwitchRouterDeps{
		Config: *cfg, RouterDeps: deps, Runtime: callControl, Events: &eventStore,
		Commands: &commandStore, Routing: &routingStore,
		Direct: callControl, Admin: ccCore, BusinessActions: callControl,
	}
	router := apphttp.NewSwitchRouter(switchDeps)

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitorLogDisk(ctx, log, cfg.Log.Dir)
	integratorDispatch.Start(ctx)
	go callControl.Run(ctx)
	go func() {
		if err := mediaSvc.ServeSIP(ctx); err != nil {
			log.Error("SIP 监听失败，交换服务退出", "err", err)
			os.Exit(1)
		}
	}()

	go func() {
		log.Info("open-switch 启动", "listen", cfg.Server.Listen)
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := callControl.Shutdown(shutdownCtx); err != nil {
		log.Warn("关停前挂断活跃通话失败", "err", err)
	}
	cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o750)
}
