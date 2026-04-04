package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// CreateNode handles POST /api/nodes.
// This only generates a reusable registration token; the actual Node record
// is created when an agent connects and sends its register message.
func CreateNode(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.CreateNodeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			// Allow empty body
			req = model.CreateNodeRequest{}
		}

		token, err := store.CreateRegistrationToken(req.Alias, deps.Config.Security.AgentTokenExpiry)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to create registration token",
			})
			return
		}

		serverAddr := fmt.Sprintf("%s:%d", deps.Config.Server.Host, deps.Config.Server.Port)
		installCmd := fmt.Sprintf("curl -fsSL http://%s/install.sh | bash -s -- --token %s --server %s", serverAddr, token.ID, serverAddr)

		c.JSON(http.StatusCreated, model.CreateNodeResponse{
			Token:       token.ID,
			InstallCmd:  installCmd,
			TokenExpiry: deps.Config.Security.AgentTokenExpiry.String(),
		})
	}
}

// ListNodes handles GET /api/nodes.
func ListNodes(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.PaginationQuery
		if err := c.ShouldBindQuery(&q); err != nil {
			q = model.PaginationQuery{}
		}
		if q.Page <= 0 {
			q.Page = 1
		}
		if q.PageSize <= 0 || q.PageSize > 100 {
			q.PageSize = 20
		}

		status := c.Query("status")

		nodes, total, err := store.ListNodes(q.Page, q.PageSize, status)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to list nodes",
			})
			return
		}

		items := make([]model.NodeListItem, 0, len(nodes))
		for _, n := range nodes {
			item := model.NodeListItem{
				ID:            n.ID,
				Name:          n.Name,
				Alias:         n.Alias,
				IP:            n.IP,
				Status:        n.Status,
				OS:            n.OS,
				LastHeartbeat: n.LastHeartbeat,
			}

			// Attach cached metrics if online
			if metrics := deps.NodeManager.GetMetrics(n.ID); metrics != nil {
				item.CPU = &metrics.CPU
				item.Memory = &metrics.Memory
				item.Disk = &metrics.Disk
			}

			items = append(items, item)
		}

		totalPages := int(total) / q.PageSize
		if int(total)%q.PageSize > 0 {
			totalPages++
		}

		c.JSON(http.StatusOK, model.PaginatedResponse{
			Items:      items,
			Total:      int(total),
			Page:       q.Page,
			PageSize:   q.PageSize,
			TotalPages: totalPages,
		})
	}
}

// GetNode handles GET /api/nodes/:id.
func GetNode(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		n, err := store.GetNodeByID(id)
		if err != nil {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "node not found",
			})
			return
		}

		detail := model.NodeDetail{
			ID:            n.ID,
			Name:          n.Name,
			Alias:         n.Alias,
			IP:            n.IP,
			OS:            n.OS,
			Kernel:        n.Kernel,
			AgentVersion:  n.AgentVersion,
			CPUCores:      n.CPUCores,
			MemoryTotalMB: n.MemoryTotalMB,
			DiskTotalGB:   n.DiskTotalGB,
			Status:        n.Status,
			LastHeartbeat: n.LastHeartbeat,
			CreatedAt:     n.CreatedAt,
			Metrics:       deps.NodeManager.GetMetrics(n.ID),
		}

		c.JSON(http.StatusOK, detail)
	}
}

// UpdateNode handles PUT /api/nodes/:id.
func UpdateNode(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		var req model.UpdateNodeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "invalid request body",
			})
			return
		}

		updates := make(map[string]any)
		if req.Alias != nil {
			updates["alias"] = *req.Alias
		}

		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "no fields to update",
			})
			return
		}

		if err := store.UpdateNode(id, updates); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to update node",
			})
			return
		}

		n, _ := store.GetNodeByID(id)
		c.JSON(http.StatusOK, n)
	}
}

// DeleteNode handles DELETE /api/nodes/:id.
func DeleteNode(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		// Remove connection if online
		deps.NodeManager.RemoveConn(id)

		if err := store.DeleteNode(id); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to delete node",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}
