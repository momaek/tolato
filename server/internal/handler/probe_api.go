package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/probe"
	"github.com/momaek/tolato/server/internal/store"
)

// ProbeReport is the payload from an agent's metrics report.
type ProbeReport struct {
	NodeID    string              `json:"node_id"`
	Timestamp string              `json:"timestamp"`
	Metrics   []ProbeReportMetric `json:"metrics"`
}

type ProbeReportMetric struct {
	TargetID       string   `json:"target_id"`
	LatencyMin     *float64 `json:"latency_min,omitempty"`
	LatencyAvg     *float64 `json:"latency_avg,omitempty"`
	LatencyMax     *float64 `json:"latency_max,omitempty"`
	PacketLoss     *float64 `json:"packet_loss,omitempty"`
	TCPConnectTime *float64 `json:"tcp_connect_time,omitempty"`
	BandwidthMbps  *float64 `json:"bandwidth_mbps,omitempty"`
}

// ProbeReportHandler handles agent metric reports.
func ProbeReportHandler(deps *Deps, probeStore *probe.Store, alertEngine *probe.AlertEngine) gin.HandlerFunc {
	return func(c *gin.Context) {
		var report ProbeReport
		if err := c.ShouldBindJSON(&report); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_request", Message: err.Error()})
			return
		}

		ts, err := time.Parse(time.RFC3339, report.Timestamp)
		if err != nil {
			ts = time.Now()
		}

		var metrics []model.ProbeMetric
		for _, m := range report.Metrics {
			linkID := fmt.Sprintf("%s->%s", report.NodeID, m.TargetID)
			metric := model.ProbeMetric{
				LinkID:         linkID,
				Timestamp:      ts,
				LatencyMin:     m.LatencyMin,
				LatencyAvg:     m.LatencyAvg,
				LatencyMax:     m.LatencyMax,
				PacketLoss:     m.PacketLoss,
				TCPConnectTime: m.TCPConnectTime,
				BandwidthMbps:  m.BandwidthMbps,
			}
			metrics = append(metrics, metric)

			// Process alert engine
			if alertEngine != nil {
				linkName := linkID
				alertEngine.ProcessMetric(&metric, linkName)
			}
		}

		if err := probeStore.CreateMetrics(metrics); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: "Failed to save metrics"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// ProbeListNodes returns all nodes with probe-related fields.
func ProbeListNodes(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodes, _, err := store.ListNodes(1, 200, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: err.Error()})
			return
		}

		items := make([]gin.H, 0, len(nodes))
		for _, n := range nodes {
			item := gin.H{
				"id":     n.ID,
				"name":   n.Name,
				"ip":     n.IP,
				"status": n.Status,
				"role":   n.Role,
			}
			if n.Alias != nil {
				item["alias"] = *n.Alias
			}
			if n.CanvasX != nil {
				item["canvas_x"] = *n.CanvasX
			}
			if n.CanvasY != nil {
				item["canvas_y"] = *n.CanvasY
			}
			items = append(items, item)
		}
		c.JSON(http.StatusOK, items)
	}
}

// ProbeListLinks returns all links with latest metrics.
func ProbeListLinks(deps *Deps, probeStore *probe.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		links, err := probeStore.ListLinks()
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: err.Error()})
			return
		}

		items := make([]gin.H, 0, len(links))
		for _, l := range links {
			item := gin.H{
				"id":        l.ID,
				"source_id": l.SourceID,
				"target_id": l.TargetID,
			}
			if l.Source != nil {
				item["source_name"] = l.Source.Name
			}
			if l.Target != nil {
				item["target_name"] = l.Target.Name
			}
			// Attach latest metric
			if metric, err := probeStore.GetLatestMetric(l.ID); err == nil {
				item["latest_metric"] = metric
			}
			items = append(items, item)
		}
		c.JSON(http.StatusOK, items)
	}
}

// ProbeGetLinkMetrics returns historical metrics for a link.
func ProbeGetLinkMetrics(deps *Deps, probeStore *probe.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		linkID := c.Param("id")
		var from, to *time.Time

		if f := c.Query("from"); f != "" {
			if t, err := time.Parse(time.RFC3339, f); err == nil {
				from = &t
			}
		}
		if t := c.Query("to"); t != "" {
			if parsed, err := time.Parse(time.RFC3339, t); err == nil {
				to = &parsed
			}
		}

		metrics, err := probeStore.ListMetrics(linkID, from, to)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: err.Error()})
			return
		}
		c.JSON(http.StatusOK, metrics)
	}
}

// ProbeListAlerts returns alerts with optional filters.
func ProbeListAlerts(deps *Deps, probeStore *probe.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var linkID, alertType *string
		var resolved *bool

		if v := c.Query("link_id"); v != "" {
			linkID = &v
		}
		if v := c.Query("type"); v != "" {
			alertType = &v
		}
		if v := c.Query("status"); v == "resolved" {
			b := true
			resolved = &b
		} else if v == "unresolved" {
			b := false
			resolved = &b
		}

		alerts, err := probeStore.ListAlerts(linkID, alertType, resolved)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: err.Error()})
			return
		}
		c.JSON(http.StatusOK, alerts)
	}
}

// ProbeCreateLink creates a new probe link.
func ProbeCreateLink(deps *Deps, probeStore *probe.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SourceID string `json:"source_id" binding:"required"`
			TargetID string `json:"target_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_request", Message: err.Error()})
			return
		}

		link := &model.ProbeLink{
			ID:       fmt.Sprintf("%s->%s", req.SourceID, req.TargetID),
			SourceID: req.SourceID,
			TargetID: req.TargetID,
		}

		if err := probeStore.CreateLink(link); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: err.Error()})
			return
		}

		// Push updated probe config to the source agent
		refreshProbeConfigForNode(deps, req.SourceID)

		c.JSON(http.StatusCreated, link)
	}
}

// ProbeDeleteLink deletes a probe link.
func ProbeDeleteLink(deps *Deps, probeStore *probe.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		// Get link info before deleting to know which agent to refresh
		link, _ := probeStore.GetLink(id)

		if err := probeStore.DeleteLink(id); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: err.Error()})
			return
		}

		// Push updated probe config to the source agent
		if link != nil {
			refreshProbeConfigForNode(deps, link.SourceID)
		}

		c.Status(http.StatusNoContent)
	}
}

// refreshProbeConfigForNode pushes updated probe config to a specific agent.
func refreshProbeConfigForNode(deps *Deps, nodeID string) {
	conn, ok := deps.NodeManager.GetConn(nodeID)
	if !ok {
		return
	}
	pushProbeConfig(deps, nodeID, conn)
}

// ProbeUpdateNodePosition updates node canvas position.
func ProbeUpdateNodePosition(deps *Deps, probeStore *probe.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")
		var req struct {
			CanvasX *float64 `json:"canvas_x"`
			CanvasY *float64 `json:"canvas_y"`
			Role    *string  `json:"role"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_request", Message: err.Error()})
			return
		}

		if req.CanvasX != nil && req.CanvasY != nil {
			probeStore.UpdateNodePosition(nodeID, *req.CanvasX, *req.CanvasY)
		}
		if req.Role != nil {
			probeStore.UpdateNodeRole(nodeID, *req.Role)
		}
		c.Status(http.StatusNoContent)
	}
}
