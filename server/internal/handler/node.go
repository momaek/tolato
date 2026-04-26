package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// publicServerURLs returns (curlURL, agentServerArg) used to build the install
// command. curlURL always has an http(s) scheme; agentServerArg is whatever
// install.sh should pass to `--server` (install.sh will normalize to ws(s)://).
//
// Preference order:
//  1. server.public_address (required when behind caddy/nginx, since the bind
//     host may be 0.0.0.0 or a private IP that external agents can't reach)
//  2. fallback: host:port (same-host / dev only)
func publicServerURLs(cfg *config.Config) (curlURL, serverArg string) {
	pub := strings.TrimRight(cfg.Server.PublicAddress, "/")
	if pub != "" {
		serverArg = pub
		switch {
		case strings.HasPrefix(pub, "http://"), strings.HasPrefix(pub, "https://"):
			curlURL = pub
		case strings.HasPrefix(pub, "wss://"):
			curlURL = "https://" + strings.TrimPrefix(pub, "wss://")
		case strings.HasPrefix(pub, "ws://"):
			curlURL = "http://" + strings.TrimPrefix(pub, "ws://")
		default:
			curlURL = "http://" + pub
		}
		return
	}
	hostPort := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return "http://" + hostPort, hostPort
}

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

		curlURL, serverArg := publicServerURLs(deps.Config)
		installCmd := fmt.Sprintf("curl -fsSL %s/install.sh | sudo bash -s -- --token %s --server %s", curlURL, token.ID, serverArg)

		tokenExpiry := ""
		if deps.Config.Security.AgentTokenExpiry > 0 {
			tokenExpiry = deps.Config.Security.AgentTokenExpiry.String()
		}

		c.JSON(http.StatusCreated, model.CreateNodeResponse{
			Token:       token.ID,
			InstallCmd:  installCmd,
			TokenExpiry: tokenExpiry,
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
				CountryCode:   n.CountryCode,
				City:          n.City,
				ASN:           n.ASN,
				Status:        n.Status,
				OS:            n.OS,
				Extra:         n.Extra,
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
			CountryCode:   n.CountryCode,
			City:          n.City,
			ASN:           n.ASN,
			Extra:         n.Extra,
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
		if req.Extra != nil {
			merged, err := mergeNodeExtra(id, req.Extra)
			if err != nil {
				c.JSON(http.StatusInternalServerError, model.ErrorResponse{
					Error:   "internal_error",
					Message: "failed to merge extra",
				})
				return
			}
			updates["extra"] = merged
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

// mergeNodeExtra applies a partial patch to a node's existing Extra map.
// Keys with non-nil values are upserted; keys with explicit nil values are
// deleted. Reads-then-writes — callers should treat concurrent edits to the
// same node as last-writer-wins.
func mergeNodeExtra(nodeID string, patch map[string]any) (model.JSONMap, error) {
	n, err := store.GetNodeByID(nodeID)
	if err != nil {
		return nil, err
	}
	merged := model.JSONMap{}
	for k, v := range n.Extra {
		merged[k] = v
	}
	for k, v := range patch {
		if v == nil {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}
	return merged, nil
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
