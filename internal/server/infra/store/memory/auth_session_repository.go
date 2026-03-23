package memory

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

type authSessionRepository struct {
	mu    sync.RWMutex
	items map[string]domain.AuthSession
}

func NewAuthSessionRepository() domain.AuthSessionRepository {
	return &authSessionRepository{
		items: make(map[string]domain.AuthSession),
	}
}

func (r *authSessionRepository) Put(ctx context.Context, session domain.AuthSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	r.items[strings.TrimSpace(session.Token)] = cloneAuthSession(session)
	r.mu.Unlock()
	return nil
}

func (r *authSessionRepository) GetByToken(ctx context.Context, token string) (domain.AuthSession, error) {
	if err := ctx.Err(); err != nil {
		return domain.AuthSession{}, err
	}

	r.mu.RLock()
	session, ok := r.items[strings.TrimSpace(token)]
	r.mu.RUnlock()
	if !ok {
		return domain.AuthSession{}, domain.ErrNotFound
	}
	return cloneAuthSession(session), nil
}

func (r *authSessionRepository) ListByUser(ctx context.Context, userID string) ([]domain.AuthSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	userID = strings.TrimSpace(userID)
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]domain.AuthSession, 0)
	for _, session := range r.items {
		if session.UserID != userID {
			continue
		}
		items = append(items, cloneAuthSession(session))
	}
	return items, nil
}

func (r *authSessionRepository) Touch(ctx context.Context, token string, lastSeenAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.items[strings.TrimSpace(token)]
	if !ok {
		return domain.ErrNotFound
	}
	session.LastSeenAt = lastSeenAt.UTC()
	r.items[strings.TrimSpace(token)] = session
	return nil
}

func (r *authSessionRepository) DeleteByToken(ctx context.Context, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	delete(r.items, strings.TrimSpace(token))
	r.mu.Unlock()
	return nil
}

func (r *authSessionRepository) DeleteByUser(ctx context.Context, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	userID = strings.TrimSpace(userID)
	r.mu.Lock()
	for token, session := range r.items {
		if session.UserID == userID {
			delete(r.items, token)
		}
	}
	r.mu.Unlock()
	return nil
}

func (r *authSessionRepository) DeleteByUserExceptSession(ctx context.Context, userID string, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	r.mu.Lock()
	for token, session := range r.items {
		if session.UserID != userID || session.SessionID == sessionID {
			continue
		}
		delete(r.items, token)
	}
	r.mu.Unlock()
	return nil
}
