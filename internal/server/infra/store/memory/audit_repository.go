package memory

import (
	"context"
	"sync"

	"github.com/momaek/tolato/internal/server/domain"
)

type auditRepository struct {
	mu    sync.RWMutex
	items map[string]domain.AuditRecord
	order []string
}

func NewAuditRepository() domain.AuditRepository {
	return &auditRepository{
		items: make(map[string]domain.AuditRecord),
	}
}

func (r *auditRepository) Append(ctx context.Context, record domain.AuditRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[record.ID]; exists {
		return domain.ErrAlreadyExists
	}

	r.items[record.ID] = cloneAuditRecord(record)
	r.order = append(r.order, record.ID)
	return nil
}

func (r *auditRepository) ListByTask(ctx context.Context, taskID string) ([]domain.AuditRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domain.AuditRecord, 0, len(r.order))
	for _, id := range r.order {
		record, ok := r.items[id]
		if !ok {
			continue
		}
		if record.TaskID == nil || *record.TaskID != taskID {
			continue
		}
		out = append(out, cloneAuditRecord(record))
	}
	return out, nil
}
