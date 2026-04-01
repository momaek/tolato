package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/momaek/tolato/internal/nodeagent"
	"github.com/momaek/tolato/internal/nodeagent/probe"
)

func main() {
	serverURL := flag.String("server", "ws://127.0.0.1:8080/ws/agent", "ws/agent websocket endpoint")
	nodeID := flag.String("node-id", "", "unique node identifier")
	region := flag.String("region", "", "node region label")
	tags := flag.String("tags", "", "comma-separated node tags")
	agentVersion := flag.String("agent-version", "nodeagent-dev", "node agent version")
	authToken := flag.String("auth-token", "", "bearer token for ws/agent authentication")
	heartbeat := flag.Duration("heartbeat", 5*time.Second, "heartbeat interval")
	reconnect := flag.Duration("reconnect", 2*time.Second, "reconnect delay")
	timeout := flag.Duration("timeout", 20*time.Second, "per-dispatch execution timeout")
	maxConcurrent := flag.Int("max-concurrent", 10, "maximum concurrent dispatch executions")

	// Probe flags
	probeConfig := flag.String("probe-config", "", "path to probe YAML config (enables probe mode)")
	serveTestfile := flag.String("serve-testfile", "", "address to serve bandwidth test file (e.g. :9090)")
	testfileSizeMB := flag.Int("testfile-size-mb", 10, "size of bandwidth test file in MB")

	flag.Parse()

	logger := log.New(os.Stdout, "[nodeagent] ", log.LstdFlags)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Standalone testfile server mode
	if *serveTestfile != "" {
		probeLogger := log.New(os.Stdout, "[testfile] ", log.LstdFlags)
		if err := probe.ServeTestFile(ctx, *serveTestfile, *testfileSizeMB, probeLogger); err != nil {
			log.Fatalf("testfile server: %v", err)
		}
		return
	}

	if *nodeID == "" {
		log.Fatal("-node-id is required")
	}

	// Start probe if configured
	if *probeConfig != "" {
		cfg, err := probe.LoadProbeConfig(*probeConfig)
		if err != nil {
			log.Fatalf("load probe config: %v", err)
		}
		// Override node_id from flag if not set in probe config
		if cfg.NodeID == "" {
			cfg.NodeID = *nodeID
		}

		probeLogger := log.New(os.Stdout, "[probe] ", log.LstdFlags)
		p := &probe.Probe{
			Config: cfg,
			Logger: probeLogger,
		}

		probeCtx, probeCancel := context.WithCancel(ctx)
		defer probeCancel()

		go func() {
			if err := p.Run(probeCtx); err != nil && probeCtx.Err() == nil {
				logger.Printf("probe stopped: %v", err)
			}
		}()
	}

	runner := &nodeagent.Runner{
		URL:               *serverURL,
		NodeID:            *nodeID,
		Region:            *region,
		Tags:              splitCSV(*tags),
		AgentVersion:      *agentVersion,
		AuthToken:         *authToken,
		HeartbeatInterval: *heartbeat,
		ReconnectDelay:    *reconnect,
		MaxConcurrent:     *maxConcurrent,
		Executor: &nodeagent.LocalExecutor{
			NodeID:  *nodeID,
			Timeout: *timeout,
		},
	}
	if err := runner.Run(ctx); err != nil {
		log.Fatalf("nodeagent stopped: %v", err)
	}
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
