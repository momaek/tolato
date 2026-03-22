package devexec

import (
	"context"
	"errors"
	"fmt"
	"time"

	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

type DispatchPublisher interface {
	DispatchToNode(ctx context.Context, nodeID string, cmd appexecution.DispatchCommand) error
}

type ExecutionRecorder interface {
	RecordChunk(ctx context.Context, input appexecution.RecordChunkInput) error
	FinishExecution(ctx context.Context, input appexecution.FinishExecutionInput) error
}

type FallbackDispatchPublisher struct {
	Primary   DispatchPublisher
	Recorder  ExecutionRecorder
	Sleep     func(time.Duration)
	ChunkText string
}

func (p *FallbackDispatchPublisher) DispatchToNode(ctx context.Context, nodeID string, cmd appexecution.DispatchCommand) error {
	if p.Primary != nil {
		err := p.Primary.DispatchToNode(ctx, nodeID, cmd)
		if err == nil {
			return nil
		}
		if !errors.Is(err, infraws.ErrNodeNotBound) && !errors.Is(err, infraws.ErrClientNotFound) {
			return err
		}
	}
	if p.Recorder == nil {
		return infraws.ErrNodeNotBound
	}

	go p.simulate(cmd)
	return nil
}

func (p *FallbackDispatchPublisher) simulate(cmd appexecution.DispatchCommand) {
	text := p.ChunkText
	if text == "" {
		text = fmt.Sprintf("dev fallback executed %s on %s\n", cmd.Action, cmd.NodeID)
	}

	_ = p.Recorder.RecordChunk(context.Background(), appexecution.RecordChunkInput{
		SessionID:   cmd.SessionID,
		TaskID:      cmd.TaskID,
		ExecutionID: cmd.ExecutionID,
		NodeID:      cmd.NodeID,
		Chunk: domain.ExecutionChunk{
			Stream: domain.ExecutionStreamStdout,
			Text:   text,
		},
	})

	p.sleep(50 * time.Millisecond)

	exitCode := 0
	_ = p.Recorder.FinishExecution(context.Background(), appexecution.FinishExecutionInput{
		SessionID:   cmd.SessionID,
		TaskID:      cmd.TaskID,
		ExecutionID: cmd.ExecutionID,
		NodeID:      cmd.NodeID,
		Status:      domain.ExecutionStatusSuccess,
		ExitCode:    &exitCode,
	})
}

func (p *FallbackDispatchPublisher) sleep(d time.Duration) {
	if p.Sleep != nil {
		p.Sleep(d)
		return
	}
	time.Sleep(d)
}
