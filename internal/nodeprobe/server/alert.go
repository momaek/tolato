package server

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/momaek/tolato/internal/nodeprobe/model"
)

// Notifier sends alert and recovery notifications.
type Notifier interface {
	SendAlert(ctx context.Context, alert model.Alert, linkName string) error
	SendRecovery(ctx context.Context, alert model.Alert, linkName string, duration time.Duration) error
}

// AlertEngine checks incoming metrics against thresholds and manages alert lifecycle.
type AlertEngine struct {
	Store      *Store
	Thresholds AlertThresholds
	Notifier   Notifier
	Logger     *log.Logger

	mu             sync.Mutex
	recoveryCounts map[string]int // key: "linkID:alertType"
}

// NewAlertEngine creates a new AlertEngine.
func NewAlertEngine(store *Store, thresholds AlertThresholds, notifier Notifier, logger *log.Logger) *AlertEngine {
	return &AlertEngine{
		Store:          store,
		Thresholds:     thresholds,
		Notifier:       notifier,
		Logger:         logger,
		recoveryCounts: make(map[string]int),
	}
}

// Check evaluates a metric against all alert thresholds for a given link.
func (e *AlertEngine) Check(ctx context.Context, linkID, linkName string, m model.MetricRow) {
	e.checkThreshold(ctx, linkID, linkName, model.AlertTypeLatency,
		m.LatencyAvg, e.Thresholds.LatencyMs, true,
		fmt.Sprintf("延迟 %.0fms（阈值 %.0fms）", m.LatencyAvg, e.Thresholds.LatencyMs))

	e.checkThreshold(ctx, linkID, linkName, model.AlertTypePacketLoss,
		m.PacketLoss, e.Thresholds.PacketLossPct, true,
		fmt.Sprintf("丢包率 %.1f%%（阈值 %.1f%%）", m.PacketLoss, e.Thresholds.PacketLossPct))

	e.checkThreshold(ctx, linkID, linkName, model.AlertTypeTCP,
		m.TCPConnectTime, e.Thresholds.TCPConnectMs, true,
		fmt.Sprintf("TCP握手 %.0fms（阈值 %.0fms）", m.TCPConnectTime, e.Thresholds.TCPConnectMs))

	if m.BandwidthMbps != nil {
		e.checkThreshold(ctx, linkID, linkName, model.AlertTypeBandwidth,
			*m.BandwidthMbps, e.Thresholds.BandwidthMbps, false,
			fmt.Sprintf("带宽 %.1fMbps（阈值 %.1fMbps）", *m.BandwidthMbps, e.Thresholds.BandwidthMbps))
	}
}

// checkThreshold handles a single metric type.
// exceedAbove=true means value > threshold triggers alert, false means value < threshold triggers.
func (e *AlertEngine) checkThreshold(
	ctx context.Context,
	linkID, linkName string,
	alertType model.AlertType,
	value, threshold float64,
	exceedAbove bool,
	message string,
) {
	exceeded := (exceedAbove && value > threshold) || (!exceedAbove && value < threshold)
	key := linkID + ":" + string(alertType)

	e.mu.Lock()
	defer e.mu.Unlock()

	openAlerts, err := e.Store.OpenAlerts(ctx, linkID, alertType)
	if err != nil {
		e.Logger.Printf("check alerts error: %v", err)
		return
	}

	if exceeded {
		e.recoveryCounts[key] = 0

		if len(openAlerts) == 0 {
			alert := model.Alert{
				LinkID:      linkID,
				Type:        alertType,
				Message:     message,
				TriggeredAt: time.Now(),
			}
			id, err := e.Store.InsertAlert(ctx, alert)
			if err != nil {
				e.Logger.Printf("insert alert error: %v", err)
				return
			}
			alert.ID = id
			e.Logger.Printf("ALERT [%s] %s: %s", alertType, linkName, message)

			if err := e.Notifier.SendAlert(ctx, alert, linkName); err != nil {
				e.Logger.Printf("send alert notification error: %v", err)
			}
		}
	} else if len(openAlerts) > 0 {
		e.recoveryCounts[key]++

		if e.recoveryCounts[key] >= e.Thresholds.RecoveryCount {
			for _, a := range openAlerts {
				now := time.Now()
				if err := e.Store.ResolveAlert(ctx, a.ID, now); err != nil {
					e.Logger.Printf("resolve alert error: %v", err)
					continue
				}
				a.ResolvedAt = &now
				duration := now.Sub(a.TriggeredAt)
				e.Logger.Printf("RECOVERED [%s] %s: lasted %s", alertType, linkName, duration.Round(time.Second))

				if err := e.Notifier.SendRecovery(ctx, a, linkName, duration); err != nil {
					e.Logger.Printf("send recovery notification error: %v", err)
				}
			}
			delete(e.recoveryCounts, key)
		}
	}
}

// RunOfflineChecker periodically checks for nodes that haven't reported recently.
func (e *AlertEngine) RunOfflineChecker(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.checkOfflineNodes(ctx)
		}
	}
}

func (e *AlertEngine) checkOfflineNodes(ctx context.Context) {
	since := time.Now().Add(-time.Duration(e.Thresholds.OfflineSeconds) * time.Second)
	nodes, err := e.Store.NodesOfflineSince(ctx, since)
	if err != nil {
		e.Logger.Printf("offline check error: %v", err)
		return
	}

	for _, node := range nodes {
		links, err := e.Store.LinksForTarget(ctx, node.ID)
		if err != nil {
			e.Logger.Printf("offline links lookup error: %v", err)
			continue
		}

		for _, link := range links {
			openAlerts, err := e.Store.OpenAlerts(ctx, link.ID, model.AlertTypeOffline)
			if err != nil || len(openAlerts) > 0 {
				continue
			}

			linkName := link.SourceID + " → " + node.Name
			alert := model.Alert{
				LinkID:      link.ID,
				Type:        model.AlertTypeOffline,
				Message:     fmt.Sprintf("节点 %s 离线（超过 %d 秒未上报）", node.Name, e.Thresholds.OfflineSeconds),
				TriggeredAt: time.Now(),
			}
			id, err := e.Store.InsertAlert(ctx, alert)
			if err != nil {
				e.Logger.Printf("insert offline alert error: %v", err)
				continue
			}
			alert.ID = id
			e.Logger.Printf("ALERT [offline] %s: node %s offline", linkName, node.Name)

			if err := e.Notifier.SendAlert(ctx, alert, linkName); err != nil {
				e.Logger.Printf("send offline alert error: %v", err)
			}
		}
	}
}

// RecoverOfflineAlerts resolves offline alerts for a node that has come back online.
func (e *AlertEngine) RecoverOfflineAlerts(ctx context.Context, nodeID, nodeName string) {
	links, err := e.Store.LinksForTarget(ctx, nodeID)
	if err != nil {
		return
	}

	for _, link := range links {
		openAlerts, err := e.Store.OpenAlerts(ctx, link.ID, model.AlertTypeOffline)
		if err != nil || len(openAlerts) == 0 {
			continue
		}
		for _, a := range openAlerts {
			now := time.Now()
			if err := e.Store.ResolveAlert(ctx, a.ID, now); err != nil {
				continue
			}
			a.ResolvedAt = &now
			linkName := link.SourceID + " → " + nodeName
			duration := now.Sub(a.TriggeredAt)
			e.Logger.Printf("RECOVERED [offline] %s: lasted %s", linkName, duration.Round(time.Second))
			_ = e.Notifier.SendRecovery(ctx, a, linkName, duration)
		}
	}
}
