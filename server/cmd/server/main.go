package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/handler"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/node"
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

	// Setup dependencies
	deps := &handler.Deps{
		Config:      cfg,
		NodeManager: nm,
	}

	// Setup router
	r := handler.SetupRouter(deps)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting tolato server on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
