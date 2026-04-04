package store

import (
	"time"

	"github.com/momaek/tolato/server/internal/model"
)

// CreateAuditLog creates a new audit log entry.
func CreateAuditLog(log *model.AuditLog) error {
	return DB.Create(log).Error
}

// ListAuditLogs returns paginated audit logs with optional filters.
func ListAuditLogs(page, pageSize int, nodeID, keyword *string, from, to *time.Time) ([]model.AuditLog, int64, error) {
	var total int64
	query := DB.Model(&model.AuditLog{})

	if nodeID != nil && *nodeID != "" {
		query = query.Where("node_id = ?", *nodeID)
	}
	if keyword != nil && *keyword != "" {
		query = query.Where("command LIKE ?", "%"+*keyword+"%")
	}
	if from != nil {
		query = query.Where("created_at >= ?", *from)
	}
	if to != nil {
		query = query.Where("created_at <= ?", *to)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []model.AuditLog
	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}
