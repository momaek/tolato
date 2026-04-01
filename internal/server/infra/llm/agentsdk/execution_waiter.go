package agentsdk

import (
	"sync"

	"github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/domain"
)

// Ensure WaiterRegistry implements ExecutionWaiterSignaler.
var _ execution.ExecutionWaiterSignaler = (*WaiterRegistry)(nil)

// executionWaiter allows a blocked tool Execute() to wait synchronously
// until all executions in a task finish.
type executionWaiter struct {
	taskID   string
	doneChan chan domain.ExecutionResult
}

// WaiterRegistry is a thread-safe registry of execution waiters.
// It is shared between the provider (which creates waiters) and the
// execution service (which signals completion).
type WaiterRegistry struct {
	mu      sync.Mutex
	waiters map[string]*executionWaiter // taskID → waiter
}

// NewWaiterRegistry creates a new empty registry.
func NewWaiterRegistry() *WaiterRegistry {
	return &WaiterRegistry{
		waiters: make(map[string]*executionWaiter),
	}
}

// Register creates a waiter for the given task and returns its done channel.
func (r *WaiterRegistry) Register(taskID string) <-chan domain.ExecutionResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	w := &executionWaiter{
		taskID:   taskID,
		doneChan: make(chan domain.ExecutionResult, 1),
	}
	r.waiters[taskID] = w
	return w.doneChan
}

// Signal implements execution.ExecutionWaiterSignaler. It notifies the
// waiter for the given task that execution is complete.
// Returns true if a waiter was found (sync path), false otherwise (legacy async path).
func (r *WaiterRegistry) Signal(taskID string, status domain.TaskStatus, aggregate domain.ExecutionAggregate) bool {
	r.mu.Lock()
	w, ok := r.waiters[taskID]
	if ok {
		delete(r.waiters, taskID)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	w.doneChan <- domain.ExecutionResult{TaskStatus: status, Aggregate: aggregate}
	return true
}

// Remove cleans up a waiter without signaling (e.g. on cancellation).
func (r *WaiterRegistry) Remove(taskID string) {
	r.mu.Lock()
	delete(r.waiters, taskID)
	r.mu.Unlock()
}
