package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/momaek/tolato/agent/internal/client"
	"github.com/momaek/tolato/agent/internal/collector"
	"github.com/momaek/tolato/agent/internal/executor"
	"github.com/momaek/tolato/agent/internal/identity"
	"github.com/momaek/tolato/agent/internal/probe"
)

func main() {
	// Handle serve-testfile subcommand
	if len(os.Args) > 1 && os.Args[1] == "serve-testfile" {
		serveCmd := flag.NewFlagSet("serve-testfile", flag.ExitOnError)
		port := serveCmd.Int("port", 9090, "Port to serve on")
		size := serveCmd.Int("size", 10, "Test file size in MB")
		serveCmd.Parse(os.Args[2:])
		if err := probe.ServeTestFile(*port, *size); err != nil {
			log.Fatalf("serve-testfile failed: %v", err)
		}
		return
	}

	var (
		serverURL string
		token     string
		dataDir   string
	)

	flag.StringVar(&serverURL, "server", "", "Server WebSocket URL (required, e.g. ws://localhost:8080/ws/agent)")
	flag.StringVar(&token, "token", "", "One-time registration token (required for first run)")
	flag.StringVar(&dataDir, "data-dir", "", "Data directory for identity storage (default: ~/.tolato)")
	flag.Parse()

	if serverURL == "" {
		fmt.Fprintln(os.Stderr, "error: --server is required")
		flag.Usage()
		os.Exit(1)
	}

	// Resolve data directory
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("failed to get home directory: %v", err)
		}
		dataDir = filepath.Join(home, ".tolato")
	}

	// Load or create identity
	store := identity.NewStore(dataDir)
	ident, err := store.Load()
	if err != nil {
		log.Fatalf("failed to load identity: %v", err)
	}

	if ident != nil {
		log.Printf("loaded identity: node_id=%s", ident.NodeID)
	} else {
		if token == "" {
			fmt.Fprintln(os.Stderr, "error: --token is required for first run (no existing identity found)")
			flag.Usage()
			os.Exit(1)
		}
		log.Println("no existing identity, will register with token")
	}

	// Create components
	col := collector.NewCollector()
	exec := executor.NewExecutor()
	wsClient := client.NewClient(serverURL, token, store, ident, col, exec)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("received signal %s, shutting down...", sig)
		wsClient.Stop()
	}()

	log.Printf("tolato agent starting (server=%s, data-dir=%s)", serverURL, dataDir)
	wsClient.Run()
	log.Println("tolato agent stopped")
}
