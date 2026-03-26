package agentsdk

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Create the agent.
	opts := []agent.Option{
		agent.WithLLM(llmClient),
		agent.WithTools(interceptedTools...),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithMaxIterations(20), // high limit — the Runtime controls the actual loop via channels
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

	// Determine the input prompt.
	prompt := conversationToPrompt(input.Conversation)

	// Launch the agent goroutine.
	responseID := p.ids.NewID("resp")
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
		// Fallback to non-streaming.
		result, err := agentInstance.Run(ctx, prompt)
		runner.doneChan <- RunResult{Content: result, Error: err, Streamed: false}
		return
	}

	eventCh, err := streamAgent.RunStream(ctx, prompt)
	if err != nil {
		runner.doneChan <- RunResult{Error: err}
		return
	}

	var finalContent string
	for event := range eventCh {
		// Forward to the streaming forwarder goroutine.
		select {
		case runner.streamChan <- event:
		case <-ctx.Done():
			runner.doneChan <- RunResult{Error: ctx.Err()}
			return
		}
		if event.Type == interfaces.AgentEventComplete {
			finalContent = event.Content
		}
		if event.Type == interfaces.AgentEventContent {
			finalContent = event.Content
		}
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

	responseID := p.ids.NewID("resp")
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
	// Start streaming forwarder.
	streamCtx, streamCancel := context.WithCancel(ctx)
	go func() {
		forwardStreamEvents(streamCtx, sessionID, responseID, runner.streamChan, p.events)
		streamCancel()
	}()
	_ = streamCancel // used in goroutine above

	select {
	case call := <-runner.toolCallChan:
		// agent-sdk-go is blocked in Execute() waiting for a result.
		callID := "call_" + strings.ReplaceAll(call.Name, " ", "_")
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

// ---------------------------------------------------------------------------
// System prompt builder
// ---------------------------------------------------------------------------

func buildSystemPrompt(input runtime.ModelTurnInput) string {
	var b strings.Builder
	b.WriteString("You are the ToLaTo control-plane runtime.\n")
	b.WriteString("Use the provided function tools directly when lookup, planning, approval, target resolution, or execution is needed.\n")
	b.WriteString("Call at most one function per turn.\n")
	b.WriteString("If a function can execute the user's request, call it instead of narrating what you would do.\n")
	b.WriteString("Ask for clarification only when required data is genuinely missing.\n")

	payload := map[string]any{
		"activeTargetContext": input.ActiveTargetContext,
		"pendingAction":      input.PendingAction,
		"currentTask":        input.CurrentTask,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return b.String()
	}
	b.WriteString("Runtime context JSON:\n")
	b.Write(raw)
	return b.String()
}
