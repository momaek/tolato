package agentsdk

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/momaek/tolato/internal/server/app/runtime"
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
	runCtx       context.Context // context for the runner goroutine, used by forwarder

	// responseID is updated atomically so the single forwarder goroutine
	// always tags events with the current turn's response ID.
	responseID atomic.Value // string

	closeOnce     sync.Once
	forwarderOnce sync.Once
}

func newRunner() *activeRunner {
	r := &activeRunner{
		toolCallChan: make(chan InterceptedCall, 1),
		resultChan:   make(chan ToolCallResult, 1),
		doneChan:     make(chan RunResult, 1),
		streamChan:   make(chan interfaces.AgentStreamEvent, 128),
	}
	r.responseID.Store("")
	return r
}

// setResponseID updates the response ID for the current turn.
func (r *activeRunner) setResponseID(id string) {
	r.responseID.Store(id)
}

// getResponseID returns the current response ID.
func (r *activeRunner) getResponseID() string {
	v, _ := r.responseID.Load().(string)
	return v
}

// startForwarder ensures a single streaming forwarder goroutine is running
// for this runner's entire lifetime, preventing goroutine leaks from
// multiple waitForEvent calls.
func (r *activeRunner) startForwarder(sessionID string, events runtime.EventPublisher) {
	r.forwarderOnce.Do(func() {
		ctx := r.runCtx
		if ctx == nil {
			ctx = context.Background()
		}
		go forwardStreamEventsWithDynamicID(ctx, sessionID, r, events)
	})
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
