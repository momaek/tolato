package memory

import (
	"context"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type toolCallRepository struct {
	mu    sync.RWMutex
	items map[string]domain.ToolCall
	order []string
}

func NewToolCallRepository() domain.ToolCallRepository {
	return &toolCallRepository{
		items: make(map[string]domain.ToolCall),
	}
}

func (r *toolCallRepository) Append(ctx context.Context, call domain.ToolCall) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[call.ID]; exists {
		return domain.ErrAlreadyExists
	}

	r.items[call.ID] = cloneToolCall(call)
	r.order = append(r.order, call.ID)
	return nil
}

func (r *toolCallRepository) ListBySession(ctx context.Context, sessionID string, page domain.CursorPage) ([]domain.ToolCall, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]domain.ToolCall, 0, len(r.order))
	for _, id := range r.order {
		item, ok := r.items[id]
		if !ok || item.SessionID != sessionID {
			continue
		}
		items = append(items, cloneToolCall(item))
	}

	if page.BeforeID != "" {
		filtered, err := filterBeforeID(items, func(item domain.ToolCall) string { return item.ID }, page.BeforeID)
		if err != nil {
			return nil, err
		}
		items = filtered
	}

	return tailByLimit(items, page.Limit), nil
}
