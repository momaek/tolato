package audit

import (
	"context"

	"github.com/momaek/tolato/internal/shared/types"
)

type AuditEvent = types.AuditEvent

type Repository interface {
	Create(ctx context.Context, event AuditEvent) error
	ListByTaskID(ctx context.Context, taskID string) ([]AuditEvent, error)
}
