package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/handler"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/probe"
	"github.com/momaek/tolato/server/internal/settings"
	"github.com/momaek/tolato/server/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	middleware.JWTSecret = cfg.Security.JWTSecret

	if err := store.InitDB(cfg.Database.DSN); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	nm := node.NewNodeManager()
	sm := handler.NewSessionManager()
	settingsCache := settings.New()

	deps := &handler.Deps{
		Config:         cfg,
		NodeManager:    nm,
		SessionManager: sm,
		Settings:       settingsCache,
	}

	handler.InitUpgraders(cfg.Server.AllowedOrigins)
	r := handler.SetupRouter(deps)

	// Root shutdown context — cancelled on SIGINT/SIGTERM. Everything spawned
	// from main (background goroutines, connection handlers that want to bail
	// on shutdown) should honor it.
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var bg sync.WaitGroup

	if cfg.Probe.Enabled {
		probeStore := probe.NewStore(store.DB)
		deps.Probe = probeStore
		alertCfg := probe.AlertConfig{
			LatencyThresholdMS:     cfg.Probe.AlertRules.LatencyThresholdMS,
			PacketLossThresholdPct: cfg.Probe.AlertRules.PacketLossThresholdPct,
			TCPConnectThresholdMS:  cfg.Probe.AlertRules.TCPConnectThresholdMS,
			BandwidthThresholdMbps: cfg.Probe.AlertRules.BandwidthThresholdMbps,
			OfflineTimeoutSeconds:  cfg.Probe.AlertRules.OfflineTimeoutSeconds,
			RecoveryCount:          cfg.Probe.AlertRules.RecoveryCount,
		}
		if alertCfg.RecoveryCount == 0 {
			alertCfg = probe.DefaultAlertConfig()
		}
		var notifier probe.Notifier
		if tn := probe.NewTelegramNotifier(cfg.Probe.Telegram.BotToken, cfg.Probe.Telegram.ChatID); tn != nil {
			notifier = tn
		}
		alertEngine := probe.NewAlertEngine(probeStore, alertCfg, notifier)
		handler.SetupProbeRoutes(r, deps, probeStore, alertEngine)

		retentionDays := cfg.Probe.RetentionDays
		if retentionDays <= 0 {
			retentionDays = 30
		}
		bg.Add(1)
		go func() {
			defer bg.Done()
			probe.StartCleanupScheduler(rootCtx, probeStore, retentionDays)
		}()

		log.Println("NodeProbe module enabled")
	}

	// Custom http.Server with sane timeouts. WebSocket handlers override the
	// read/write deadlines on their own connections, so the global timeouts are
	// only a slow-loris guard for normal HTTP requests.
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("Starting tolato server on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-rootCtx.Done()
	log.Println("Shutdown signal received, draining...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Wait for background goroutines (probe scheduler etc.) to exit.
	done := make(chan struct{})
	go func() { bg.Wait(); close(done) }()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		log.Println("Background goroutines did not exit in time")
	}

	log.Println("Bye")
}
