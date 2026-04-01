package server

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/internal/nodeprobe/model"
)

// API handles all HTTP endpoints for the nodeprobe subsystem.
type API struct {
	Store       *Store
	AlertEngine *AlertEngine
	AuthToken   string
	Logger      *log.Logger
}

// RegisterRoutes registers probe API routes on the given router group.
// Call this from the main server with the /api/v1 group.
func (a *API) RegisterRoutes(group gin.IRouter) {
	probe := group.Group("/probe")
	probe.GET("/nodes", a.handleListNodes)
	probe.GET("/links", a.handleListLinks)
	probe.GET("/links/:id/metrics", a.handleLinkMetrics)
	probe.GET("/alerts", a.handleListAlerts)
	probe.POST("/report", a.requireAgentToken(), a.handleReport)
}

func (a *API) requireAgentToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
		if token != a.AuthToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func (a *API) handleReport(c *gin.Context) {
	var report model.MetricReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if report.NodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id is required"})
		return
	}
	if report.Timestamp.IsZero() {
		report.Timestamp = time.Now()
	}

	ctx := c.Request.Context()

	sourceNode := model.Node{
		ID:       report.NodeID,
		Name:     report.NodeID,
		Role:     model.NodeRoleRelay,
		LastSeen: report.Timestamp,
	}
	if err := a.Store.UpsertNode(ctx, sourceNode); err != nil {
		a.Logger.Printf("upsert source node error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	var metricRows []model.MetricRow
	for _, tm := range report.Metrics {
		targetNode := model.Node{
			ID:       tm.TargetID,
			Name:     tm.TargetID,
			Role:     model.NodeRoleLanding,
			LastSeen: report.Timestamp,
		}
		if err := a.Store.UpsertNode(ctx, targetNode); err != nil {
			a.Logger.Printf("upsert target node error: %v", err)
			continue
		}

		linkID := model.LinkID(report.NodeID, tm.TargetID)
		link := model.Link{ID: linkID, SourceID: report.NodeID, TargetID: tm.TargetID}
		if err := a.Store.UpsertLink(ctx, link); err != nil {
			a.Logger.Printf("upsert link error: %v", err)
			continue
		}

		metricRows = append(metricRows, model.MetricRow{
			LinkID:         linkID,
			Timestamp:      report.Timestamp,
			LatencyMin:     tm.LatencyMin,
			LatencyAvg:     tm.LatencyAvg,
			LatencyMax:     tm.LatencyMax,
			PacketLoss:     tm.PacketLoss,
			TCPConnectTime: tm.TCPConnectTime,
			BandwidthMbps:  tm.BandwidthMbps,
		})
	}

	if len(metricRows) > 0 {
		if err := a.Store.InsertMetrics(ctx, metricRows); err != nil {
			a.Logger.Printf("insert metrics error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
	}

	for _, row := range metricRows {
		linkName := report.NodeID + " -> " + row.LinkID
		a.AlertEngine.Check(ctx, row.LinkID, linkName, row)
	}
	for _, tm := range report.Metrics {
		a.AlertEngine.RecoverOfflineAlerts(ctx, tm.TargetID, tm.TargetID)
	}
	a.AlertEngine.RecoverOfflineAlerts(ctx, report.NodeID, report.NodeID)

	c.JSON(http.StatusOK, model.ReportResponse{Status: "ok", Received: len(report.Metrics)})
}

func (a *API) handleListNodes(c *gin.Context) {
	nodes, err := a.Store.ListNodes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if nodes == nil {
		nodes = []model.Node{}
	}
	c.JSON(http.StatusOK, nodes)
}

func (a *API) handleListLinks(c *gin.Context) {
	links, err := a.Store.ListLinksWithStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if links == nil {
		links = []model.LinkStatus{}
	}
	c.JSON(http.StatusOK, links)
}

func (a *API) handleLinkMetrics(c *gin.Context) {
	linkID := c.Param("id")
	fromStr := c.DefaultQuery("from", time.Now().Add(-1*time.Hour).Format(time.RFC3339))
	toStr := c.DefaultQuery("to", time.Now().Format(time.RFC3339))

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'from' time"})
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'to' time"})
		return
	}

	metrics, err := a.Store.QueryMetrics(c.Request.Context(), linkID, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if metrics == nil {
		metrics = []model.MetricRow{}
	}
	c.JSON(http.StatusOK, metrics)
}

func (a *API) handleListAlerts(c *gin.Context) {
	var filter model.AlertFilter
	if linkID := c.Query("link_id"); linkID != "" {
		filter.LinkID = &linkID
	}
	if typ := c.Query("type"); typ != "" {
		at := model.AlertType(typ)
		filter.Type = &at
	}
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}
	filter.Limit = 100

	alerts, err := a.Store.ListAlerts(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if alerts == nil {
		alerts = []model.Alert{}
	}
	c.JSON(http.StatusOK, alerts)
}
