package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/handler"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/probe"
	"github.com/momaek/tolato/server/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set JWT secret
	middleware.JWTSecret = cfg.Security.JWTSecret

	// Initialize database
	if err := store.InitDB(cfg.Database.DSN); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create NodeManager
	nm := node.NewNodeManager()

	// Create SessionManager
	sm := handler.NewSessionManager()

	// Setup dependencies
	deps := &handler.Deps{
		Config:         cfg,
		NodeManager:    nm,
		SessionManager: sm,
	}

	// Setup router
	r := handler.SetupRouter(deps)

	// Setup probe routes if enabled
	if cfg.Probe.Enabled {
		probeStore := probe.NewStore(store.DB)
		alertCfg := probe.AlertConfig{
			LatencyThresholdMS:       cfg.Probe.AlertRules.LatencyThresholdMS,
			PacketLossThresholdPct:   cfg.Probe.AlertRules.PacketLossThresholdPct,
			TCPConnectThresholdMS:    cfg.Probe.AlertRules.TCPConnectThresholdMS,
			BandwidthThresholdMbps:   cfg.Probe.AlertRules.BandwidthThresholdMbps,
			OfflineTimeoutSeconds:    cfg.Probe.AlertRules.OfflineTimeoutSeconds,
			RecoveryCount:            cfg.Probe.AlertRules.RecoveryCount,
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

		// Start probe data cleanup goroutine
		retentionDays := cfg.Probe.RetentionDays
		if retentionDays <= 0 {
			retentionDays = 30
		}
		go probe.StartCleanupScheduler(probeStore, retentionDays)

		log.Println("NodeProbe module enabled")
	}

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting tolato server on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
