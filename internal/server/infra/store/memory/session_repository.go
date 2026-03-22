package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type sessionRepository struct {
	mu    sync.RWMutex
	items map[string]domain.Session
	order []string
}

func NewSessionRepository() domain.SessionRepository {
	return &sessionRepository{
		items: make(map[string]domain.Session),
	}
}

func (r *sessionRepository) Create(ctx context.Context, session domain.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[session.ID]; exists {
		return domain.ErrAlreadyExists
	}

	r.items[session.ID] = cloneSession(session)
	r.order = append(r.order, session.ID)
	return nil
}

func (r *sessionRepository) Get(ctx context.Context, sessionID string) (domain.Session, error) {
	if err := ctx.Err(); err != nil {
		return domain.Session{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.items[sessionID]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}

	return cloneSession(session), nil
}

func (r *sessionRepository) List(ctx context.Context, filter domain.SessionFilter) ([]domain.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]domain.Session, 0, len(r.order))
	statusSet := make(map[domain.SessionStatus]struct{}, len(filter.Statuses))
	for _, status := range filter.Statuses {
		statusSet[status] = struct{}{}
	}

	for _, id := range r.order {
		session, ok := r.items[id]
		if !ok {
			continue
		}
		if len(statusSet) > 0 {
			if _, ok := statusSet[session.Status]; !ok {
				continue
			}
		}
		items = append(items, cloneSession(session))
	}

	if len(items) > 1 {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
				return items[i].ID < items[j].ID
			}
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		})
	}

	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}

	return items, nil
}

func (r *sessionRepository) Update(ctx context.Context, session domain.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[session.ID]; !exists {
		return domain.ErrNotFound
	}

	r.items[session.ID] = cloneSession(session)
	return nil
}
