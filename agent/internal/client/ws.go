package client

import (
	"context"
	"encoding/base64"
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
	"github.com/momaek/tolato/agent/internal/files"
	"github.com/momaek/tolato/agent/internal/identity"
	"github.com/momaek/tolato/agent/internal/terminal"
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

// --- PTY / File op payloads (agent side) ---

type PTYOpenPayload struct {
	Cols  uint16 `json:"cols"`
	Rows  uint16 `json:"rows"`
	Shell string `json:"shell,omitempty"`
	Cwd   string `json:"cwd,omitempty"`
}

type PTYInputPayload struct {
	Data string `json:"data"` // base64
}

type PTYResizePayload struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type PTYOutputPayload struct {
	Data string `json:"data"` // base64
}

type PTYExitPayload struct {
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

type FileOpPayload struct {
	Op     string `json:"op"`
	Path   string `json:"path"`
	Data   string `json:"data,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Length int64  `json:"length,omitempty"`
}

// ---- Client ----

// Client manages the WebSocket connection to the server.
type Client struct {
	serverURL string
	token     string // one-time registration token

	identityStore *identity.Store
	ident         *identity.Identity

	collector *collector.Collector
	executor  *executor.Executor

	conn   *websocket.Conn
	connMu sync.Mutex // protects conn writes

	ptyMu       sync.Mutex
	ptySessions map[string]*terminal.Session // streamID -> session

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
		ptySessions:   make(map[string]*terminal.Session),
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
		case "pty_open":
			go c.handlePTYOpen(msg)
		case "pty_input":
			c.handlePTYInput(msg)
		case "pty_resize":
			c.handlePTYResize(msg)
		case "pty_close":
			c.handlePTYClose(msg)
		case "file_op":
			go c.handleFileOp(msg)
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

// ============================================================================
// PTY + file op handlers
// ============================================================================

func (c *Client) handlePTYOpen(msg WSMessage) {
	var p PTYOpenPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		log.Printf("[ws] pty_open parse: %v", err)
		return
	}

	sess, err := terminal.Start(p.Shell, p.Cwd, p.Cols, p.Rows)
	if err != nil {
		log.Printf("[ws] pty_open start: %v", err)
		_ = c.sendMessage("pty_exit", msg.ID, PTYExitPayload{ExitCode: -1, Error: err.Error()})
		return
	}

	c.ptyMu.Lock()
	// If a session already exists for this stream ID, kill it first.
	if old, ok := c.ptySessions[msg.ID]; ok {
		old.Close()
	}
	c.ptySessions[msg.ID] = sess
	c.ptyMu.Unlock()

	streamID := msg.ID

	// Output pump: read PTY bytes and emit pty_output frames to the server.
	go func() {
		for chunk := range sess.Output() {
			payload := PTYOutputPayload{Data: base64.StdEncoding.EncodeToString(chunk)}
			if err := c.sendMessage("pty_output", streamID, payload); err != nil {
				log.Printf("[ws] pty_output send: %v", err)
				return
			}
		}
		// Output channel closed → session ended.
		<-sess.Closed()

		c.ptyMu.Lock()
		delete(c.ptySessions, streamID)
		c.ptyMu.Unlock()

		exitErr := ""
		if e := sess.ExitError(); e != nil {
			exitErr = e.Error()
		}
		_ = c.sendMessage("pty_exit", streamID, PTYExitPayload{
			ExitCode: sess.ExitCode(),
			Error:    exitErr,
		})
	}()
}

func (c *Client) handlePTYInput(msg WSMessage) {
	var p PTYInputPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return
	}
	c.ptyMu.Lock()
	sess := c.ptySessions[msg.ID]
	c.ptyMu.Unlock()
	if sess == nil {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return
	}
	_, _ = sess.Write(raw)
}

func (c *Client) handlePTYResize(msg WSMessage) {
	var p PTYResizePayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return
	}
	c.ptyMu.Lock()
	sess := c.ptySessions[msg.ID]
	c.ptyMu.Unlock()
	if sess == nil {
		return
	}
	_ = sess.Resize(p.Cols, p.Rows)
}

func (c *Client) handlePTYClose(msg WSMessage) {
	c.ptyMu.Lock()
	sess := c.ptySessions[msg.ID]
	delete(c.ptySessions, msg.ID)
	c.ptyMu.Unlock()
	if sess != nil {
		sess.Close()
	}
}

func (c *Client) handleFileOp(msg WSMessage) {
	var p FileOpPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		_ = c.sendMessage("file_result", msg.ID, files.Response{OK: false, Error: "invalid payload"})
		return
	}
	resp := files.Handle(files.Request{
		Op:     p.Op,
		Path:   p.Path,
		Data:   p.Data,
		Mode:   p.Mode,
		Offset: p.Offset,
		Length: p.Length,
	})
	_ = c.sendMessage("file_result", msg.ID, resp)
}
