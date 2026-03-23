package devseed

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func SeedConsoleStore(ctx context.Context, store *memory.Store, now time.Time) error {
	if store == nil {
		return domain.ErrInvalidArgument
	}

	return EnsureConsoleSession(ctx, store.Sessions, now)
}

func EnsureConsoleSession(ctx context.Context, repo domain.SessionRepository, now time.Time) error {
	if repo == nil {
		return domain.ErrInvalidArgument
	}

	sessions, err := repo.List(ctx, domain.SessionFilter{Limit: 1})
	if err != nil {
		return err
	}
	if len(sessions) > 0 {
		return nil
	}

	return repo.Create(ctx, domain.Session{
		ID:        "sess-console-1",
		Title:     "Console Session",
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: now.UTC().Add(-5 * time.Minute),
		UpdatedAt: now.UTC(),
	})
}
