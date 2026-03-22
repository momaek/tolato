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
	if model.Provider != "OpenAI" || model.Model != "gpt-5.4" || model.ApprovalMode != "balanced" {
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
