package store

import (
	"time"

	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/gorm"
)

// CommandEventWindow is a fixed upper bound plus an exclusive seek position.
// Equal timestamps are ordered by id; newly appended rows do not move pages.
type CommandEventWindow struct {
	Since     time.Time `json:"since"`
	UpperID   uint      `json:"upper_id"`
	LowerID   uint      `json:"lower_id"`
	AfterTime time.Time `json:"after_time"`
	AfterID   uint      `json:"after_id"`
}

func LatestNodeAuditID(nodeID string) (uint, error) {
	var id uint
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Wait for in-flight inserts before taking the watermark. No ID from
		// a later same-node insert can overtake an earlier uncommitted write.
		if err := tx.Exec("SET LOCAL lock_timeout = '5s'").Error; err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock_shared(hashtextextended(?, 0))", "tolato.audit."+nodeID).Error; err != nil {
			return err
		}
		return tx.Model(&model.AuditLog{}).Where("node_id = ?", nodeID).
			Select("COALESCE(MAX(id), 0)").Scan(&id).Error
	})
	return id, err
}

func ListNodeCommandEvents(nodeID string, w CommandEventWindow, page, size int) ([]model.CommandEvent, int64, error) {
	q := DB.Model(&model.AuditLog{}).Where("node_id = ? AND id > ? AND id <= ?", nodeID, w.LowerID, w.UpperID)
	if !w.Since.IsZero() {
		q = q.Where("created_at > ?", w.Since)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if w.AfterID != 0 {
		q = q.Where("created_at > ? OR (created_at = ? AND id > ?)", w.AfterTime, w.AfterTime, w.AfterID)
	}
	items := make([]model.CommandEvent, 0)
	err := q.Select("id", "command", "source", "actor", "created_at").
		Order("created_at ASC, id ASC").Offset((page - 1) * size).Limit(size + 1).Find(&items).Error
	return items, total, err
}
