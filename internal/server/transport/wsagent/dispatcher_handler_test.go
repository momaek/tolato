package wsagent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

func TestHandlerRoutesRegisterAndHeartbeat(t *testing.T) {
	hub := infraws.NewMemoryHub()
	client := infraws.NewMemoryClient("agent-9", infraws.ClientKindAgent, 4)
	hub.Register(client)
	registry := infraws.NewMemoryAgentRegistry(hub)
	handler := Handler{
		Dispatcher: Dispatcher{
			Agents: registry,
			Now:    func() time.Time { return time.Date(2026, 3, 22, 21, 5, 0, 0, time.UTC) },
		},
	}

	registerRaw, err := handler.Handle(context.Background(), "agent-9", mustAgentMessage(t, Message{
		Type: TypeAgentRegister,
		Payload: mustAgentPayload(t, RegisterPayload{
			NodeID: "node-9",
			Metadata: infraws.AgentNodeMetadata{
				Hostname: "node-9.example",
				Region:   "Tokyo",
				OS:       "linux",
				Version:  "1.0.0",
			},
		}),
	}))
	if err != nil {
		t.Fatalf("Handle(register) error = %v", err)
	}
	var ack Ack
	if err := json.Unmarshal(registerRaw, &ack); err != nil {
		t.Fatalf("json.Unmarshal(register) error = %v", err)
	}
	if ack.Type != TypeAgentAck || ack.NodeID != "node-9" {
		t.Fatalf("ack = %#v", ack)
	}

	if _, err := handler.Handle(context.Background(), "agent-9", mustAgentMessage(t, Message{
		Type: TypeAgentHeartbeat,
		Payload: mustAgentPayload(t, HeartbeatPayload{
			NodeID: "node-9",
			Runtime: infraws.AgentNodeRuntime{
				Busy: true,
				Metrics: infraws.AgentNodeMetrics{
					CPU:    0.4,
					Memory: 0.5,
					Disk:   0.6,
				},
			},
		}),
	})); err != nil {
		t.Fatalf("Handle(heartbeat) error = %v", err)
	}
	if got, ok := registry.LastHeartbeat("node-9"); !ok || got.UTC().Format(time.RFC3339) != "2026-03-22T21:05:00Z" {
		t.Fatalf("LastHeartbeat() = %v, %v", got, ok)
	}
	snapshots := registry.Snapshots()
	if len(snapshots) != 1 || snapshots[0].Hostname != "node-9.example" || !snapshots[0].Busy {
		t.Fatalf("Snapshots() = %#v, want metadata/runtime persisted", snapshots)
	}
}

func TestDispatcherRoutesChunkAndFinished(t *testing.T) {
	executions := &stubExecutionService{}
	dispatcher := Dispatcher{
		Executions: executions,
		Now:        func() time.Time { return time.Date(2026, 3, 22, 21, 10, 0, 0, time.UTC) },
	}
	ctx := WithClientID(context.Background(), "agent-3")

	if _, err := dispatcher.Dispatch(ctx, mustAgentMessage(t, Message{
		Type: TypeExecutionChunk,
		Payload: mustAgentPayload(t, ChunkPayload{
			SessionID:   "sess-3",
			TaskID:      "task-3",
			ExecutionID: "exec-3",
			NodeID:      "node-3",
			Chunk: domain.ExecutionChunk{
				Stream: domain.ExecutionStreamStdout,
				Text:   "hello\n",
			},
		}),
	})); err != nil {
		t.Fatalf("Dispatch(chunk) error = %v", err)
	}

	exitCode := 0
	if _, err := dispatcher.Dispatch(ctx, mustAgentMessage(t, Message{
		Type: TypeExecutionFinish,
		Payload: mustAgentPayload(t, FinishedPayload{
			SessionID:   "sess-3",
			TaskID:      "task-3",
			ExecutionID: "exec-3",
			NodeID:      "node-3",
			Status:      domain.ExecutionStatusSuccess,
			ExitCode:    &exitCode,
		}),
	})); err != nil {
		t.Fatalf("Dispatch(finished) error = %v", err)
	}

	if executions.chunk.ExecutionID != "exec-3" || executions.chunk.Chunk.Text != "hello\n" {
		t.Fatalf("chunk input = %#v", executions.chunk)
	}
	if executions.finished.ExecutionID != "exec-3" || executions.finished.Status != domain.ExecutionStatusSuccess {
		t.Fatalf("finished input = %#v", executions.finished)
	}
}

type stubExecutionService struct {
	chunk    appexecution.RecordChunkInput
	finished appexecution.FinishExecutionInput
}

func (s *stubExecutionService) RecordChunk(ctx context.Context, input appexecution.RecordChunkInput) error {
	_ = ctx
	s.chunk = input
	return nil
}

func (s *stubExecutionService) FinishExecution(ctx context.Context, input appexecution.FinishExecutionInput) error {
	_ = ctx
	s.finished = input
	return nil
}

func mustAgentMessage(t *testing.T, msg Message) []byte {
	t.Helper()
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal(msg) error = %v", err)
	}
	return raw
}

func mustAgentPayload(t *testing.T, payload any) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal(payload) error = %v", err)
	}
	return raw
}
