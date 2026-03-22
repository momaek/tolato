package lock

import (
	"context"
	"errors"
	"testing"

	"github.com/momaek/tolato/internal/server/domain"
)

func TestMemoryLockManagerRejectsConcurrentSession(t *testing.T) {
	manager := NewMemoryLockManager()

	unlock, err := manager.LockSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("LockSession() error = %v", err)
	}
	defer unlock()

	_, err = manager.LockSession(context.Background(), "sess-1")
	if !errors.Is(err, domain.ErrSessionBusy) {
		t.Fatalf("LockSession() error = %v, want ErrSessionBusy", err)
	}
}

func TestMemoryLockManagerReleasesSession(t *testing.T) {
	manager := NewMemoryLockManager()

	unlock, err := manager.LockSession(context.Background(), "sess-2")
	if err != nil {
		t.Fatalf("LockSession() error = %v", err)
	}
	unlock()
	unlock()

	reunlock, err := manager.LockSession(context.Background(), "sess-2")
	if err != nil {
		t.Fatalf("LockSession() error after release = %v", err)
	}
	reunlock()
}
