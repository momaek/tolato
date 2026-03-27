package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
	"github.com/momaek/tolato/internal/server/transport/wsagent"
)

type Runner struct {
	URL               string
	NodeID            string
	Region            string
	Tags              []string
	AgentVersion      string
	AuthToken         string
	HeartbeatInterval time.Duration
	ReconnectDelay    time.Duration
	MaxConcurrent     int
	Dialer            *websocket.Dialer
	Logger            *log.Logger
	Executor          Executor
}

func (r *Runner) Run(ctx context.Context) error {
	if strings.TrimSpace(r.URL) == "" || strings.TrimSpace(r.NodeID) == "" {
		return errors.New("url and node id are required")
	}

	failures := 0
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := r.runSession(ctx); err != nil {
			if errors.Is(err, errUpgradeExit) {
				r.logger().Printf("nodeagent exiting after upgrade")
				os.Exit(0)
			}
			if ctx.Err() == nil {
				failures++
				r.logger().Printf("nodeagent reconnecting after error: %v", err)
			}
		} else {
			failures = 0
		}

		delay := r.backoffDelay(failures)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

// errUpgradeExit is a sentinel error indicating the agent should exit
// after a successful binary upgrade so that systemd can restart it.
var errUpgradeExit = errors.New("agent upgrade completed, exiting for restart")

const maxReconnectDelay = 60 * time.Second

func (r *Runner) backoffDelay(failures int) time.Duration {
	base := r.reconnectDelay()
	if failures <= 0 {
		return base
	}
	shift := failures
	if shift > 5 {
		shift = 5 // cap at 2^5 = 32x
	}
	delay := base * (1 << shift)
	if delay > maxReconnectDelay {
		delay = maxReconnectDelay
	}
	return delay
}

func (r *Runner) runSession(ctx context.Context) error {
	headers := http.Header{}
	if token := strings.TrimSpace(r.AuthToken); token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	conn, _, err := r.dialer().DialContext(ctx, r.URL, headers)
	if err != nil {
		return err
	}
	defer conn.Close()

	r.logger().Printf("nodeagent connected to %s as %s", r.URL, r.NodeID)
	session := &session{
		nodeID:    r.NodeID,
		metadata:  r.metadata(),
		conn:      conn,
		logger:    r.logger(),
		executor:  r.executor(),
		active:    make(map[string]struct{}),
		sem:       make(chan struct{}, r.maxConcurrent()),
		shells:    make(map[string]*ShellSession),
		upgradeCh: make(chan struct{}),
	}
	if err := session.sendRegister(); err != nil {
		return err
	}
	if err := session.sendHeartbeat(); err != nil {
		return err
	}

	readErr := make(chan error, 1)
	go func() {
		readErr <- session.readLoop(ctx)
	}()

	ticker := time.NewTicker(r.heartbeatInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			session.gracefulShutdown()
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutting down"), time.Now().Add(time.Second))
			return nil
		case <-ticker.C:
			if err := session.sendHeartbeat(); err != nil {
				return err
			}
		case err := <-readErr:
			return err
		case <-session.upgradeCh:
			session.gracefulShutdown()
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "upgrade"), time.Now().Add(time.Second))
			return errUpgradeExit
		}
	}
}

func (r *Runner) dialer() *websocket.Dialer {
	if r.Dialer != nil {
		return r.Dialer
	}
	return websocket.DefaultDialer
}

func (r *Runner) logger() *log.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return log.Default()
}

func (r *Runner) executor() Executor {
	if r.Executor != nil {
		return r.Executor
	}
	return &LocalExecutor{
		NodeID:  r.NodeID,
		Timeout: 20 * time.Second,
	}
}

func (r *Runner) heartbeatInterval() time.Duration {
	if r.HeartbeatInterval > 0 {
		return r.HeartbeatInterval
	}
	return 5 * time.Second
}

func (r *Runner) reconnectDelay() time.Duration {
	if r.ReconnectDelay > 0 {
		return r.ReconnectDelay
	}
	return 2 * time.Second
}

func (r *Runner) maxConcurrent() int {
	if r.MaxConcurrent > 0 {
		return r.MaxConcurrent
	}
	return 10
}

type session struct {
	nodeID   string
	metadata infraws.AgentNodeMetadata
	conn     *websocket.Conn
	logger   *log.Logger
	executor Executor

	writeMu  sync.Mutex
	activeMu sync.Mutex
	active   map[string]struct{}
	wg       sync.WaitGroup
	sem      chan struct{} // concurrency limiter

	shellsMu sync.RWMutex
	shells   map[string]*ShellSession // executionID -> ShellSession

	upgradeOnce sync.Once
	upgradeCh   chan struct{} // closed when upgrade completes and agent should exit
}

func (s *session) readLoop(ctx context.Context) error {
	for {
		_, raw, err := s.conn.ReadMessage()
		if err != nil {
			return err
		}

		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			s.logger.Printf("nodeagent received malformed message: %v", err)
			continue
		}

		switch envelope.Type {
		case wsagent.TypeTaskDispatch:
			var cmd appexecution.DispatchCommand
			if err := json.Unmarshal(raw, &cmd); err != nil {
				s.logger.Printf("nodeagent failed to decode dispatch: %v", err)
				continue
			}
			select {
			case s.sem <- struct{}{}:
				s.wg.Add(1)
				go func() {
					defer func() { <-s.sem; s.wg.Done() }()
					s.handleDispatch(ctx, cmd)
				}()
			default:
				s.logger.Printf("nodeagent rejected dispatch execution=%s: concurrency limit reached", cmd.ExecutionID)
				s.finishExecution(cmd, failureResult(75, "agent concurrency limit reached"))
			}
		case wsagent.TypeShellInput:
			var msg wsagent.Message
			if err := json.Unmarshal(raw, &msg); err == nil {
				var payload wsagent.ShellInputPayload
				if err := json.Unmarshal(msg.Payload, &payload); err == nil {
					s.handleShellInput(payload)
				}
			}
		case wsagent.TypeShellResize:
			var msg wsagent.Message
			if err := json.Unmarshal(raw, &msg); err == nil {
				var payload wsagent.ShellResizePayload
				if err := json.Unmarshal(msg.Payload, &payload); err == nil {
					s.handleShellResize(payload)
				}
			}
		case wsagent.TypeAgentAck:
			var ack wsagent.Ack
			if err := json.Unmarshal(raw, &ack); err == nil {
				s.logger.Printf("nodeagent ack type=%s node=%s task=%s", ack.Type, ack.NodeID, ack.TaskID)
			}
		case wsagent.TypeAgentError:
			var msg wsagent.Error
			if err := json.Unmarshal(raw, &msg); err == nil {
				s.logger.Printf("nodeagent server error code=%s message=%s", msg.Code, msg.Message)
			}
		default:
			s.logger.Printf("nodeagent ignored message type=%s", envelope.Type)
		}
	}
}

func (s *session) handleDispatch(ctx context.Context, cmd appexecution.DispatchCommand) {
	if cmd.ExecutionID == "" || cmd.TaskID == "" || cmd.SessionID == "" || cmd.NodeID == "" {
		s.logger.Printf("nodeagent ignored invalid dispatch: %+v", cmd)
		return
	}
	if cmd.NodeID != s.nodeID {
		s.logger.Printf("nodeagent rejected dispatch for node %s on %s", cmd.NodeID, s.nodeID)
		result := failureResult(64, "dispatch node mismatch")
		s.finishExecution(cmd, result)
		return
	}

	if cmd.Action == "open_shell" {
		s.handleShellDispatch(ctx, cmd)
		return
	}

	if cmd.Action == "upgrade_agent" {
		s.handleUpgradeDispatch(ctx, cmd)
		return
	}

	if !s.beginExecution(cmd.ExecutionID) {
		s.logger.Printf("nodeagent ignored duplicate dispatch execution=%s", cmd.ExecutionID)
		return
	}
	defer s.endExecution(cmd.ExecutionID)

	s.logger.Printf("nodeagent executing task=%s execution=%s action=%s", cmd.TaskID, cmd.ExecutionID, cmd.Action)
	result := s.executor.Execute(ctx, cmd, chunkEmitterFunc(func(stream domain.ExecutionStream, text string) error {
		return s.sendChunk(cmd, stream, text)
	}))
	s.finishExecution(cmd, result)
}

func (s *session) beginExecution(executionID string) bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()

	if _, ok := s.active[executionID]; ok {
		return false
	}
	s.active[executionID] = struct{}{}
	return true
}

func (s *session) endExecution(executionID string) {
	s.activeMu.Lock()
	delete(s.active, executionID)
	s.activeMu.Unlock()
}

func (s *session) registerShell(executionID string, shell *ShellSession) {
	s.shellsMu.Lock()
	s.shells[executionID] = shell
	s.shellsMu.Unlock()
}

func (s *session) unregisterShell(executionID string) {
	s.shellsMu.Lock()
	delete(s.shells, executionID)
	s.shellsMu.Unlock()
}

func (s *session) getShell(executionID string) *ShellSession {
	s.shellsMu.RLock()
	defer s.shellsMu.RUnlock()
	return s.shells[executionID]
}

func (s *session) handleShellInput(payload wsagent.ShellInputPayload) {
	shell := s.getShell(payload.ExecutionID)
	if shell == nil {
		s.logger.Printf("nodeagent shell input for unknown execution=%s", payload.ExecutionID)
		return
	}
	if err := shell.WriteBase64(payload.Data); err != nil {
		s.logger.Printf("nodeagent shell input write error execution=%s: %v", payload.ExecutionID, err)
	}
}

func (s *session) handleShellResize(payload wsagent.ShellResizePayload) {
	shell := s.getShell(payload.ExecutionID)
	if shell == nil {
		s.logger.Printf("nodeagent shell resize for unknown execution=%s", payload.ExecutionID)
		return
	}
	if err := shell.Resize(payload.Rows, payload.Cols); err != nil {
		s.logger.Printf("nodeagent shell resize error execution=%s: %v", payload.ExecutionID, err)
	}
}

func (s *session) handleShellDispatch(ctx context.Context, cmd appexecution.DispatchCommand) {
	if !s.beginExecution(cmd.ExecutionID) {
		s.logger.Printf("nodeagent ignored duplicate shell dispatch execution=%s", cmd.ExecutionID)
		return
	}
	defer s.endExecution(cmd.ExecutionID)

	var args appexecution.OpenShellArgs
	if len(cmd.Args) > 0 {
		_ = json.Unmarshal(cmd.Args, &args)
	}
	if args.Rows <= 0 {
		args.Rows = 24
	}
	if args.Cols <= 0 {
		args.Cols = 80
	}

	emitter := chunkEmitterFunc(func(stream domain.ExecutionStream, text string) error {
		return s.sendChunk(cmd, stream, text)
	})

	s.logger.Printf("nodeagent opening shell execution=%s shell=%s rows=%d cols=%d", cmd.ExecutionID, args.Shell, args.Rows, args.Cols)
	shell, err := StartShell(ctx, args.Shell, args.Rows, args.Cols, emitter)
	if err != nil {
		s.logger.Printf("nodeagent shell start failed execution=%s: %v", cmd.ExecutionID, err)
		s.finishExecution(cmd, failureResult(1, err.Error()))
		return
	}

	s.registerShell(cmd.ExecutionID, shell)
	defer func() {
		s.unregisterShell(cmd.ExecutionID)
		shell.Close()
	}()

	result := shell.Wait()
	s.logger.Printf("nodeagent shell exited execution=%s status=%s", cmd.ExecutionID, result.Status)
	s.finishExecution(cmd, result)
}

func (s *session) handleUpgradeDispatch(ctx context.Context, cmd appexecution.DispatchCommand) {
	if !s.beginExecution(cmd.ExecutionID) {
		s.logger.Printf("nodeagent ignored duplicate upgrade dispatch execution=%s", cmd.ExecutionID)
		return
	}
	defer s.endExecution(cmd.ExecutionID)

	var args appexecution.UpgradeAgentArgs
	if len(cmd.Args) > 0 {
		_ = json.Unmarshal(cmd.Args, &args)
	}
	if args.DownloadURL == "" {
		s.finishExecution(cmd, failureResult(64, "upgrade requires downloadUrl"))
		return
	}

	emitter := chunkEmitterFunc(func(stream domain.ExecutionStream, text string) error {
		return s.sendChunk(cmd, stream, text)
	})

	s.logger.Printf("nodeagent starting upgrade execution=%s url=%s version=%s", cmd.ExecutionID, args.DownloadURL, args.TargetVersion)
	result := handleUpgrade(ctx, args.DownloadURL, args.TargetVersion, emitter)
	s.finishExecution(cmd, result)

	if result.Status == domain.ExecutionStatusSuccess {
		s.upgradeOnce.Do(func() {
			close(s.upgradeCh)
		})
	}
}

const gracefulShutdownTimeout = 5 * time.Second

func (s *session) gracefulShutdown() {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		s.logger.Printf("nodeagent all executions finished")
	case <-time.After(gracefulShutdownTimeout):
		s.logger.Printf("nodeagent shutdown timeout, %d executions still running", s.activeCount())
	}
}

func (s *session) activeCount() int {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return len(s.active)
}

func (s *session) sendRegister() error {
	payload, err := json.Marshal(wsagent.RegisterPayload{
		NodeID:   s.nodeID,
		Metadata: s.metadata,
	})
	if err != nil {
		return err
	}
	return s.sendEnvelope(wsagent.Message{
		Type:    wsagent.TypeAgentRegister,
		NodeID:  s.nodeID,
		Payload: payload,
	})
}

func (s *session) isBusy() bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return len(s.active) > 0
}

func (s *session) sendHeartbeat() error {
	payload, err := json.Marshal(wsagent.HeartbeatPayload{
		NodeID:  s.nodeID,
		Runtime: collectRuntime(s.isBusy()),
	})
	if err != nil {
		return err
	}
	return s.sendEnvelope(wsagent.Message{
		Type:    wsagent.TypeAgentHeartbeat,
		NodeID:  s.nodeID,
		Payload: payload,
	})
}

func (r *Runner) metadata() infraws.AgentNodeMetadata {
	hostname := strings.TrimSpace(r.NodeID)
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		hostname = strings.TrimSpace(host)
	}
	version := strings.TrimSpace(r.AgentVersion)
	if version == "" {
		version = "nodeagent-dev"
	}
	return infraws.AgentNodeMetadata{
		Hostname: hostname,
		Region:   strings.TrimSpace(r.Region),
		OS:       runtime.GOOS,
		Version:  version,
		Tags:     append([]string(nil), r.Tags...),
	}
}

func (s *session) sendChunk(cmd appexecution.DispatchCommand, stream domain.ExecutionStream, text string) error {
	if text == "" {
		return nil
	}
	payload, err := json.Marshal(wsagent.ChunkPayload{
		SessionID:   cmd.SessionID,
		TaskID:      cmd.TaskID,
		ExecutionID: cmd.ExecutionID,
		NodeID:      cmd.NodeID,
		Chunk: domain.ExecutionChunk{
			Stream: stream,
			Text:   text,
		},
	})
	if err != nil {
		return err
	}
	return s.sendEnvelope(wsagent.Message{
		Type:    wsagent.TypeExecutionChunk,
		NodeID:  cmd.NodeID,
		TaskID:  cmd.TaskID,
		Payload: payload,
	})
}

func (s *session) finishExecution(cmd appexecution.DispatchCommand, result ExecutionResult) {
	payload, err := json.Marshal(wsagent.FinishedPayload{
		SessionID:    cmd.SessionID,
		TaskID:       cmd.TaskID,
		ExecutionID:  cmd.ExecutionID,
		NodeID:       cmd.NodeID,
		Status:       result.Status,
		ExitCode:     result.ExitCode,
		StatusReason: result.StatusReason,
	})
	if err != nil {
		s.logger.Printf("nodeagent failed to encode finished event: %v", err)
		return
	}
	if err := s.sendEnvelope(wsagent.Message{
		Type:    wsagent.TypeExecutionFinish,
		NodeID:  cmd.NodeID,
		TaskID:  cmd.TaskID,
		Payload: payload,
	}); err != nil {
		s.logger.Printf("nodeagent failed to send finished task=%s execution=%s: %v", cmd.TaskID, cmd.ExecutionID, err)
		return
	}
	s.logger.Printf("nodeagent finished task=%s execution=%s status=%s", cmd.TaskID, cmd.ExecutionID, result.Status)
}

func (s *session) sendEnvelope(msg wsagent.Message) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, raw)
}

type chunkEmitterFunc func(stream domain.ExecutionStream, text string) error

func (f chunkEmitterFunc) Emit(stream domain.ExecutionStream, text string) error {
	return f(stream, text)
}
