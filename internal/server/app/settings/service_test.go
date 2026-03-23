package settings

import (
	"context"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func TestServiceDefaults(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	svc := &service{
		repos: Repositories{Settings: store.Settings},
		now: func() time.Time {
			return time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
		},
	}

	model, err := svc.GetModelConfig(context.Background(), "")
	if err != nil {
		t.Fatalf("GetModelConfig() error = %v", err)
	}
	if model.Provider != "openai" || model.Model != "gpt-5.4" || model.ApprovalMode != "balanced" {
		t.Fatalf("model = %#v, want default dev model config", model)
	}

	account, err := svc.GetAccountSecurity(context.Background(), "")
	if err != nil {
		t.Fatalf("GetAccountSecurity() error = %v", err)
	}
	if account.Username != "local-dev" || !account.MFAEnabled {
		t.Fatalf("account = %#v, want default account security", account)
	}

	preferences, err := svc.GetPreferences(context.Background(), "")
	if err != nil {
		t.Fatalf("GetPreferences() error = %v", err)
	}
	if preferences.Locale != "zh-CN" || preferences.DefaultMode != "ai_agent" {
		t.Fatalf("preferences = %#v, want default preferences", preferences)
	}
}

func TestServicePersistsUpdates(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	svc := NewService(Repositories{Settings: store.Settings})

	model, err := svc.PutModelConfig(context.Background(), "alice", UpdateModelConfigInput{
		Provider:     "OpenAI",
		Model:        "gpt-5.4-mini",
		Endpoint:     "https://api.openai.com/v1",
		APIKey:       "sk-dev",
		Temperature:  0.4,
		MaxTokens:    1024,
		TimeoutSec:   30,
		ApprovalMode: "safe",
	})
	if err != nil {
		t.Fatalf("PutModelConfig() error = %v", err)
	}
	if !model.HasAPIKey || model.Model != "gpt-5.4-mini" {
		t.Fatalf("model = %#v, want persisted api key and model", model)
	}
	if model.Provider != "openai" {
		t.Fatalf("model.Provider = %q, want openai", model.Provider)
	}

	preferences, err := svc.PutPreferences(context.Background(), "alice", UpdatePreferencesInput{
		PreferredRegion: "San Francisco",
		DefaultMode:     "direct_shell",
		Locale:          "en-US",
		CompactTimeline: true,
		StreamMarkdown:  false,
	})
	if err != nil {
		t.Fatalf("PutPreferences() error = %v", err)
	}
	if preferences.Locale != "en-US" || !preferences.CompactTimeline {
		t.Fatalf("preferences = %#v, want persisted preferences", preferences)
	}

	loaded, err := svc.GetPreferences(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetPreferences() error = %v", err)
	}
	if loaded.PreferredRegion != "San Francisco" || loaded.DefaultMode != "direct_shell" {
		t.Fatalf("loaded = %#v, want stored preferences", loaded)
	}
}

func TestServiceRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	svc := NewService(Repositories{Settings: store.Settings})

	if _, err := svc.PutModelConfig(context.Background(), "alice", UpdateModelConfigInput{
		Provider:     "OpenAI",
		Model:        "gpt-5.4",
		Temperature:  3,
		ApprovalMode: "balanced",
	}); err == nil {
		t.Fatal("PutModelConfig() error = nil, want invalid argument")
	}

	if err := svc.ChangePassword(context.Background(), "alice", ChangePasswordInput{
		CurrentPassword: "same",
		NewPassword:     "same",
	}); err == nil {
		t.Fatal("ChangePassword() error = nil, want invalid argument")
	}

	if _, err := svc.PutPreferences(context.Background(), "alice", UpdatePreferencesInput{
		PreferredRegion: "Tokyo",
		DefaultMode:     "unknown",
		Locale:          "zh-CN",
	}); err == nil {
		t.Fatal("PutPreferences() error = nil, want invalid argument")
	}
}

func TestServiceDelegatesSecurityOperations(t *testing.T) {
	store := memory.NewStore()
	security := &stubSecurityService{}
	svc := NewService(Repositories{
		Settings: store.Settings,
		Security: security,
	})

	if err := svc.ChangePassword(context.Background(), "alice", ChangePasswordInput{
		CurrentPassword: "old",
		NewPassword:     "new",
	}); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if security.userID != "alice" || security.currentPassword != "old" || security.newPassword != "new" {
		t.Fatalf("security = %#v, want delegated password change", security)
	}

	if err := svc.RevokeOtherSessions(context.Background(), "alice", "sess-1"); err != nil {
		t.Fatalf("RevokeOtherSessions() error = %v", err)
	}
	if security.revokedUserID != "alice" || security.currentSessionID != "sess-1" {
		t.Fatalf("security revoke = %#v, want delegated revoke", security)
	}
}

func TestServiceListsModelOptionsUsingStoredAPIKey(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	catalog := &stubModelCatalog{
		models: []ModelOption{
			{ID: "gpt-5.4", Label: "gpt-5.4"},
			{ID: "gpt-5.4-mini", Label: "gpt-5.4-mini"},
		},
	}
	svc := NewService(Repositories{
		Settings: store.Settings,
		Models:   catalog,
	})

	if _, err := svc.PutModelConfig(context.Background(), "alice", UpdateModelConfigInput{
		Provider:     "openai",
		Model:        "gpt-5.4",
		Endpoint:     "https://api.openai.com/v1",
		APIKey:       "sk-stored",
		Temperature:  0.2,
		ApprovalMode: "balanced",
	}); err != nil {
		t.Fatalf("PutModelConfig() error = %v", err)
	}

	models, err := svc.ListModelOptions(context.Background(), "alice", ListModelOptionsInput{
		Provider: "openai",
		Endpoint: "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("ListModelOptions() error = %v", err)
	}
	if len(models) != 2 || models[0].ID != "gpt-5.4" {
		t.Fatalf("models = %#v, want catalog output", models)
	}
	if catalog.provider != "openai" || catalog.endpoint != "https://api.openai.com/v1" || catalog.apiKey != "sk-stored" {
		t.Fatalf("catalog = %#v, want stored config forwarded", catalog)
	}
}

type stubSecurityService struct {
	userID           string
	currentPassword  string
	newPassword      string
	revokedUserID    string
	currentSessionID string
}

type stubModelCatalog struct {
	provider string
	endpoint string
	apiKey   string
	models   []ModelOption
}

func (s *stubSecurityService) ChangePassword(ctx context.Context, userID string, currentPassword string, newPassword string) error {
	_ = ctx
	s.userID = userID
	s.currentPassword = currentPassword
	s.newPassword = newPassword
	return nil
}

func (s *stubSecurityService) RevokeOtherSessions(ctx context.Context, userID string, currentSessionID string) error {
	_ = ctx
	s.revokedUserID = userID
	s.currentSessionID = currentSessionID
	return nil
}

func (s *stubModelCatalog) ListModels(ctx context.Context, provider string, endpoint string, apiKey string) ([]ModelOption, error) {
	_ = ctx
	s.provider = provider
	s.endpoint = endpoint
	s.apiKey = apiKey
	return append([]ModelOption(nil), s.models...), nil
}
