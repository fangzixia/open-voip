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
	if err := migrate.Migrate(db); err != nil {
		return fmt.Errorf("数据库迁移: %w", err)
	}
	if err := store.Ping(db); err != nil {
		return fmt.Errorf("数据库 Ping: %w", err)
	}

	mediaSvc, err := media.NewService(media.Options{
		ICE: cfg.ICE, TURN: cfg.TURN, RecordingsDir: cfg.Recordings.Dir,
		VideoFormat: cfg.Recordings.VideoFormat, FFmpegPath: cfg.Recordings.FFmpegPath, SIP: cfg.SIP,
	})
	if err != nil {
		return fmt.Errorf("媒体层: %w", err)
	}

	eventStore := store.CallEvents{DB: db}
	ccCore := cccore.New(db, eventStore, cccore.Options{
		RecordingMode: "audio",
		NotifyMessage: cfg.Recordings.NotifyMessage,
		RetainDays:    cfg.Recordings.RetainDays,
	})
	controlDeps := control.Deps{
		BusinessActions: ccCore, Media: mediaSvc, ACD: ccCore, Config: ccCore, Agents: ccCore,
		RecordingPolicy: ccCore, CDR: ccCore, Recordings: ccCore,
		CallEvents: eventStore,
	}
	callStore := store.NewCallStore(db)
	limits := map[string]int{}
	for _, application := range cfg.Applications {
		limits[application.ID] = application.MaxConcurrentCalls
	}
	callStore.SetApplicationLimits(limits)
	controlDeps.Calls = callStore
	callControl := control.NewService(controlDeps)

	if err := callControl.Recover(context.Background()); err != nil {
		return fmt.Errorf("遗留通话恢复: %w", err)
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
		ctx = scope.WithApplication(ctx, route.ApplicationID)
		ctx = scope.WithConfigVersion(ctx, route.ConfigVersion)
		req := dto.InboundRequest{
			ApplicationID: route.ApplicationID, ConfigVersion: route.ConfigVersion,
			CallID: callID, Caller: from, SessionType: dto.SessionTypeAudio,
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
			ApplicationID: req.ApplicationID, ConfigVersion: req.ConfigVersion,
			CallID: req.CallID, QueueID: req.QueueID, IVRFlowID: req.IVRFlowID,
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
	mediaSvc.SetInboundHandler(startInbound)
	mediaSvc.SetDeviceHandler(func(ctx context.Context, _ string, destination, from, callID string) (string, string, error) {
		appID := ""
		for _, device := range cfg.SIP.Devices {
			if device.Username == from {
				appID = device.ApplicationID
				break
			}
		}
		if appID == "" {
			return "", "", errs.Forbidden("设备缺少应用绑定")
		}
		ctx = scope.WithApplication(ctx, appID)
		_, routeErr := ccCore.ResolveDID(ctx, "*", destination)
		if routeErr == nil {
			return startInbound(ctx, "*", destination, from, callID)
		}
		if !errors.Is(routeErr, errs.ErrNotFound) {
			return "", "", routeErr
		}
		return callControl.SIPSource(ctx, callID, from, destination)
	})

	deps := apphttp.RouterDeps{
		Config:      *cfg,
		CallControl: callControl,
		Signaling:   callControl,
	}
	switchDeps := apphttp.SwitchRouterDeps{Config: *cfg, RouterDeps: deps, Runtime: callControl, Events: &eventStore, Direct: callControl, Admin: ccCore, BusinessActions: callControl}
	router := apphttp.NewSwitchRouter(switchDeps)

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitorLogDisk(ctx, log, cfg.Log.Dir)
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
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return callControl.Shutdown(shutdownCtx)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o750)
}
