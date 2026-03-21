package outbox

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/shared/types"
)

type Message = types.OutboxMessage

type Repository interface {
	Create(ctx context.Context, message Message) error
	ListPending(ctx context.Context, limit int) ([]Message, error)
	MarkPublished(ctx context.Context, id string, at time.Time) error
	IncrementAttempts(ctx context.Context, id string) error
}
