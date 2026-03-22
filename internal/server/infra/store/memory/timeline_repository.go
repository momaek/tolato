package memory

import (
	"context"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type timelineRepository struct {
	mu    sync.RWMutex
	items map[string]domain.TimelineRow
	order []string
}

func NewTimelineRepository() domain.TimelineRepository {
	return &timelineRepository{
		items: make(map[string]domain.TimelineRow),
	}
}

func (r *timelineRepository) Append(ctx context.Context, row domain.TimelineRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[row.ID]; exists {
		return domain.ErrAlreadyExists
	}

	r.items[row.ID] = cloneTimelineRow(row)
	r.order = append(r.order, row.ID)
	return nil
}

func (r *timelineRepository) ListBySession(ctx context.Context, sessionID string, page domain.CursorPage) ([]domain.TimelineRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	selected := make([]domain.TimelineRow, 0, len(r.order))
	if page.BeforeID == "" {
		for _, id := range r.order {
			row, ok := r.items[id]
			if !ok || row.SessionID != sessionID {
				continue
			}
			selected = append(selected, cloneTimelineRow(row))
		}
		return tailByLimit(selected, page.Limit), nil
	}

	found := false
	for _, id := range r.order {
		if id == page.BeforeID {
			found = true
			break
		}
		row, ok := r.items[id]
		if !ok || row.SessionID != sessionID {
			continue
		}
		selected = append(selected, cloneTimelineRow(row))
	}

	if !found {
		return []domain.TimelineRow{}, nil
	}

	return tailByLimit(selected, page.Limit), nil
}
