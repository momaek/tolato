package ginhttp

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appauth "github.com/momaek/tolato/internal/server/app/auth"
	appexecution "github.com/momaek/tolato/internal/server/app/execution"
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
	ListModelOptions(ctx context.Context, userID string, in settings.ListModelOptionsInput) ([]settings.ModelOption, error)
	GetAccountSecurity(ctx context.Context, userID string) (settings.AccountSecurityView, error)
	PutAccountSecurity(ctx context.Context, userID string, in settings.UpdateAccountSecurityInput) (settings.AccountSecurityView, error)
	ChangePassword(ctx context.Context, userID string, in settings.ChangePasswordInput) error
	RevokeOtherSessions(ctx context.Context, userID string, currentSessionID string) error
	GetPreferences(ctx context.Context, userID string) (settings.UserPreferencesView, error)
	PutPreferences(ctx context.Context, userID string, in settings.UpdatePreferencesInput) (settings.UserPreferencesView, error)
}

type AuthService interface {
	Login(ctx context.Context, username string, password string) (appauth.LoginResult, error)
	AuthenticateToken(ctx context.Context, token string) (appauth.Principal, error)
}

type ExecutionService interface {
	StartUpgrade(ctx context.Context, input appexecution.StartUpgradeInput) (appexecution.StartDispatchResult, error)
}

type Handler struct {
	Nodes      NodeViewService
	History    HistoryService
	Settings   SettingsService
	Auth       AuthService
	Execution  ExecutionService
	AgentToken string
}

func (h Handler) RegisterRoutes(router gin.IRouter) {
	api := router.Group("/api/v1")
	api.POST("/auth/login", h.login)

	protected := api.Group("")
	if h.Auth != nil {
		protected.Use(h.requireAuth())
	}
	protected.GET("/agent-token", h.getAgentToken)
	protected.GET("/nodes", h.listNodes)
	protected.GET("/nodes/:id", h.getNode)
	protected.POST("/nodes/:id/upgrade", h.upgradeNode)
	protected.GET("/history/tasks", h.listHistoryTasks)
	protected.GET("/history/tasks/:id", h.getHistoryTask)
	protected.GET("/settings/model-config", h.getModelConfig)
	protected.PUT("/settings/model-config", h.putModelConfig)
	protected.POST("/settings/model-config/test", h.testModelConfig)
	protected.POST("/settings/model-config/models", h.listModelOptions)
	protected.GET("/settings/account-security", h.getAccountSecurity)
	protected.PUT("/settings/account-security", h.putAccountSecurity)
	protected.POST("/settings/password/change", h.changePassword)
	protected.POST("/settings/sessions/revoke-others", h.revokeOtherSessions)
	protected.GET("/settings/preferences", h.getPreferences)
	protected.PUT("/settings/preferences", h.putPreferences)
}

func (h Handler) login(c *gin.Context) {
	if h.Auth == nil {
		writeError(c, http.StatusNotImplemented, "auth service is not configured")
		return
	}

	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.Auth.Login(c.Request.Context(), input.Username, input.Password)
	if err != nil {
		if errors.Is(err, appauth.ErrUnauthorized) {
			writeError(c, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h Handler) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			writeError(c, http.StatusUnauthorized, "missing bearer token")
			c.Abort()
			return
		}
		principal, err := h.Auth.AuthenticateToken(c.Request.Context(), token)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "invalid bearer token")
			c.Abort()
			return
		}
		c.Set("auth.user_id", principal.UserID)
		c.Set("auth.session_id", principal.SessionID)
		c.Next()
	}
}

func (h Handler) getAgentToken(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"agent_token": h.AgentToken})
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

func (h Handler) upgradeNode(c *gin.Context) {
	if h.Execution == nil {
		writeError(c, http.StatusNotImplemented, "execution service is not configured")
		return
	}

	nodeID := c.Param("id")
	var input struct {
		DownloadURL   string `json:"downloadUrl"`
		TargetVersion string `json:"targetVersion"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(input.DownloadURL) == "" {
		writeError(c, http.StatusBadRequest, "downloadUrl is required")
		return
	}

	result, err := h.Execution.StartUpgrade(c.Request.Context(), appexecution.StartUpgradeInput{
		SessionID:     "sess-console-1",
		NodeID:        nodeID,
		DownloadURL:   input.DownloadURL,
		TargetVersion: input.TargetVersion,
	})
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, result)
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

func (h Handler) listModelOptions(c *gin.Context) {
	if h.Settings == nil {
		writeError(c, http.StatusNotImplemented, "settings service is not configured")
		return
	}

	var input settings.ListModelOptionsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	items, err := h.Settings.ListModelOptions(c.Request.Context(), requestUserID(c), input)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": items})
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
	if value, ok := c.Get("auth.user_id"); ok {
		if userID, ok := value.(string); ok && strings.TrimSpace(userID) != "" {
			return strings.TrimSpace(userID)
		}
	}
	if value := strings.TrimSpace(c.GetHeader("X-User-ID")); value != "" {
		return value
	}
	return ""
}

func requestSessionID(c *gin.Context) string {
	if value, ok := c.Get("auth.session_id"); ok {
		if sessionID, ok := value.(string); ok && strings.TrimSpace(sessionID) != "" {
			return strings.TrimSpace(sessionID)
		}
	}
	if value := strings.TrimSpace(c.GetHeader("X-Session-ID")); value != "" {
		return value
	}
	return ""
}

func bearerToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
