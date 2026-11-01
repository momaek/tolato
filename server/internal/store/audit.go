package store

import (
	"encoding/json"
	"time"

	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/gorm"
)

// CreateAuditLog creates a new audit log entry.
func CreateAuditLog(log *model.AuditLog) error {
	// Coordinate per-node commit order with polling watermarks. A sequence ID
	// alone is not a commit watermark: another transaction can commit a larger
	// ID while this insert is still in flight. Transaction-scoped locks work
	// across server instances and only serialize writes for the same node.
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "tolato.audit."+log.NodeID).Error; err != nil {
			return err
		}
		return tx.Create(log).Error
	})
}

// AuditLogFilter narrows an audit log listing. Nil and empty fields don't
// filter.
type AuditLogFilter struct {
	ID       *uint
	NodeID   *string
	Keyword  *string
	From, To *time.Time
	// Metadata matches rows tagged with every one of these pairs, exactly.
	Metadata map[string]string
}

// ListAuditLogs returns paginated audit logs with optional filters.
func ListAuditLogs(page, pageSize int, f AuditLogFilter) ([]model.AuditLog, int64, error) {
	return ListAuditLogsScoped(page, pageSize, f, nil, true)
}

// ListAuditLogsScoped is ListAuditLogs restricted to a set of node ids.
// unrestricted skips the restriction (admins, and grants on "all").
func ListAuditLogsScoped(page, pageSize int, f AuditLogFilter, visibleNodeIDs []string, unrestricted bool) ([]model.AuditLog, int64, error) {
	var total int64
	query := DB.Model(&model.AuditLog{})

	if !unrestricted {
		query = query.Where("node_id IN ?", visibleNodeIDs)
	}
	if f.ID != nil {
		query = query.Where("id = ?", *f.ID)
	}
	if f.NodeID != nil && *f.NodeID != "" {
		query = query.Where("node_id = ?", *f.NodeID)
	}
	if f.Keyword != nil && *f.Keyword != "" {
		query = query.Where("command LIKE ?", "%"+*f.Keyword+"%")
	}
	if f.From != nil {
		query = query.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		query = query.Where("created_at <= ?", *f.To)
	}
	if len(f.Metadata) > 0 {
		// Containment rather than one ->> per key, so the GIN index applies.
		b, err := json.Marshal(f.Metadata)
		if err != nil {
			return nil, 0, err
		}
		query = query.Where("metadata @> ?::jsonb", string(b))
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []model.AuditLog
	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}
