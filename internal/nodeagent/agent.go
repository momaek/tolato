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
	Dialer            *websocket.Dialer
	Logger            *log.Logger
	Executor          Executor
}

func (r *Runner) Run(ctx context.Context) error {
	if strings.TrimSpace(r.URL) == "" || strings.TrimSpace(r.NodeID) == "" {
		return errors.New("url and node id are required")
	}

	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := r.runSession(ctx); err != nil && ctx.Err() == nil {
			r.logger().Printf("nodeagent reconnecting after error: %v", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.reconnectDelay()):
		}
	}
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
		nodeID:   r.NodeID,
		metadata: r.metadata(),
		conn:     conn,
		logger:   r.logger(),
		executor: r.executor(),
		active:   make(map[string]struct{}),
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
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutting down"), time.Now().Add(time.Second))
			return nil
		case <-ticker.C:
			if err := session.sendHeartbeat(); err != nil {
				return err
			}
		case err := <-readErr:
			return err
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

type session struct {
	nodeID   string
	metadata infraws.AgentNodeMetadata
	conn     *websocket.Conn
	logger   *log.Logger
	executor Executor

	writeMu  sync.Mutex
	activeMu sync.Mutex
	active   map[string]struct{}
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
			go s.handleDispatch(ctx, cmd)
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

func (s *session) sendHeartbeat() error {
	payload, err := json.Marshal(wsagent.HeartbeatPayload{
		NodeID:  s.nodeID,
		Runtime: collectRuntime(),
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
