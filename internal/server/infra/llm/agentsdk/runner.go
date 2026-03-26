package agentsdk

import (
	"context"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// RunResult is the final output from an agent-sdk-go run.
type RunResult struct {
	Content  string
	Error    error
	Streamed bool
}

// activeRunner holds the live state of an agent-sdk-go session goroutine.
// It is keyed by session ID in the provider's runner map.
type activeRunner struct {
	toolCallChan chan InterceptedCall
	resultChan   chan ToolCallResult
	doneChan     chan RunResult
	streamChan   chan interfaces.AgentStreamEvent
	cancel       context.CancelFunc

	closeOnce sync.Once
}

func newRunner() *activeRunner {
	return &activeRunner{
		toolCallChan: make(chan InterceptedCall, 1),
		resultChan:   make(chan ToolCallResult, 1),
		doneChan:     make(chan RunResult, 1),
		streamChan:   make(chan interfaces.AgentStreamEvent, 128),
	}
}

// stop cancels the underlying context, causing the blocked Execute() calls
// to return and the agent-sdk-go goroutine to terminate.
func (r *activeRunner) stop() {
	r.closeOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
	})
}
