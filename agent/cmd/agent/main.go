package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/momaek/tolato/agent/internal/attestation"
	"github.com/momaek/tolato/agent/internal/client"
	"github.com/momaek/tolato/agent/internal/collector"
	"github.com/momaek/tolato/agent/internal/executor"
	"github.com/momaek/tolato/agent/internal/identity"
	"github.com/momaek/tolato/agent/internal/nodeexporter"
	"github.com/momaek/tolato/agent/internal/services"
)

func main() {
	var (
		serverURL   string
		token       string
		dataDir     string
		showVersion bool
	)

	enabled := kingpin.Flag("node-exporter.enabled", "Enable embedded node_exporter collection over the server WebSocket.").Default("false").Bool()
	attestationSocket := kingpin.Flag("attestation.socket", "Root-only Unix socket for local machine proofs (disabled when empty).").String()
	managedServices := kingpin.Flag("managed-services.enabled", "Supervise patato-node and allow audited service management (Linux root only).").Default("false").Bool()
	interval := kingpin.Flag("node-exporter.interval", "Metrics collection interval (minimum 5s).").Default("30s").Duration()
	timeout := kingpin.Flag("node-exporter.timeout", "Collection deadline; timed-out collectors cannot overlap subsequent runs.").Default("10s").Duration()
	kingpin.Flag("server", "Server WebSocket URL (required, e.g. ws://localhost:8080/ws/agent)").StringVar(&serverURL)
	kingpin.Flag("token", "One-time registration token (required for first run)").StringVar(&token)
	kingpin.Flag("data-dir", "Data directory for identity storage (default: ~/.tolato)").StringVar(&dataDir)
	kingpin.Flag("version", "Print the agent version and exit").BoolVar(&showVersion)
	kingpin.CommandLine.HelpFlag.Short('h')
	kingpin.Parse()

	// --version prints just the version string; the self-update flow relies on
	// this to verify a freshly downloaded binary before swapping it in.
	if showVersion {
		fmt.Println(client.Version)
		return
	}

	if serverURL == "" {
		fmt.Fprintln(os.Stderr, "error: --server is required")
		kingpin.Usage()
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
			kingpin.Usage()
			os.Exit(1)
		}
		log.Println("no existing identity, will register with token")
	}

	// Create components
	col := collector.NewCollector()
	exec := executor.NewExecutor()
	wsClient := client.NewClient(serverURL, token, store, ident, col, exec)
	if *managedServices {
		manager, err := services.New(filepath.Join(dataDir, "services", services.Name))
		if err != nil {
			log.Fatalf("initialize managed services: %v", err)
		}
		defer manager.Close()
		wsClient.EnableServices(manager)
	}
	if *attestationSocket != "" {
		listener, err := attestation.Listen(*attestationSocket)
		if err != nil {
			log.Fatalf("initialize attestation socket: %v", err)
		}
		server := &http.Server{Handler: attestation.Handler(store.Load), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
		defer server.Close()
		go func() {
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				log.Printf("attestation socket: %v", err)
				wsClient.Stop()
			}
		}()
	}

	if *enabled {
		if *interval < 5*time.Second || *interval > time.Hour || *timeout <= 0 || *timeout > *interval {
			log.Fatal("node-exporter.interval must be between 5s and 1h; timeout must be positive and <= interval")
		}
		exporter, err := nodeexporter.New(slog.Default())
		if err != nil {
			log.Fatalf("initialize node_exporter: %v", err)
		}
		wsClient.EnableNodeExporter(exporter, *interval, *timeout)
	}

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
