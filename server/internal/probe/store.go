package probe

import (
	"log"
	"time"

	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/gorm"
)

// Store handles probe-related database operations.
type Store struct {
	db *gorm.DB
}

// NewStore creates a new probe Store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// --- ProbeLink ---

func (s *Store) CreateLink(link *model.ProbeLink) error {
	return s.db.Create(link).Error
}

func (s *Store) ListLinks() ([]model.ProbeLink, error) {
	var links []model.ProbeLink
	err := s.db.Preload("Source").Preload("Target").Find(&links).Error
	return links, err
}

func (s *Store) GetLink(id string) (*model.ProbeLink, error) {
	var link model.ProbeLink
	err := s.db.Preload("Source").Preload("Target").First(&link, "id = ?", id).Error
	return &link, err
}

func (s *Store) DeleteLink(id string) error {
	return s.db.Where("id = ?", id).Delete(&model.ProbeLink{}).Error
}

// --- ProbeMetric ---

func (s *Store) CreateMetrics(metrics []model.ProbeMetric) error {
	if len(metrics) == 0 {
		return nil
	}
	return s.db.Create(&metrics).Error
}

func (s *Store) ListMetrics(linkID string, from, to *time.Time) ([]model.ProbeMetric, error) {
	query := s.db.Where("link_id = ?", linkID)
	if from != nil {
		query = query.Where("timestamp >= ?", *from)
	}
	if to != nil {
		query = query.Where("timestamp <= ?", *to)
	}
	var metrics []model.ProbeMetric
	err := query.Order("timestamp DESC").Limit(1000).Find(&metrics).Error
	return metrics, err
}

func (s *Store) GetLatestMetric(linkID string) (*model.ProbeMetric, error) {
	var metric model.ProbeMetric
	err := s.db.Where("link_id = ?", linkID).Order("timestamp DESC").First(&metric).Error
	if err != nil {
		return nil, err
	}
	return &metric, nil
}

func (s *Store) CleanupOldMetrics(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	return s.db.Where("timestamp < ?", cutoff).Delete(&model.ProbeMetric{}).Error
}

// --- ProbeAlert ---

func (s *Store) CreateAlert(alert *model.ProbeAlert) error {
	return s.db.Create(alert).Error
}

func (s *Store) ListAlerts(linkID *string, alertType *string, resolved *bool) ([]model.ProbeAlert, error) {
	query := s.db.Model(&model.ProbeAlert{})
	if linkID != nil && *linkID != "" {
		query = query.Where("link_id = ?", *linkID)
	}
	if alertType != nil && *alertType != "" {
		query = query.Where("type = ?", *alertType)
	}
	if resolved != nil {
		if *resolved {
			query = query.Where("resolved_at IS NOT NULL")
		} else {
			query = query.Where("resolved_at IS NULL")
		}
	}
	var alerts []model.ProbeAlert
	err := query.Order("triggered_at DESC").Limit(200).Find(&alerts).Error
	return alerts, err
}

func (s *Store) GetUnresolvedAlert(linkID, alertType string) (*model.ProbeAlert, error) {
	var alert model.ProbeAlert
	err := s.db.Where("link_id = ? AND type = ? AND resolved_at IS NULL", linkID, alertType).First(&alert).Error
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

func (s *Store) ResolveAlert(id uint) error {
	now := time.Now()
	return s.db.Model(&model.ProbeAlert{}).Where("id = ?", id).Update("resolved_at", &now).Error
}

func (s *Store) CleanupResolvedAlerts() error {
	return s.db.Where("resolved_at IS NOT NULL").Delete(&model.ProbeAlert{}).Error
}

// StartCleanupScheduler runs a periodic cleanup task for old metrics and resolved alerts.
func StartCleanupScheduler(s *Store, retentionDays int) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		if err := s.CleanupOldMetrics(retentionDays); err != nil {
			log.Printf("[probe] cleanup old metrics failed: %v", err)
		}
		if err := s.CleanupResolvedAlerts(); err != nil {
			log.Printf("[probe] cleanup resolved alerts failed: %v", err)
		}
	}
}

// --- Node positions ---

func (s *Store) UpdateNodePosition(nodeID string, x, y float64) error {
	return s.db.Model(&model.Node{}).Where("id = ?", nodeID).Updates(map[string]any{
		"canvas_x": x,
		"canvas_y": y,
	}).Error
}

func (s *Store) UpdateNodeRole(nodeID, role string) error {
	return s.db.Model(&model.Node{}).Where("id = ?", nodeID).Update("role", role).Error
}
