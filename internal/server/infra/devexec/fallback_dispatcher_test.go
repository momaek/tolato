package devexec

import (
	"context"
	"testing"
	"time"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

func TestFallbackDispatchPublisherSimulatesWhenNodeMissing(t *testing.T) {
	t.Parallel()

	recorder := &stubRecorder{}
	publisher := &FallbackDispatchPublisher{
		Primary:  stubPrimary{err: infraws.ErrNodeNotBound},
		Recorder: recorder,
		Sleep:    func(_ time.Duration) {},
	}

	err := publisher.DispatchToNode(context.Background(), "jp-tokyo-01", appexecution.DispatchCommand{
		SessionID:   "sess-1",
		TaskID:      "task-1",
		ExecutionID: "exec-1",
		NodeID:      "jp-tokyo-01",
		Action:      "run_command",
	})
	if err != nil {
		t.Fatalf("DispatchToNode() error = %v", err)
	}
	recorder.wait()

	if recorder.recorded.ExecutionID != "exec-1" || recorder.finished.ExecutionID != "exec-1" {
		t.Fatalf("recorder = %#v, want simulated chunk + finish", recorder)
	}
	if recorder.finished.Status != domain.ExecutionStatusSuccess {
		t.Fatalf("finish = %#v, want success", recorder.finished)
	}
}

type stubPrimary struct {
	err error
}

func (s stubPrimary) DispatchToNode(ctx context.Context, nodeID string, cmd appexecution.DispatchCommand) error {
	_ = ctx
	_ = nodeID
	_ = cmd
	return s.err
}

type stubRecorder struct {
	recorded appexecution.RecordChunkInput
	finished appexecution.FinishExecutionInput
	done     chan struct{}
}

func (s *stubRecorder) RecordChunk(ctx context.Context, input appexecution.RecordChunkInput) error {
	_ = ctx
	s.recorded = input
	if s.done == nil {
		s.done = make(chan struct{}, 1)
	}
	return nil
}

func (s *stubRecorder) FinishExecution(ctx context.Context, input appexecution.FinishExecutionInput) error {
	_ = ctx
	s.finished = input
	if s.done == nil {
		s.done = make(chan struct{}, 1)
	}
	select {
	case s.done <- struct{}{}:
	default:
	}
	return nil
}

func (s *stubRecorder) wait() {
	if s.done == nil {
		s.done = make(chan struct{}, 1)
	}
	select {
	case <-s.done:
	case <-time.After(200 * time.Millisecond):
	}
}
