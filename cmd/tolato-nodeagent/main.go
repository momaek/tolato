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
	flag.Parse()

	if *nodeID == "" {
		log.Fatal("-node-id is required")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	runner := &nodeagent.Runner{
		URL:               *serverURL,
		NodeID:            *nodeID,
		Region:            *region,
		Tags:              splitCSV(*tags),
		AgentVersion:      *agentVersion,
		AuthToken:         *authToken,
		HeartbeatInterval: *heartbeat,
		ReconnectDelay:    *reconnect,
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
