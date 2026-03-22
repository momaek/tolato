package ginhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/internal/server/app/history"
	"github.com/momaek/tolato/internal/server/app/nodeview"
	"github.com/momaek/tolato/internal/server/app/settings"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestHandlerListNodesReturnsWrappedCollection(t *testing.T) {
	t.Parallel()

	router := gin.New()
	nodes := &fakeNodeViewService{
		items: []nodeview.NodeSummary{{
			ID:         "jp-tokyo-01",
			Hostname:   "jp-tokyo-01",
			Region:     "Tokyo",
			OS:         "Ubuntu 24.04",
			Version:    "1.28.3",
			Tags:       []string{"edge"},
			Status:     "busy",
			Busy:       true,
			LastSeenAt: "2026-03-22T12:00:00Z",
		}},
	}
	handler := Handler{
		Nodes: nodes,
	}
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?q=tokyo&busy=true&limit=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Nodes []nodeview.NodeSummary `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(body.Nodes) != 1 || body.Nodes[0].ID != "jp-tokyo-01" {
		t.Fatalf("body = %#v, want wrapped nodes list", body)
	}
	if nodes.lastFilter.Query != "tokyo" || nodes.lastFilter.Limit != 10 || nodes.lastFilter.Busy == nil || !*nodes.lastFilter.Busy {
		t.Fatal("expected query filter to be forwarded")
	}
}

func TestHandlerListHistoryTasksReturnsArray(t *testing.T) {
	t.Parallel()

	router := gin.New()
	handler := Handler{
		History: &fakeHistoryService{
			items: []history.TaskItem{{
				ID:             "task-1",
				Title:          "restart nginx",
				Summary:        "execution completed successfully",
				Status:         "success",
				ApprovalStatus: "approved",
				Risk:           "medium",
				TargetLabels:   []string{"jp-tokyo-01"},
				CreatedAt:      "2026-03-22T12:00:00Z",
				UpdatedAt:      "2026-03-22T12:00:00Z",
			}},
		},
	}
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history/tasks?status=success&approvalStatus=approved&q=nginx&limit=5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body []history.TaskItem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(body) != 1 || body[0].ID != "task-1" {
		t.Fatalf("body = %#v, want history task list", body)
	}
	if handler.History.(*fakeHistoryService).lastFilter.Query != "nginx" || handler.History.(*fakeHistoryService).lastFilter.Limit != 5 {
		t.Fatalf("expected history filter to be forwarded: %#v", handler.History.(*fakeHistoryService).lastFilter)
	}
}

func TestHandlerGetHistoryTaskReturnsDetail(t *testing.T) {
	t.Parallel()

	router := gin.New()
	handler := Handler{
		History: &fakeHistoryService{
			detail: history.TaskDetail{
				TaskItem: history.TaskItem{
					ID:             "task-1",
					Title:          "restart nginx",
					Summary:        "execution completed successfully",
					Status:         "success",
					ApprovalStatus: "approved",
					Risk:           "medium",
					TargetLabels:   []string{"jp-tokyo-01"},
					CreatedAt:      "2026-03-22T12:00:00Z",
					UpdatedAt:      "2026-03-22T12:00:00Z",
				},
				Impact:    "Moderate-risk operation targeting jp-tokyo-01.",
				Steps:     []string{"Resolve target scope: jp-tokyo-01"},
				AISummary: "execution completed successfully",
			},
		},
	}
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history/tasks/task-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body history.TaskDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.ID != "task-1" || body.AISummary == "" {
		t.Fatalf("body = %#v, want history detail", body)
	}
}

func TestHandlerGetNodeReturnsDetail(t *testing.T) {
	t.Parallel()

	router := gin.New()
	nodes := &fakeNodeViewService{
		detail: nodeview.NodeDetail{
			NodeSummary: nodeview.NodeSummary{
				ID:         "jp-tokyo-01",
				Hostname:   "jp-tokyo-01",
				Region:     "Tokyo",
				OS:         "Ubuntu 24.04",
				Version:    "1.28.3",
				Tags:       []string{"edge"},
				Status:     "busy",
				Busy:       true,
				LastSeenAt: "2026-03-22T12:00:00Z",
			},
			Provider:   "unknown",
			IPAddress:  "-",
			Kernel:     "-",
			Uptime:     "-",
			AgentVer:   "-",
			RiskSignal: []string{},
			RecentTask: []nodeview.NodeTaskSummary{},
		},
	}
	handler := Handler{
		Nodes: nodes,
	}
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/jp-tokyo-01", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body nodeview.NodeDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.ID != "jp-tokyo-01" || body.Provider != "unknown" {
		t.Fatalf("body = %#v, want node detail payload", body)
	}
}

func TestHandlerSettingsRoundTrip(t *testing.T) {
	t.Parallel()

	router := gin.New()
	handler := Handler{
		Settings: &fakeSettingsService{
			model: settings.ModelConfigView{
				Provider:     "OpenAI",
				Model:        "gpt-5.4",
				Temperature:  0.2,
				ApprovalMode: "balanced",
				HasAPIKey:    true,
			},
			account: settings.AccountSecurityView{
				Username:           "admin",
				LastLoginAt:        "2026-03-22T08:00:00Z",
				MFAEnabled:         true,
				AuditRetentionDays: 90,
			},
			preferences: settings.UserPreferencesView{
				PreferredRegion: "Tokyo",
				DefaultMode:     "ai_agent",
				Locale:          "zh-CN",
				CompactTimeline: false,
				StreamMarkdown:  true,
			},
		},
	}
	handler.RegisterRoutes(router)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings/preferences", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /preferences status = %d, want 200", getRec.Code)
	}

	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/settings/model-config", strings.NewReader(`{"provider":"OpenAI","model":"gpt-5.4-mini","temperature":0.3,"approvalMode":"safe"}`))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT /model-config status = %d, want 200", putRec.Code)
	}

	var body settings.ModelConfigView
	if err := json.Unmarshal(putRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.Model != "gpt-5.4-mini" || body.ApprovalMode != "safe" {
		t.Fatalf("body = %#v, want updated model config", body)
	}
}

func TestHandlerSettingsPasswordChange(t *testing.T) {
	t.Parallel()

	router := gin.New()
	handler := Handler{
		Settings: &fakeSettingsService{},
	}
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/password/change", strings.NewReader(`{"currentPassword":"old","newPassword":"new"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestHandlerGetNodeMapsNotFound(t *testing.T) {
	t.Parallel()

	router := gin.New()
	handler := Handler{Nodes: &fakeNodeViewService{getErr: domain.ErrNotFound}}
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/missing", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlerListNodesRejectsInvalidBusyQuery(t *testing.T) {
	t.Parallel()

	router := gin.New()
	handler := Handler{Nodes: &fakeNodeViewService{}}
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?busy=not-bool", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

type fakeNodeViewService struct {
	items      []nodeview.NodeSummary
	detail     nodeview.NodeDetail
	listErr    error
	getErr     error
	lastFilter nodeview.ListFilter
}

type fakeHistoryService struct {
	items      []history.TaskItem
	detail     history.TaskDetail
	listErr    error
	getErr     error
	lastFilter history.ListFilter
}

type fakeSettingsService struct {
	model       settings.ModelConfigView
	account     settings.AccountSecurityView
	preferences settings.UserPreferencesView
	err         error
}

func (f *fakeHistoryService) ListTasks(ctx context.Context, filter history.ListFilter) ([]history.TaskItem, error) {
	f.lastFilter = filter
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]history.TaskItem(nil), f.items...), nil
}

func (f *fakeHistoryService) GetTaskDetail(ctx context.Context, taskID string) (history.TaskDetail, error) {
	if f.getErr != nil {
		return history.TaskDetail{}, f.getErr
	}
	return f.detail, nil
}

func (f *fakeNodeViewService) ListNodes(ctx context.Context, filter nodeview.ListFilter) ([]nodeview.NodeSummary, error) {
	f.lastFilter = filter
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]nodeview.NodeSummary(nil), f.items...), nil
}

func (f *fakeNodeViewService) GetNode(ctx context.Context, nodeID string) (nodeview.NodeDetail, error) {
	if f.getErr != nil {
		return nodeview.NodeDetail{}, f.getErr
	}
	return f.detail, nil
}

func (f *fakeSettingsService) GetModelConfig(ctx context.Context, userID string) (settings.ModelConfigView, error) {
	return f.model, f.err
}

func (f *fakeSettingsService) PutModelConfig(ctx context.Context, userID string, in settings.UpdateModelConfigInput) (settings.ModelConfigView, error) {
	if f.err != nil {
		return settings.ModelConfigView{}, f.err
	}
	f.model = settings.ModelConfigView{
		Provider:     in.Provider,
		Model:        in.Model,
		Endpoint:     in.Endpoint,
		Temperature:  in.Temperature,
		MaxTokens:    in.MaxTokens,
		TimeoutSec:   in.TimeoutSec,
		ApprovalMode: in.ApprovalMode,
		HasAPIKey:    in.APIKey != "",
	}
	return f.model, nil
}

func (f *fakeSettingsService) TestModelConfig(ctx context.Context, userID string, in settings.TestModelConfigInput) (settings.ModelConfigTestResult, error) {
	if f.err != nil {
		return settings.ModelConfigTestResult{}, f.err
	}
	return settings.ModelConfigTestResult{OK: true, Message: "connection test succeeded"}, nil
}

func (f *fakeSettingsService) GetAccountSecurity(ctx context.Context, userID string) (settings.AccountSecurityView, error) {
	return f.account, f.err
}

func (f *fakeSettingsService) PutAccountSecurity(ctx context.Context, userID string, in settings.UpdateAccountSecurityInput) (settings.AccountSecurityView, error) {
	if f.err != nil {
		return settings.AccountSecurityView{}, f.err
	}
	f.account = settings.AccountSecurityView{
		Username:           in.Username,
		LastLoginAt:        in.LastLoginAt,
		MFAEnabled:         in.MFAEnabled,
		AuditRetentionDays: in.AuditRetentionDays,
	}
	return f.account, nil
}

func (f *fakeSettingsService) ChangePassword(ctx context.Context, userID string, in settings.ChangePasswordInput) error {
	return f.err
}

func (f *fakeSettingsService) RevokeOtherSessions(ctx context.Context, userID string, currentSessionID string) error {
	return f.err
}

func (f *fakeSettingsService) GetPreferences(ctx context.Context, userID string) (settings.UserPreferencesView, error) {
	return f.preferences, f.err
}

func (f *fakeSettingsService) PutPreferences(ctx context.Context, userID string, in settings.UpdatePreferencesInput) (settings.UserPreferencesView, error) {
	if f.err != nil {
		return settings.UserPreferencesView{}, f.err
	}
	f.preferences = settings.UserPreferencesView{
		PreferredRegion: in.PreferredRegion,
		DefaultMode:     in.DefaultMode,
		Locale:          in.Locale,
		CompactTimeline: in.CompactTimeline,
		StreamMarkdown:  in.StreamMarkdown,
	}
	return f.preferences, nil
}
