package probe

import (
	"fmt"
	"log"
	"time"

	"github.com/momaek/tolato/server/internal/model"
)

// AlertConfig holds alert threshold configuration.
type AlertConfig struct {
	LatencyThresholdMS       float64
	PacketLossThresholdPct   float64
	TCPConnectThresholdMS    float64
	BandwidthThresholdMbps   float64
	OfflineTimeoutSeconds    int
	RecoveryCount            int
}

// DefaultAlertConfig returns default alert thresholds.
func DefaultAlertConfig() AlertConfig {
	return AlertConfig{
		LatencyThresholdMS:     200,
		PacketLossThresholdPct: 5,
		TCPConnectThresholdMS:  500,
		BandwidthThresholdMbps: 10,
		OfflineTimeoutSeconds:  180,
		RecoveryCount:          3,
	}
}

// AlertEngine processes metrics and triggers/resolves alerts.
type AlertEngine struct {
	store    *Store
	config   AlertConfig
	notifier Notifier
	// recoveryCounters tracks consecutive normal readings per (linkID, alertType)
	recoveryCounters map[string]int
}

// Notifier sends alert/recovery notifications.
type Notifier interface {
	SendAlert(alert *model.ProbeAlert, linkName string) error
	SendRecovery(alert *model.ProbeAlert, linkName string, duration time.Duration) error
}

// NewAlertEngine creates a new AlertEngine.
func NewAlertEngine(store *Store, config AlertConfig, notifier Notifier) *AlertEngine {
	return &AlertEngine{
		store:            store,
		config:           config,
		notifier:         notifier,
		recoveryCounters: make(map[string]int),
	}
}

// ProcessMetric checks a metric against thresholds and triggers/resolves alerts.
func (ae *AlertEngine) ProcessMetric(metric *model.ProbeMetric, linkName string) {
	checks := []struct {
		alertType string
		value     *float64
		threshold float64
		above     bool // true = alert when above threshold, false = alert when below
	}{
		{"latency", metric.LatencyAvg, ae.config.LatencyThresholdMS, true},
		{"packet_loss", metric.PacketLoss, ae.config.PacketLossThresholdPct, true},
		{"tcp", metric.TCPConnectTime, ae.config.TCPConnectThresholdMS, true},
		{"bandwidth", metric.BandwidthMbps, ae.config.BandwidthThresholdMbps, false},
	}

	for _, check := range checks {
		if check.value == nil {
			continue
		}

		var isAlerting bool
		if check.above {
			isAlerting = *check.value > check.threshold
		} else {
			isAlerting = *check.value < check.threshold
		}

		key := metric.LinkID + ":" + check.alertType

		if isAlerting {
			ae.recoveryCounters[key] = 0
			ae.triggerAlert(metric.LinkID, check.alertType, *check.value, check.threshold, linkName)
		} else {
			ae.checkRecovery(metric.LinkID, check.alertType, key, linkName)
		}
	}
}

func (ae *AlertEngine) triggerAlert(linkID, alertType string, value, threshold float64, linkName string) {
	// Check if there's already an unresolved alert
	existing, err := ae.store.GetUnresolvedAlert(linkID, alertType)
	if err == nil && existing != nil {
		return // already alerting
	}

	message := fmt.Sprintf("%s: %.1f (threshold: %.1f)", alertType, value, threshold)
	alert := &model.ProbeAlert{
		LinkID:      linkID,
		Type:        alertType,
		Message:     message,
		TriggeredAt: time.Now(),
	}

	if err := ae.store.CreateAlert(alert); err != nil {
		log.Printf("[alert] failed to create alert: %v", err)
		return
	}

	if ae.notifier != nil {
		ae.notifier.SendAlert(alert, linkName)
	}
}

func (ae *AlertEngine) checkRecovery(linkID, alertType, key, linkName string) {
	ae.recoveryCounters[key]++

	if ae.recoveryCounters[key] >= ae.config.RecoveryCount {
		ae.recoveryCounters[key] = 0

		existing, err := ae.store.GetUnresolvedAlert(linkID, alertType)
		if err != nil || existing == nil {
			return // no alert to resolve
		}

		if err := ae.store.ResolveAlert(existing.ID); err != nil {
			log.Printf("[alert] failed to resolve alert: %v", err)
			return
		}

		duration := time.Since(existing.TriggeredAt)
		if ae.notifier != nil {
			ae.notifier.SendRecovery(existing, linkName, duration)
		}
	}
}
