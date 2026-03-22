package lock

import (
	"context"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type MemoryLockManager struct {
	mu    sync.Mutex
	locks map[string]struct{}
}

func NewMemoryLockManager() *MemoryLockManager {
	return &MemoryLockManager{
		locks: make(map[string]struct{}),
	}
}

func (m *MemoryLockManager) LockSession(ctx context.Context, sessionID string) (domain.UnlockFunc, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sessionID == "" {
		return nil, domain.ErrInvalidArgument
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.locks[sessionID]; exists {
		return nil, domain.ErrSessionBusy
	}
	m.locks[sessionID] = struct{}{}

	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.locks, sessionID)
			m.mu.Unlock()
		})
	}, nil
}
