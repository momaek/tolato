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
	"github.com/momaek/tolato/server/internal/handler"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/node"
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

	<-rootCtx.Done()
	log.Println("Shutdown signal received, draining...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Bye")
}
