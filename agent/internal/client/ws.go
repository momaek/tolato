package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/momaek/tolato/agent/internal/collector"
	"github.com/momaek/tolato/agent/internal/executor"
	"github.com/momaek/tolato/agent/internal/identity"
	"github.com/momaek/tolato/agent/internal/probe"
)

const (
	agentVersion      = "0.1.0"
	heartbeatInterval = 30 * time.Second
	initialBackoff    = 1 * time.Second
	maxBackoff        = 60 * time.Second
	writeWait         = 10 * time.Second
)

// ---- Wire protocol types ----

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// RegisterPayload is sent from agent to server on connect.
type RegisterPayload struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	IP            string `json:"ip"`
	AgentVersion  string `json:"agent_version"`
	CPUCores      int    `json:"cpu_cores"`
	MemoryTotalMB int    `json:"memory_total_mb"`
	DiskTotalGB   int    `json:"disk_total_gb"`
}

// HeartbeatPayload is the periodic metrics report.
type HeartbeatPayload struct {
	CPU     float64    `json:"cpu"`
	Memory  float64    `json:"memory"`
	Disk    float64    `json:"disk"`
	Uptime  int64      `json:"uptime"`
	LoadAvg [3]float64 `json:"load_avg"`
}

// CommandPayload is received from the server.
type CommandPayload struct {
	Action  string `json:"action"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// CommandResultPayload is sent back to the server.
type CommandResultPayload struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
}

// RegisterAckPayload is received from the server after registration.
type RegisterAckPayload struct {
	NodeID string `json:"node_id"`
	Secret string `json:"secret"`
}

// ---- Client ----

// Client manages the WebSocket connection to the server.
type Client struct {
	serverURL string
	token     string // one-time registration token

	identityStore *identity.Store
	ident         *identity.Identity

	collector      *collector.Collector
	executor       *executor.Executor
	probeScheduler *probe.Scheduler

	conn   *websocket.Conn
	connMu sync.Mutex // protects conn writes

	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

// NewClient creates a new WebSocket client.
func NewClient(serverURL, token string, store *identity.Store, ident *identity.Identity, col *collector.Collector, exec *executor.Executor) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		serverURL:     serverURL,
		token:         token,
		identityStore: store,
		ident:         ident,
		collector:     col,
		executor:      exec,
		done:          make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Run connects to the server and enters the reconnect loop.
// It blocks until the context is cancelled or Stop() is called.
func (c *Client) Run() {
	backoff := initialBackoff

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		err := c.connectAndServe()
		if err != nil {
			log.Printf("[ws] connection error: %v", err)
		}

		// Check if we are shutting down
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		log.Printf("[ws] reconnecting in %v ...", backoff)
		select {
		case <-time.After(backoff):
		case <-c.ctx.Done():
			return
		}

		// Exponential backoff
		backoff = time.Duration(math.Min(float64(backoff)*2, float64(maxBackoff)))
	}
}

// Stop gracefully shuts down the client.
func (c *Client) Stop() {
	c.cancel()
	c.connMu.Lock()
	if c.conn != nil {
		_ = c.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		_ = c.conn.Close()
	}
	c.connMu.Unlock()
}

// connectAndServe dials the server, registers, and runs read/heartbeat loops.
func (c *Client) connectAndServe() error {
	wsURL, err := c.buildURL()
	if err != nil {
		return fmt.Errorf("build URL: %w", err)
	}

	log.Printf("[ws] connecting to %s", wsURL)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	defer func() {
		c.connMu.Lock()
		_ = c.conn.Close()
		c.conn = nil
		c.connMu.Unlock()
	}()

	// Send register message
	if err := c.sendRegister(); err != nil {
		return fmt.Errorf("register: %w", err)
	}

	// Per-connection context for heartbeat goroutine
	connCtx, connCancel := context.WithCancel(c.ctx)
	defer connCancel()

	// Start heartbeat in background
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		c.heartbeatLoop(connCtx)
	}()

	// Read loop (blocks until error)
	err = c.readLoop()

	// Cancel heartbeat and wait for it
	connCancel()
	<-heartbeatDone

	return err
}

// buildURL constructs the WebSocket URL with appropriate query params.
func (c *Client) buildURL() (string, error) {
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return "", err
	}

	q := u.Query()
	if c.ident != nil {
		// Reconnect with identity
		q.Set("node_id", c.ident.NodeID)
		q.Set("secret", c.ident.Secret)
	} else if c.token != "" {
		// First connect with token
		q.Set("token", c.token)
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// sendRegister sends the register message with system info.
func (c *Client) sendRegister() error {
	sysInfo := c.collector.GetSystemInfo()

	payload := RegisterPayload{
		Hostname:      sysInfo.Hostname,
		OS:            sysInfo.OS,
		Kernel:        sysInfo.Kernel,
		IP:            sysInfo.IP,
		AgentVersion:  agentVersion,
		CPUCores:      sysInfo.CPUCores,
		MemoryTotalMB: sysInfo.MemoryTotalMB,
		DiskTotalGB:   sysInfo.DiskTotalGB,
	}

	return c.sendMessage("register", "", payload)
}

// sendMessage marshals and sends a WSMessage.
func (c *Client) sendMessage(msgType, id string, payload interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	msg := WSMessage{
		Type:    msgType,
		ID:      id,
		Payload: raw,
	}

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("connection closed")
	}

	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteJSON(msg)
}

// readLoop reads messages from the server until an error occurs.
func (c *Client) readLoop() error {
	for {
		var msg WSMessage
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		switch msg.Type {
		case "register_ack":
			c.handleRegisterAck(msg)
		case "command":
			go c.handleCommand(msg)
		case "probe_config":
			c.handleProbeConfig(msg)
		default:
			log.Printf("[ws] unknown message type: %s", msg.Type)
		}
	}
}

// handleRegisterAck processes the server's registration acknowledgment.
func (c *Client) handleRegisterAck(msg WSMessage) {
	var ack RegisterAckPayload
	if err := json.Unmarshal(msg.Payload, &ack); err != nil {
		log.Printf("[ws] failed to parse register_ack: %v", err)
		return
	}

	sysInfo := c.collector.GetSystemInfo()
	c.ident = &identity.Identity{
		NodeID:   ack.NodeID,
		Secret:   ack.Secret,
		Hostname: sysInfo.Hostname,
		OS:       sysInfo.OS,
		Version:  agentVersion,
	}

	if err := c.identityStore.Save(c.ident); err != nil {
		log.Printf("[ws] failed to save identity: %v", err)
	} else {
		log.Printf("[ws] registered as node_id=%s, identity saved to %s", ack.NodeID, c.identityStore.Path())
	}
}

// handleCommand executes a command received from the server.
func (c *Client) handleCommand(msg WSMessage) {
	var cmd CommandPayload
	if err := json.Unmarshal(msg.Payload, &cmd); err != nil {
		log.Printf("[ws] failed to parse command: %v", err)
		return
	}

	if cmd.Action != "execute_command" {
		log.Printf("[ws] unknown command action: %s", cmd.Action)
		return
	}

	log.Printf("[ws] executing command (id=%s, timeout=%ds): %s", msg.ID, cmd.Timeout, cmd.Command)

	result := c.executor.Execute(context.Background(), cmd.Command, cmd.Timeout)

	payload := CommandResultPayload{
		ExitCode:   result.ExitCode,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		DurationMS: result.DurationMS,
	}

	if err := c.sendMessage("command_result", msg.ID, payload); err != nil {
		log.Printf("[ws] failed to send command_result: %v", err)
	}
}

// handleProbeConfig processes probe configuration from the server.
func (c *Client) handleProbeConfig(msg WSMessage) {
	var config probe.ProbeConfig
	if err := json.Unmarshal(msg.Payload, &config); err != nil {
		log.Printf("[ws] failed to parse probe_config: %v", err)
		return
	}

	log.Printf("[ws] received probe_config: enabled=%v, targets=%d", config.Enabled, len(config.Targets))

	if c.probeScheduler == nil {
		nodeID := ""
		if c.ident != nil {
			nodeID = c.ident.NodeID
		}
		c.probeScheduler = probe.NewScheduler(nodeID)
	}

	c.probeScheduler.UpdateConfig(config)
}

// heartbeatLoop sends periodic heartbeat messages.
func (c *Client) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := c.collector.GetMetrics()
			payload := HeartbeatPayload{
				CPU:     metrics.CPU,
				Memory:  metrics.Memory,
				Disk:    metrics.Disk,
				Uptime:  metrics.Uptime,
				LoadAvg: metrics.LoadAvg,
			}
			if err := c.sendMessage("heartbeat", "", payload); err != nil {
				log.Printf("[ws] heartbeat send failed: %v", err)
				return
			}
		}
	}
}
