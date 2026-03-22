package memory

import (
	"context"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type executionRepository struct {
	mu    sync.RWMutex
	items map[string]domain.Execution
	order []string
}

func NewExecutionRepository() domain.ExecutionRepository {
	return &executionRepository{
		items: make(map[string]domain.Execution),
	}
}

func (r *executionRepository) Create(ctx context.Context, execution domain.Execution) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[execution.ID]; exists {
		return domain.ErrAlreadyExists
	}

	r.items[execution.ID] = cloneExecution(execution)
	r.order = append(r.order, execution.ID)
	return nil
}

func (r *executionRepository) Get(ctx context.Context, executionID string) (domain.Execution, error) {
	if err := ctx.Err(); err != nil {
		return domain.Execution{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	execution, ok := r.items[executionID]
	if !ok {
		return domain.Execution{}, domain.ErrNotFound
	}

	return cloneExecution(execution), nil
}

func (r *executionRepository) ListByTask(ctx context.Context, taskID string) ([]domain.Execution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.Execution, 0, len(r.order))
	for _, id := range r.order {
		execution, ok := r.items[id]
		if !ok || execution.TaskID != taskID {
			continue
		}
		out = append(out, cloneExecution(execution))
	}
	return out, nil
}

func (r *executionRepository) Update(ctx context.Context, execution domain.Execution) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[execution.ID]; !exists {
		return domain.ErrNotFound
	}

	r.items[execution.ID] = cloneExecution(execution)
	return nil
}

func (r *executionRepository) AggregateByTask(ctx context.Context, taskID string) (domain.ExecutionAggregate, error) {
	if err := ctx.Err(); err != nil {
		return domain.ExecutionAggregate{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var aggregate domain.ExecutionAggregate
	for _, id := range r.order {
		execution, ok := r.items[id]
		if !ok || execution.TaskID != taskID {
			continue
		}
		aggregate.Total++
		switch execution.Status {
		case domain.ExecutionStatusQueued:
			aggregate.Queued++
		case domain.ExecutionStatusDispatched:
			aggregate.Dispatched++
		case domain.ExecutionStatusRunning:
			aggregate.Running++
		case domain.ExecutionStatusSuccess:
			aggregate.Success++
		case domain.ExecutionStatusFailed:
			aggregate.Failed++
		case domain.ExecutionStatusTimeout:
			aggregate.Timeout++
		case domain.ExecutionStatusCancelled:
			aggregate.Cancelled++
		}
	}

	return aggregate, nil
}
