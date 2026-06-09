package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/geoip"
	"github.com/momaek/tolato/server/internal/handler"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/notify"
	"github.com/momaek/tolato/server/internal/settings"
	"github.com/momaek/tolato/server/internal/store"
)

// version is injected at build time via -ldflags "-X main.version=<tag>"
// (see server/Dockerfile). Defaults to "dev" for local `go run`/`go build`.
var version = "dev"

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
	settingsCache := settings.New()
	notifier := notify.New(settingsCache)

	var geoSvc *geoip.Service
	if cfg.GeoIP.Enabled {
		var err error
		geoSvc, err = geoip.New(cfg.GeoIP.DataDir)
		if err != nil {
			log.Printf("GeoIP service init failed (continuing without): %v", err)
		}
	}

	deps := &handler.Deps{
		Config:      cfg,
		NodeManager: nm,
		Settings:    settingsCache,
		GeoIP:       geoSvc,
		Notifier:    notifier,
		Version:     version,
	}

	handler.InitUpgraders(cfg.Server.AllowedOrigins)
	r := handler.SetupRouter(deps)

	// Root shutdown context — cancelled on SIGINT/SIGTERM. Connection handlers
	// that want to bail on shutdown should honor it.
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	if geoSvc != nil {
		go runGeoIPBackfill(rootCtx, geoSvc)
		if cfg.GeoIP.RefreshInterval > 0 {
			go runGeoIPRefresh(rootCtx, geoSvc, cfg.GeoIP.RefreshInterval)
		}
	}

	go runOfflineMonitor(rootCtx, settingsCache, notifier)

	<-rootCtx.Done()
	log.Println("Shutdown signal received, draining...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	if geoSvc != nil {
		geoSvc.Close()
	}

	log.Println("Bye")
}

// runGeoIPRefresh re-downloads the .mmdb files on the configured interval.
func runGeoIPRefresh(ctx context.Context, svc *geoip.Service, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			svc.Refresh()
		}
	}
}

// runOfflineMonitor periodically marks nodes offline when their heartbeat goes
// stale, catching agents that vanished without a clean WebSocket close
// (kill -9, network partition, host crash). The conditional UPDATE in
// store.MarkOffline guarantees each offline notification fires exactly once even
// if the WS-disconnect path races this loop.
func runOfflineMonitor(ctx context.Context, s *settings.Cache, n *notify.Dispatcher) {
	const tick = 30 * time.Second
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg := s.Notify()
			if cfg.OfflineThresholdSeconds <= 0 {
				continue // monitor disabled
			}
			threshold := time.Duration(cfg.OfflineThresholdSeconds) * time.Second
			nodes, err := store.ListStaleOnlineNodes(threshold)
			if err != nil {
				log.Printf("offline monitor: list failed: %v", err)
				continue
			}
			for _, nd := range nodes {
				if changed, _ := store.MarkOffline(nd.ID); changed {
					log.Printf("offline monitor: node %s went stale (no heartbeat > %s)", nd.ID, threshold)
					n.NotifyOffline(nd.ID)
				}
			}
		}
	}
}

// runGeoIPBackfill fills in country_code/city/asn for nodes that registered
// before the geoip service was available.
func runGeoIPBackfill(ctx context.Context, svc *geoip.Service) {
	nodes, err := store.ListNodesMissingGeo()
	if err != nil {
		log.Printf("GeoIP backfill: list failed: %v", err)
		return
	}
	if len(nodes) == 0 {
		return
	}
	log.Printf("GeoIP backfill: resolving %d nodes", len(nodes))
	for _, n := range nodes {
		if ctx.Err() != nil {
			return
		}
		geo, _ := svc.Lookup(n.IP)
		if geo.IsZero() {
			continue
		}
		_ = store.UpdateNode(n.ID, map[string]any{
			"country_code": geo.CountryCode,
			"city":         geo.City,
			"asn":          geo.ASN,
		})
	}
}
