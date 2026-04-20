package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/security"
	"github.com/momaek/tolato/server/internal/store"
)

// External API — /api/v1/

type ExecuteCommandRequest struct {
	Command string `json:"command" binding:"required"`
	Timeout int    `json:"timeout"`
	Confirm bool   `json:"confirm"`
}

// ExternalListNodes returns all nodes for external API consumers.
func ExternalListNodes(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodes, _, err := store.ListNodes(1, 200, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: "Failed to list nodes"})
			return
		}

		items := make([]model.NodeListItem, 0, len(nodes))
		for _, n := range nodes {
			item := model.NodeListItem{
				ID:     n.ID,
				Name:   n.Name,
				Alias:  n.Alias,
				IP:     n.IP,
				Status: n.Status,
				OS:     n.OS,
			}
			if metrics := deps.NodeManager.GetMetrics(n.ID); metrics != nil {
				item.CPU = &metrics.CPU
				item.Memory = &metrics.Memory
				item.Disk = &metrics.Disk
			}
			item.LastHeartbeat = n.LastHeartbeat
			items = append(items, item)
		}
		c.JSON(http.StatusOK, items)
	}
}

// ExternalGetNode returns node detail for external API.
func ExternalGetNode(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		n, err := store.GetNodeByID(id)
		if err != nil {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "Node not found"})
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
		}
		if metrics := deps.NodeManager.GetMetrics(n.ID); metrics != nil {
			detail.Metrics = metrics
		}
		c.JSON(http.StatusOK, detail)
	}
}

// ExternalExecuteCommand runs a command on a node for external API.
func ExternalExecuteCommand(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")
		permission := c.GetString("api_key_permission")
		apiKeyID := c.GetString("api_key_id")

		// Readonly keys cannot execute commands
		if permission == "readonly" {
			c.JSON(http.StatusForbidden, model.ErrorResponse{
				Error:   "forbidden",
				Message: "Read-only API keys cannot execute commands",
			})
			return
		}

		var req ExecuteCommandRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_request", Message: err.Error()})
			return
		}

		if req.Timeout <= 0 {
			req.Timeout = 60
		}

		// Check sensitive operation
		checker := security.NewChecker(deps.Settings)
		if permission != "admin" && checker.IsSensitive(req.Command) && !req.Confirm {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error:   "sensitive_operation",
				Message: "This command requires confirmation. Set confirm: true to proceed.",
			})
			return
		}

		// Check blacklist
		if checker.IsBlacklisted(req.Command) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{
				Error:   "blacklisted",
				Message: "This command is blacklisted",
			})
			return
		}

		// Get node info for audit
		n, err := store.GetNodeByID(nodeID)
		if err != nil {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "Node not found"})
			return
		}

		// Execute
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(req.Timeout+30)*time.Second)
		defer cancel()

		result, err := deps.NodeManager.ExecuteCommand(ctx, nodeID, req.Command, req.Timeout)
		if err != nil {
			// Log failed attempt
			store.CreateAuditLog(&model.AuditLog{
				NodeID:   nodeID,
				NodeName: n.Name,
				Command:  req.Command,
				Source:   "api",
				APIKeyID: &apiKeyID,
			})
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "execution_failed",
				Message: err.Error(),
			})
			return
		}

		// Audit log
		stdout := result.Stdout
		stderr := result.Stderr
		store.CreateAuditLog(&model.AuditLog{
			NodeID:     nodeID,
			NodeName:   n.Name,
			Command:    req.Command,
			ExitCode:   &result.ExitCode,
			Stdout:     &stdout,
			Stderr:     &stderr,
			DurationMS: &result.DurationMS,
			Confirmed:  req.Confirm || permission == "admin",
			Source:     "api",
			APIKeyID:   &apiKeyID,
		})

		c.JSON(http.StatusOK, gin.H{
			"id":          "",
			"node_id":     nodeID,
			"command":     req.Command,
			"exit_code":   result.ExitCode,
			"stdout":      result.Stdout,
			"stderr":      result.Stderr,
			"duration_ms": result.DurationMS,
		})
	}
}

// VerifyLLMSettings verifies LLM API configuration and returns available models.
// Accepts an optional body {api_base_url, api_key} so the user can verify form values
// before saving. Empty fields fall back to stored settings, which lets the UI omit
// the masked api_key when it hasn't been edited.
func VerifyLLMSettings(deps *Deps) gin.HandlerFunc {
	type verifyReq struct {
		APIBaseURL string `json:"api_base_url"`
		APIKey     string `json:"api_key"`
	}
	return func(c *gin.Context) {
		var req verifyReq
		_ = c.ShouldBindJSON(&req)

		stored := deps.Settings.LLM()
		baseURL := strings.TrimSpace(req.APIBaseURL)
		if baseURL == "" {
			baseURL = stored.APIBaseURL
		}
		apiKey := strings.TrimSpace(req.APIKey)
		if apiKey == "" {
			apiKey = stored.APIKey
		}

		if baseURL == "" || apiKey == "" {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "incomplete_config",
				Message: "API base URL and API key are required",
			})
			return
		}

		// Normalize so users can paste the URL with or without `/v1`. The chat
		// client applies the same normalization at llm.ClientConfig build time.
		baseURL = normalizeLLMBaseURL(baseURL)

		client := &http.Client{Timeout: 10 * time.Second}
		httpReq, err := http.NewRequestWithContext(c.Request.Context(), "GET", baseURL+"/models", nil)
		if err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_url", Message: err.Error()})
			return
		}
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(httpReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, model.ErrorResponse{
				Error:   "connection_failed",
				Message: "Failed to connect to LLM API: " + err.Error(),
			})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			c.JSON(http.StatusBadGateway, model.ErrorResponse{
				Error:   "api_error",
				Message: "LLM API returned status " + resp.Status,
			})
			return
		}

		var result struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			// 200 with non-JSON (likely the wrong endpoint hit a SPA / 404 page).
			// Don't claim success.
			c.JSON(http.StatusBadGateway, model.ErrorResponse{
				Error:   "invalid_response",
				Message: "LLM API returned non-JSON response",
			})
			return
		}

		models := make([]string, 0, len(result.Data))
		for _, m := range result.Data {
			models = append(models, m.ID)
		}

		// Cache the model list so the chat UI can populate the dropdown without
		// hitting the upstream API on every page open.
		if b, err := json.Marshal(models); err == nil {
			_ = store.SetSetting("llm.cached_models", string(b))
		}

		// Field name `success` matches the frontend VerifyLLMResponse type.
		c.JSON(http.StatusOK, gin.H{"success": true, "models": models})
	}
}

// GetLLMModels returns the cached list of models from the last successful verify.
func GetLLMModels(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		models := []string{}
		if s, err := store.GetSetting("llm.cached_models"); err == nil {
			_ = json.Unmarshal([]byte(s.Value), &models)
		}
		c.JSON(http.StatusOK, gin.H{"models": models})
	}
}

// ListNodeCommands returns command execution history for a specific node.
func ListNodeCommands(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")
		var query model.PaginationQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			query.Page = 1
			query.PageSize = 20
		}
		if query.Page < 1 {
			query.Page = 1
		}
		if query.PageSize < 1 || query.PageSize > 100 {
			query.PageSize = 20
		}

		logs, total, err := store.ListAuditLogs(query.Page, query.PageSize, &nodeID, nil, nil, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: "Failed to list commands"})
			return
		}

		items := make([]model.AuditLogItem, 0, len(logs))
		for _, l := range logs {
			items = append(items, model.AuditLogItem{
				ID:         l.ID,
				NodeID:     l.NodeID,
				NodeName:   l.NodeName,
				Command:    l.Command,
				ExitCode:   l.ExitCode,
				Stdout:     l.Stdout,
				Stderr:     l.Stderr,
				DurationMS: l.DurationMS,
				Confirmed:  l.Confirmed,
				Source:     l.Source,
				CreatedAt:  l.CreatedAt,
			})
		}

		totalPages := int(total) / query.PageSize
		if int(total)%query.PageSize > 0 {
			totalPages++
		}

		c.JSON(http.StatusOK, model.PaginatedResponse{
			Items:      items,
			Total:      int(total),
			Page:       query.Page,
			PageSize:   query.PageSize,
			TotalPages: totalPages,
		})
	}
}
