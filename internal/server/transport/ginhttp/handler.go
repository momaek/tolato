package ginhttp

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/internal/server/app/history"
	"github.com/momaek/tolato/internal/server/app/nodeview"
	"github.com/momaek/tolato/internal/server/app/settings"
	"github.com/momaek/tolato/internal/server/domain"
)

type NodeViewService interface {
	ListNodes(ctx context.Context, filter nodeview.ListFilter) ([]nodeview.NodeSummary, error)
	GetNode(ctx context.Context, nodeID string) (nodeview.NodeDetail, error)
}

type HistoryService interface {
	ListTasks(ctx context.Context, filter history.ListFilter) ([]history.TaskItem, error)
	GetTaskDetail(ctx context.Context, taskID string) (history.TaskDetail, error)
}

type SettingsService interface {
	GetModelConfig(ctx context.Context, userID string) (settings.ModelConfigView, error)
	PutModelConfig(ctx context.Context, userID string, in settings.UpdateModelConfigInput) (settings.ModelConfigView, error)
	TestModelConfig(ctx context.Context, userID string, in settings.TestModelConfigInput) (settings.ModelConfigTestResult, error)
	GetAccountSecurity(ctx context.Context, userID string) (settings.AccountSecurityView, error)
	PutAccountSecurity(ctx context.Context, userID string, in settings.UpdateAccountSecurityInput) (settings.AccountSecurityView, error)
	ChangePassword(ctx context.Context, userID string, in settings.ChangePasswordInput) error
	RevokeOtherSessions(ctx context.Context, userID string, currentSessionID string) error
	GetPreferences(ctx context.Context, userID string) (settings.UserPreferencesView, error)
	PutPreferences(ctx context.Context, userID string, in settings.UpdatePreferencesInput) (settings.UserPreferencesView, error)
}

type Handler struct {
	Nodes    NodeViewService
	History  HistoryService
	Settings SettingsService
}

func (h Handler) RegisterRoutes(router gin.IRouter) {
	api := router.Group("/api/v1")
	api.GET("/nodes", h.listNodes)
	api.GET("/nodes/:id", h.getNode)
	api.GET("/history/tasks", h.listHistoryTasks)
	api.GET("/history/tasks/:id", h.getHistoryTask)
	api.GET("/settings/model-config", h.getModelConfig)
	api.PUT("/settings/model-config", h.putModelConfig)
	api.POST("/settings/model-config/test", h.testModelConfig)
	api.GET("/settings/account-security", h.getAccountSecurity)
	api.PUT("/settings/account-security", h.putAccountSecurity)
	api.POST("/settings/password/change", h.changePassword)
	api.POST("/settings/sessions/revoke-others", h.revokeOtherSessions)
	api.GET("/settings/preferences", h.getPreferences)
	api.PUT("/settings/preferences", h.putPreferences)
}

func (h Handler) listNodes(c *gin.Context) {
	if h.Nodes == nil {
		writeError(c, http.StatusNotImplemented, "node view service is not configured")
		return
	}

	filter, err := parseListFilter(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	items, err := h.Nodes.ListNodes(c.Request.Context(), filter)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": items})
}

func (h Handler) getNode(c *gin.Context) {
	if h.Nodes == nil {
		writeError(c, http.StatusNotImplemented, "node view service is not configured")
		return
	}

	item, err := h.Nodes.GetNode(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h Handler) listHistoryTasks(c *gin.Context) {
	if h.History == nil {
		writeError(c, http.StatusNotImplemented, "history service is not configured")
		return
	}

	filter, err := parseHistoryListFilter(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	items, err := h.History.ListTasks(c.Request.Context(), filter)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func parseHistoryListFilter(c *gin.Context) (history.ListFilter, error) {
	filter := history.ListFilter{
		Query: c.Query("q"),
	}

	if rawLimit, ok := c.GetQuery("limit"); ok {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 0 {
			return history.ListFilter{}, errors.New("invalid limit query parameter")
		}
		filter.Limit = limit
	}
	if rawStatuses := strings.TrimSpace(c.Query("status")); rawStatuses != "" {
		values, err := parseCSVStatuses[domain.TaskStatus](rawStatuses, map[string]domain.TaskStatus{
			string(domain.TaskStatusPlanned):         domain.TaskStatusPlanned,
			string(domain.TaskStatusWaitingApproval): domain.TaskStatusWaitingApproval,
			string(domain.TaskStatusApproved):        domain.TaskStatusApproved,
			string(domain.TaskStatusQueued):          domain.TaskStatusQueued,
			string(domain.TaskStatusDispatched):      domain.TaskStatusDispatched,
			string(domain.TaskStatusRunning):         domain.TaskStatusRunning,
			string(domain.TaskStatusSuccess):         domain.TaskStatusSuccess,
			string(domain.TaskStatusFailed):          domain.TaskStatusFailed,
			string(domain.TaskStatusPartialFailed):   domain.TaskStatusPartialFailed,
			string(domain.TaskStatusTimeout):         domain.TaskStatusTimeout,
			string(domain.TaskStatusCancelled):       domain.TaskStatusCancelled,
		})
		if err != nil {
			return history.ListFilter{}, err
		}
		filter.Statuses = values
	}
	if rawApprovals := strings.TrimSpace(c.Query("approvalStatus")); rawApprovals != "" {
		values, err := parseCSVStatuses[domain.ApprovalStatus](rawApprovals, map[string]domain.ApprovalStatus{
			string(domain.ApprovalStatusNotRequired): domain.ApprovalStatusNotRequired,
			string(domain.ApprovalStatusPending):     domain.ApprovalStatusPending,
			string(domain.ApprovalStatusApproved):    domain.ApprovalStatusApproved,
			string(domain.ApprovalStatusRejected):    domain.ApprovalStatusRejected,
			string(domain.ApprovalStatusCancelled):   domain.ApprovalStatusCancelled,
		})
		if err != nil {
			return history.ListFilter{}, err
		}
		filter.ApprovalStatuses = values
	}

	return filter, nil
}

func (h Handler) getHistoryTask(c *gin.Context) {
	if h.History == nil {
		writeError(c, http.StatusNotImplemented, "history service is not configured")
		return
	}

	item, err := h.History.GetTaskDetail(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h Handler) getModelConfig(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	item, err := h.Settings.GetModelConfig(c.Request.Context(), requestUserID(c))
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h Handler) putModelConfig(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	var input settings.UpdateModelConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.Settings.PutModelConfig(c.Request.Context(), requestUserID(c), input)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h Handler) testModelConfig(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	var input settings.TestModelConfigInput
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&input); err != nil {
			writeError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	result, err := h.Settings.TestModelConfig(c.Request.Context(), requestUserID(c), input)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h Handler) getAccountSecurity(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	item, err := h.Settings.GetAccountSecurity(c.Request.Context(), requestUserID(c))
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h Handler) putAccountSecurity(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	var input settings.UpdateAccountSecurityInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.Settings.PutAccountSecurity(c.Request.Context(), requestUserID(c), input)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h Handler) changePassword(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	var input settings.ChangePasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Settings.ChangePassword(c.Request.Context(), requestUserID(c), input); err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "password changed"})
}

func (h Handler) revokeOtherSessions(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	if err := h.Settings.RevokeOtherSessions(c.Request.Context(), requestUserID(c), requestSessionID(c)); err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "other sessions revoked"})
}

func (h Handler) getPreferences(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	item, err := h.Settings.GetPreferences(c.Request.Context(), requestUserID(c))
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h Handler) putPreferences(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	var input settings.UpdatePreferencesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.Settings.PutPreferences(c.Request.Context(), requestUserID(c), input)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func parseListFilter(c *gin.Context) (nodeview.ListFilter, error) {
	filter := nodeview.ListFilter{
		Query:  c.Query("q"),
		Status: c.Query("status"),
		Region: c.Query("region"),
		Tag:    c.Query("tag"),
	}

	if rawBusy, ok := c.GetQuery("busy"); ok {
		busy, err := strconv.ParseBool(rawBusy)
		if err != nil {
			return nodeview.ListFilter{}, errors.New("invalid busy query parameter")
		}
		filter.Busy = &busy
	}
	if rawLimit, ok := c.GetQuery("limit"); ok {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 0 {
			return nodeview.ListFilter{}, errors.New("invalid limit query parameter")
		}
		filter.Limit = limit
	}
	return filter, nil
}

func parseCSVStatuses[T comparable](raw string, allowed map[string]T) ([]T, error) {
	parts := strings.Split(raw, ",")
	out := make([]T, 0, len(parts))
	for _, part := range parts {
		key := strings.ToLower(strings.TrimSpace(part))
		value, ok := allowed[key]
		if !ok {
			return nil, errors.New("invalid status query parameter")
		}
		out = append(out, value)
	}
	return out, nil
}

func writeDomainError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidArgument):
		writeError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrUnsupportedConfig):
		writeError(c, http.StatusNotImplemented, err.Error())
	default:
		writeError(c, http.StatusInternalServerError, err.Error())
	}
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func requestUserID(c *gin.Context) string {
	if value := strings.TrimSpace(c.GetHeader("X-User-ID")); value != "" {
		return value
	}
	return "local-dev"
}

func requestSessionID(c *gin.Context) string {
	if value := strings.TrimSpace(c.GetHeader("X-Session-ID")); value != "" {
		return value
	}
	return "dev-session"
}
