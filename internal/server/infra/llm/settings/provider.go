package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/runtime"
	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra/llm/agentsdk"
	devllm "github.com/momaek/tolato/internal/server/infra/llm/devloop"
)

type Provider struct {
	Settings      domain.SettingsRepository
	DefaultUserID string
	Logger        domain.Logger
	Events        runtime.EventPublisher
	IDGenerator   domain.IDGenerator
}

type modelConfig struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Endpoint     string  `json:"endpoint,omitempty"`
	APIKey       string  `json:"apiKey,omitempty"`
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"maxTokens,omitempty"`
	TimeoutSec   int     `json:"timeoutSec,omitempty"`
	ApprovalMode string  `json:"approvalMode"`
}

func (p *Provider) RunTurn(ctx context.Context, input runtime.ModelTurnInput, tools []agentapi.ToolSpec) (runtime.ModelTurnOutput, error) {
	cfg, err := p.loadConfig(ctx)
	if err != nil {
		p.logError(ctx, "llm settings provider failed to load config", "error", err)
		return runtime.ModelTurnOutput{}, err
	}

	providerName := normalizeProvider(cfg.Provider)
	p.logInfo(ctx, "llm turn requested",
		"provider", providerName,
		"model", strings.TrimSpace(cfg.Model),
		"session_id", input.SessionID,
		"conversation_items", len(input.Conversation),
		"tool_count", len(tools),
	)

	var out runtime.ModelTurnOutput
	switch providerName {
	case "devloop":
		out, err = devllm.New().RunTurn(ctx, input, tools)
	case "openai", "anthropic", "gemini":
		client := agentsdk.NewProvider(agentsdk.ProviderConfig{
			ProviderType:   providerName,
			Model:          cfg.Model,
			APIKey:         cfg.APIKey,
			Endpoint:       cfg.Endpoint,
			Temperature:    cfg.Temperature,
			MaxTokens:      cfg.MaxTokens,
			EnableThinking: true,
		}, p.Events, p.idGenerator())
		out, err = client.RunTurn(ctx, input, tools)
	default:
		return runtime.ModelTurnOutput{}, fmt.Errorf("unsupported llm provider %q", cfg.Provider)
	}
	if err != nil {
		p.logError(ctx, "llm turn failed",
			"provider", providerName,
			"model", strings.TrimSpace(cfg.Model),
			"session_id", input.SessionID,
			"error", err,
		)
		return runtime.ModelTurnOutput{}, err
	}
	p.logInfo(ctx, "llm turn completed",
		"provider", providerName,
		"model", strings.TrimSpace(cfg.Model),
		"session_id", input.SessionID,
		"done", out.Done,
		"output_items", len(out.Items),
		"tool_name", toolName(out.Items),
	)
	return out, nil
}

func (p *Provider) loadConfig(ctx context.Context) (modelConfig, error) {
	if p.Settings == nil {
		return modelConfig{}, domain.ErrUnsupportedConfig
	}
	record, err := p.Settings.Get(ctx, p.userID(), domain.SettingKeyModelConfig)
	if err != nil {
		return modelConfig{}, err
	}
	if len(record.Value) == 0 {
		return modelConfig{}, errors.New("model config is empty")
	}
	var cfg modelConfig
	if err := json.Unmarshal(record.Value, &cfg); err != nil {
		return modelConfig{}, err
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		return modelConfig{}, errors.New("model provider is required")
	}
	return cfg, nil
}

func (p *Provider) idGenerator() domain.IDGenerator {
	if p.IDGenerator != nil {
		return p.IDGenerator
	}
	return simpleIDGen{}
}

type simpleIDGen struct{}

func (simpleIDGen) NewID(prefix string) string {
	return prefix + "_" + fmt.Sprintf("%d", time.Now().UnixNano())
}

func (p *Provider) userID() string {
	if strings.TrimSpace(p.DefaultUserID) != "" {
		return strings.TrimSpace(p.DefaultUserID)
	}
	return "admin"
}

func normalizeProvider(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func (p *Provider) logInfo(ctx context.Context, msg string, args ...any) {
	if p.Logger != nil {
		p.Logger.InfoContext(ctx, msg, args...)
	}
}

func (p *Provider) logError(ctx context.Context, msg string, args ...any) {
	if p.Logger != nil {
		p.Logger.ErrorContext(ctx, msg, args...)
	}
}

func toolName(items []agentapi.Item) string {
	for _, item := range items {
		if strings.TrimSpace(item.Type) == "function_call" {
			return strings.TrimSpace(item.Name)
		}
	}
	return ""
}
