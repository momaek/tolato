package memory

import (
	"context"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type threadMessageRepository struct {
	mu    sync.RWMutex
	items map[string]domain.ThreadMessage
	order []string
}

func NewThreadMessageRepository() domain.ThreadMessageRepository {
	return &threadMessageRepository{
		items: make(map[string]domain.ThreadMessage),
	}
}

func (r *threadMessageRepository) Append(ctx context.Context, message domain.ThreadMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[message.ID]; exists {
		return domain.ErrAlreadyExists
	}

	r.items[message.ID] = cloneThreadMessage(message)
	r.order = append(r.order, message.ID)
	return nil
}

func (r *threadMessageRepository) ListBySession(ctx context.Context, sessionID string, page domain.CursorPage) ([]domain.ThreadMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	selected := make([]domain.ThreadMessage, 0, len(r.order))
	if page.BeforeID == "" {
		for _, id := range r.order {
			message, ok := r.items[id]
			if !ok || message.SessionID != sessionID {
				continue
			}
			selected = append(selected, cloneThreadMessage(message))
		}
		return tailByLimit(selected, page.Limit), nil
	}

	found := false
	for _, id := range r.order {
		if id == page.BeforeID {
			found = true
			break
		}
		message, ok := r.items[id]
		if !ok || message.SessionID != sessionID {
			continue
		}
		selected = append(selected, cloneThreadMessage(message))
	}

	if !found {
		return []domain.ThreadMessage{}, nil
	}

	return tailByLimit(selected, page.Limit), nil
}
