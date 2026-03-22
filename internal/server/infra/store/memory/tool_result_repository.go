package memory

import (
	"context"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type toolResultRepository struct {
	mu    sync.RWMutex
	items map[string]domain.ToolResult
	order []string
}

func NewToolResultRepository() domain.ToolResultRepository {
	return &toolResultRepository{
		items: make(map[string]domain.ToolResult),
	}
}

func (r *toolResultRepository) Append(ctx context.Context, result domain.ToolResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[result.ID]; exists {
		return domain.ErrAlreadyExists
	}

	r.items[result.ID] = cloneToolResult(result)
	r.order = append(r.order, result.ID)
	return nil
}

func (r *toolResultRepository) ListBySession(ctx context.Context, sessionID string, page domain.CursorPage) ([]domain.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]domain.ToolResult, 0, len(r.order))
	for _, id := range r.order {
		item, ok := r.items[id]
		if !ok || item.SessionID != sessionID {
			continue
		}
		items = append(items, cloneToolResult(item))
	}

	if page.BeforeID != "" {
		filtered, err := filterBeforeID(items, func(item domain.ToolResult) string { return item.ID }, page.BeforeID)
		if err != nil {
			return nil, err
		}
		items = filtered
	}

	return tailByLimit(items, page.Limit), nil
}

func (r *toolResultRepository) ListByTask(ctx context.Context, taskID string) ([]domain.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]domain.ToolResult, 0, len(r.order))
	for _, id := range r.order {
		item, ok := r.items[id]
		if !ok || item.TaskID == nil || *item.TaskID != taskID {
			continue
		}
		items = append(items, cloneToolResult(item))
	}

	return items, nil
}
