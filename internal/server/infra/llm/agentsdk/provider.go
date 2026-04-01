package agentsdk

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	openaillm "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/runtime"
	"github.com/momaek/tolato/internal/server/domain"
)

// Provider implements runtime.LLMClient using agent-sdk-go.
// It manages per-session goroutines (activeRunner) that run agent-sdk-go
// and communicate with the Runtime via channels.
type Provider struct {
	config  ProviderConfig
	events  runtime.EventPublisher
	ids     domain.IDGenerator
	runners sync.Map // sessionID → *activeRunner
}

var _ runtime.LLMClient = (*Provider)(nil)

// NewProvider creates a new agent-sdk-go based provider.
func NewProvider(config ProviderConfig, events runtime.EventPublisher, ids domain.IDGenerator) *Provider {
	return &Provider{
		config: config,
		events: events,
		ids:    ids,
	}
}

// RunTurn implements runtime.LLMClient. It either resumes an existing
// runner (if the session has one blocked on a tool call) or starts a new one.
func (p *Provider) RunTurn(
	ctx context.Context,
	input runtime.ModelTurnInput,
	tools []agentapi.ToolSpec,
) (runtime.ModelTurnOutput, error) {
	// Resume path: there is an active runner blocked in Execute().
	if raw, ok := p.runners.Load(input.SessionID); ok {
		runner := raw.(*activeRunner)
		return p.resumeRunner(ctx, input, runner)
	}

	// New run path.
	return p.startNewRunner(ctx, input, tools)
}

// CleanupRunner cancels and removes the runner for the given session.
func (p *Provider) CleanupRunner(sessionID string) {
	if raw, ok := p.runners.LoadAndDelete(sessionID); ok {
		raw.(*activeRunner).stop()
	}
}

// ---------------------------------------------------------------------------
// Start a new agent-sdk-go run
// ---------------------------------------------------------------------------

func (p *Provider) startNewRunner(
	ctx context.Context,
	input runtime.ModelTurnInput,
	tools []agentapi.ToolSpec,
) (runtime.ModelTurnOutput, error) {
	runner := newRunner()

	// Build the timeout-guarded context for the agent goroutine.
	timeout := time.Duration(p.config.runnerTimeout()) * time.Second
	runCtx, cancel := context.WithTimeout(context.Background(), timeout)
	runner.cancel = cancel
	runner.runCtx = runCtx

	// Convert ToLaTo tools into intercepted tools.
	interceptedTools := wrapToolSpecs(tools, runner.toolCallChan, runner.resultChan)

	// Create the agent-sdk-go LLM client.
	llmClient, err := p.createLLMClient()
	if err != nil {
		cancel()
		return runtime.ModelTurnOutput{}, fmt.Errorf("agentsdk: create llm client: %w", err)
	}

	// Build system prompt with runtime context.
	systemPrompt := buildSystemPrompt(input)

	// Build conversation memory from history so the agent has full context.
	mem, prompt := buildConversationMemory(input.Conversation)

	// Create the agent.
	opts := []agent.Option{
		agent.WithLLM(llmClient),
		agent.WithTools(interceptedTools...),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithMemory(mem),
		agent.WithMaxIterations(20),         // high limit — the Runtime controls the actual loop via channels
		agent.WithRequirePlanApproval(false), // disable execution plans — we use a direct agentic loop
	}
	if p.config.EnableThinking {
		opts = append(opts, agent.WithStreamConfig(&interfaces.StreamConfig{
			BufferSize:          128,
			IncludeThinking:     true,
			IncludeToolProgress: true,
		}))
	} else {
		opts = append(opts, agent.WithStreamConfig(&interfaces.StreamConfig{
			BufferSize:          128,
			IncludeThinking:     false,
			IncludeToolProgress: true,
		}))
	}

	agentInstance, err := agent.NewAgent(opts...)
	if err != nil {
		cancel()
		return runtime.ModelTurnOutput{}, fmt.Errorf("agentsdk: create agent: %w", err)
	}

	// Store the runner so resume can find it.
	p.runners.Store(input.SessionID, runner)

	// Set response ID BEFORE starting the forwarder to avoid race condition.
	responseID := p.ids.NewID("resp")
	runner.setResponseID(responseID)

	// Launch the agent goroutine.
	go p.runAgent(runCtx, input.SessionID, responseID, agentInstance, prompt, runner)

	// Wait for the first event.
	return p.waitForEvent(ctx, input.SessionID, runner, responseID)
}

// runAgent drives agent-sdk-go in a background goroutine.
func (p *Provider) runAgent(
	ctx context.Context,
	sessionID string,
	responseID string,
	agentInstance *agent.Agent,
	prompt string,
	runner *activeRunner,
) {
	defer close(runner.streamChan)

	// Use RunStream to get thinking/content events for the frontend.
	streamAgent, ok := interface{}(agentInstance).(interfaces.StreamingAgent)
	if !ok {
		slog.Warn("agentsdk: agent does not implement StreamingAgent, falling back to non-streaming",
			"session_id", sessionID)
		result, err := agentInstance.Run(ctx, prompt)
		runner.doneChan <- RunResult{Content: result, Error: err, Streamed: false}
		return
	}

	slog.Info("agentsdk: starting RunStream",
		"session_id", sessionID, "response_id", responseID,
		"prompt_len", len(prompt))
	eventCh, err := streamAgent.RunStream(ctx, prompt)
	if err != nil {
		slog.Error("agentsdk: RunStream failed", "session_id", sessionID, "error", err)
		runner.doneChan <- RunResult{Error: err}
		return
	}

	var contentBuilder strings.Builder
	var completeContent string
	for event := range eventCh {
		// Forward to the streaming forwarder goroutine.
		select {
		case runner.streamChan <- event:
		case <-ctx.Done():
			runner.doneChan <- RunResult{Error: ctx.Err()}
			return
		}
		switch event.Type {
		case interfaces.AgentEventComplete:
			// AgentEventComplete carries the full final content.
			completeContent = event.Content
		case interfaces.AgentEventContent:
			// AgentEventContent deltas need to be accumulated.
			contentBuilder.WriteString(event.Content)
		}
	}

	// Prefer the complete event's content; fall back to accumulated deltas.
	finalContent := completeContent
	if finalContent == "" {
		finalContent = contentBuilder.String()
	}
	runner.doneChan <- RunResult{Content: finalContent, Streamed: true}
}

// ---------------------------------------------------------------------------
// Resume an existing runner
// ---------------------------------------------------------------------------

func (p *Provider) resumeRunner(
	ctx context.Context,
	input runtime.ModelTurnInput,
	runner *activeRunner,
) (runtime.ModelTurnOutput, error) {
	// Find the last function_call_output in the conversation.
	output := extractLastFunctionOutput(input.Conversation)

	// Send it to the blocked Execute().
	select {
	case runner.resultChan <- ToolCallResult{Output: output}:
	case <-ctx.Done():
		return runtime.ModelTurnOutput{}, ctx.Err()
	}

	// Set response ID BEFORE waiting so the forwarder uses the new ID.
	responseID := p.ids.NewID("resp")
	runner.setResponseID(responseID)

	return p.waitForEvent(ctx, input.SessionID, runner, responseID)
}

// ---------------------------------------------------------------------------
// Wait for the next event from the runner
// ---------------------------------------------------------------------------

func (p *Provider) waitForEvent(
	ctx context.Context,
	sessionID string,
	runner *activeRunner,
	responseID string,
) (runtime.ModelTurnOutput, error) {
	// Ensure the single streaming forwarder goroutine is started exactly once
	// for this runner's lifetime. The responseID was already set before calling
	// this method, so the forwarder will tag events with the correct turn ID.
	runner.startForwarder(sessionID, p.events)

	select {
	case call := <-runner.toolCallChan:
		// agent-sdk-go is blocked in Execute() waiting for a result.
		callID := p.ids.NewID("call")
		return runtime.ModelTurnOutput{
			ResponseID: responseID,
			Items:      []agentapi.Item{toolCallToItem(call, callID)},
			Done:       false,
			Streamed:   true,
		}, nil

	case result := <-runner.doneChan:
		// Agent finished. Clean up.
		p.runners.Delete(sessionID)
		runner.stop()
		if result.Error != nil {
			return runtime.ModelTurnOutput{}, result.Error
		}
		var items []agentapi.Item
		if strings.TrimSpace(result.Content) != "" {
			items = []agentapi.Item{messageItem(result.Content)}
		}
		return runtime.ModelTurnOutput{
			ResponseID: responseID,
			Items:      items,
			Done:       true,
			Streamed:   result.Streamed,
		}, nil

	case <-ctx.Done():
		p.runners.Delete(sessionID)
		runner.stop()
		return runtime.ModelTurnOutput{}, ctx.Err()
	}
}

// ---------------------------------------------------------------------------
// LLM client factory
// ---------------------------------------------------------------------------

func (p *Provider) createLLMClient() (interfaces.LLM, error) {
	switch strings.ToLower(strings.TrimSpace(p.config.ProviderType)) {
	case "openai", "":
		opts := []openaillm.Option{
			openaillm.WithModel(p.config.Model),
		}
		if strings.TrimSpace(p.config.Endpoint) != "" {
			opts = append(opts, openaillm.WithBaseURL(strings.TrimSpace(p.config.Endpoint)))
		}
		client := openaillm.NewClient(p.config.APIKey, opts...)
		return client, nil
	// TODO: add anthropic, gemini when needed
	default:
		return nil, fmt.Errorf("agentsdk: unsupported provider type %q", p.config.ProviderType)
	}
}

// buildSystemPrompt delegates to the shared runtime.BuildSystemPrompt.
func buildSystemPrompt(input runtime.ModelTurnInput) string {
	return runtime.BuildSystemPrompt(input)
}
