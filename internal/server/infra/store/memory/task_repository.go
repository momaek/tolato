package memory

import (
	"context"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type taskRepository struct {
	mu    sync.RWMutex
	items map[string]domain.Task
	order []string
}

func NewTaskRepository() domain.TaskRepository {
	return &taskRepository{
		items: make(map[string]domain.Task),
	}
}

func (r *taskRepository) Create(ctx context.Context, task domain.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[task.ID]; exists {
		return domain.ErrAlreadyExists
	}

	r.items[task.ID] = cloneTask(task)
	r.order = append(r.order, task.ID)
	return nil
}

func (r *taskRepository) Get(ctx context.Context, taskID string) (domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return domain.Task{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	task, ok := r.items[taskID]
	if !ok {
		return domain.Task{}, domain.ErrNotFound
	}

	return cloneTask(task), nil
}

func (r *taskRepository) ListBySession(ctx context.Context, sessionID string) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.Task, 0, len(r.order))
	for _, id := range r.order {
		task, ok := r.items[id]
		if !ok || task.SessionID != sessionID {
			continue
		}
		out = append(out, cloneTask(task))
	}
	return out, nil
}

func (r *taskRepository) Update(ctx context.Context, task domain.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[task.ID]; !exists {
		return domain.ErrNotFound
	}

	r.items[task.ID] = cloneTask(task)
	return nil
}
