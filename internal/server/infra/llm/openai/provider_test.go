package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momaek/tolato/internal/server/app/runtime"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestSystemPromptIncludesExecOnNodesArgGuidance(t *testing.T) {
	prompt := systemPrompt([]runtime.ToolDefinition{{
		Name:        "exec_on_nodes",
		Description: "dispatch command execution",
	}})

	for _, want := range []string{
		"sessionId, inputText, command, commandArgs, targetContext, riskLevel",
		"Never emit node_ids or task_text.",
		"set command to \"bash\" and commandArgs to [\"-lc\", \"<snippet>\"]",
		"Use native OpenAI function tools",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("systemPrompt() missing %q in %q", want, prompt)
		}
	}
}

func TestToolArgsGuidanceOnlyTargetsExecOnNodes(t *testing.T) {
	if got := toolArgsGuidance("list_nodes"); got != "" {
		t.Fatalf("toolArgsGuidance(list_nodes) = %q, want empty", got)
	}
	if got := toolArgsGuidance("exec_on_nodes"); !strings.Contains(got, "Function arguments must use canonical keys only") {
		t.Fatalf("toolArgsGuidance(exec_on_nodes) = %q, want canonical guidance", got)
	}
}

func TestRunTurnStreamsToolCallAndPublishesEvents(t *testing.T) {
	var captured requestBody
	events := &stubEvents{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			"event: response.output_text.delta\n",
			"data: {\"type\":\"response.output_text.delta\",\"response_id\":\"resp_1\",\"delta\":\"准备执行\"}\n\n",
			"event: response.output_item.added\n",
			"data: {\"type\":\"response.output_item.added\",\"response_id\":\"resp_1\",\"item\":{\"type\":\"function_call\",\"name\":\"exec_on_nodes\"}}\n\n",
			"event: response.function_call_arguments.delta\n",
			"data: {\"type\":\"response.function_call_arguments.delta\",\"response_id\":\"resp_1\",\"delta\":\"{\\\"inputText\\\":\\\"ls -la ~\\\"\"}\n\n",
			"event: response.function_call_arguments.delta\n",
			"data: {\"type\":\"response.function_call_arguments.delta\",\"response_id\":\"resp_1\",\"delta\":\",\\\"command\\\":\\\"bash\\\",\\\"commandArgs\\\":[\\\"-lc\\\",\\\"ls -la ~\\\"]}\"}\n\n",
			"event: response.completed\n",
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"type\":\"function_call\",\"name\":\"exec_on_nodes\",\"arguments\":\"{\\\"inputText\\\":\\\"ls -la ~\\\",\\\"command\\\":\\\"bash\\\",\\\"commandArgs\\\":[\\\"-lc\\\",\\\"ls -la ~\\\"]}\"}]}}\n\n",
			"data: [DONE]\n\n",
		}, "")))
	}))
	defer server.Close()

	provider := Provider{
		Model:    "gpt-test",
		Endpoint: server.URL,
		APIKey:   "test-key",
		Events:   events,
	}
	output, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{SessionID: "sess-1"}, []runtime.ToolDefinition{{
		Name:        "exec_on_nodes",
		Description: "dispatch command execution",
	}})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if output.ToolCall == nil || output.ToolCall.Name != "exec_on_nodes" {
		t.Fatalf("ToolCall = %#v, want exec_on_nodes", output.ToolCall)
	}
	if got := string(output.ToolCall.Args); got != `{"inputText":"ls -la ~","command":"bash","commandArgs":["-lc","ls -la ~"]}` {
		t.Fatalf("ToolCall args = %s", got)
	}
	if output.AssistantText != nil {
		t.Fatalf("AssistantText = %#v, want nil", output.AssistantText)
	}
	if output.Done {
		t.Fatalf("Done = true, want false")
	}
	if !output.Streamed {
		t.Fatalf("Streamed = false, want true")
	}
	if captured.ToolChoice != "auto" || !captured.Stream {
		t.Fatalf("captured request = %#v, want tool_choice=auto and stream=true", captured)
	}
	if len(captured.Tools) != 1 || captured.Tools[0].Name != "exec_on_nodes" {
		t.Fatalf("captured tools = %#v", captured.Tools)
	}
	if len(events.sse) != 4 {
		t.Fatalf("llm sse events = %#v, want 4 streamed events before completion", events.sse)
	}
	if len(events.completed) != 1 {
		t.Fatalf("completed events = %#v, want 1", events.completed)
	}
}

func TestRunTurnStreamsAssistantText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			"event: response.reasoning_text.delta\n",
			"data: {\"type\":\"response.reasoning_text.delta\",\"response_id\":\"resp_2\",\"delta\":\"先检查上下文。\"}\n\n",
			"event: response.output_text.delta\n",
			"data: {\"type\":\"response.output_text.delta\",\"response_id\":\"resp_2\",\"delta\":\"列出了 home 目录内容。\"}\n\n",
			"event: response.completed\n",
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"output_text\":\"列出了 home 目录内容。\"}}\n\n",
			"data: [DONE]\n\n",
		}, "")))
	}))
	defer server.Close()

	provider := Provider{
		Model:    "gpt-test",
		Endpoint: server.URL,
		APIKey:   "test-key",
	}
	output, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{SessionID: "sess-1"}, nil)
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if output.AssistantText == nil || *output.AssistantText != "列出了 home 目录内容。" {
		t.Fatalf("AssistantText = %#v", output.AssistantText)
	}
	if output.ToolCall != nil {
		t.Fatalf("ToolCall = %#v, want nil", output.ToolCall)
	}
	if !output.Done {
		t.Fatalf("Done = false, want true")
	}
}

func TestFinalizeTurnOutputFallsBackToCompletedReasoning(t *testing.T) {
	acc := &streamAccumulator{
		completedResponse: json.RawMessage(`{"type":"response.completed","response":{"id":"resp_3","output":[{"type":"message","content":[{"type":"reasoning_text","text":"推断当前需要先确认目标节点。"}]}]}}`),
	}
	output := finalizeTurnOutput(acc)
	if output.AssistantText == nil || *output.AssistantText != "推断当前需要先确认目标节点。" {
		t.Fatalf("AssistantText = %#v", output.AssistantText)
	}
}

type stubEvents struct {
	sse       []stubSSE
	completed []json.RawMessage
}

type stubSSE struct {
	sessionID         string
	responseID        string
	sequenceNumber    int
	upstreamEventType string
	rawEvent          json.RawMessage
}

func (s *stubEvents) SessionStateUpdated(context.Context, domain.Session) error { return nil }
func (s *stubEvents) TimelineRowAppended(context.Context, domain.Session, domain.TimelineRow) error {
	return nil
}
func (s *stubEvents) ThreadTargetPending(context.Context, domain.Session) error   { return nil }
func (s *stubEvents) ThreadTargetConfirmed(context.Context, domain.Session) error { return nil }
func (s *stubEvents) ThreadTargetCleared(context.Context, domain.Session) error   { return nil }
func (s *stubEvents) ExecutionChunk(context.Context, string, string, domain.Execution, domain.ExecutionChunk) error {
	return nil
}
func (s *stubEvents) ExecutionFinished(context.Context, string, string, domain.Execution) error {
	return nil
}
func (s *stubEvents) LLMSSEEvent(_ context.Context, sessionID string, responseID string, sequenceNumber int, upstreamEventType string, rawEvent json.RawMessage) error {
	s.sse = append(s.sse, stubSSE{
		sessionID:         sessionID,
		responseID:        responseID,
		sequenceNumber:    sequenceNumber,
		upstreamEventType: upstreamEventType,
		rawEvent:          append(json.RawMessage(nil), rawEvent...),
	})
	return nil
}
func (s *stubEvents) LLMResponseCompleted(_ context.Context, _ string, _ string, rawResponse json.RawMessage) error {
	s.completed = append(s.completed, append(json.RawMessage(nil), rawResponse...))
	return nil
}
