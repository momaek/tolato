package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/runtime"
	"github.com/momaek/tolato/internal/server/domain"
)

func TestInstructionsIncludeSystemPrompt(t *testing.T) {
	text := instructions(runtime.ModelTurnInput{
		SessionID: "sess-1",
	})
	if !strings.Contains(text, "VPS fleet management assistant") {
		t.Fatalf("instructions() = %q, want system prompt content", text)
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
			"data: {\"type\":\"response.output_item.added\",\"response_id\":\"resp_1\",\"item\":{\"type\":\"function_call\",\"name\":\"run_on_node\"}}\n\n",
			"event: response.function_call_arguments.delta\n",
			"data: {\"type\":\"response.function_call_arguments.delta\",\"response_id\":\"resp_1\",\"delta\":\"{\\\"target\\\":\\\"tokyo\\\"\"}\n\n",
			"event: response.function_call_arguments.delta\n",
			"data: {\"type\":\"response.function_call_arguments.delta\",\"response_id\":\"resp_1\",\"delta\":\",\\\"command\\\":\\\"system_status\\\"}\"}\n\n",
			"event: response.completed\n",
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"type\":\"function_call\",\"name\":\"run_on_node\",\"arguments\":\"{\\\"target\\\":\\\"tokyo\\\",\\\"command\\\":\\\"system_status\\\"}\"}]}}\n\n",
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
	output, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{SessionID: "sess-1"}, []agentapi.ToolSpec{
		agentapi.NewFunctionTool("run_on_node", "run command on node", map[string]any{"type": "object"}),
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(output.Items) != 1 || output.Items[0].Type != "function_call" || output.Items[0].Name != "run_on_node" {
		t.Fatalf("Items = %#v, want run_on_node function_call", output.Items)
	}
	if got := output.Items[0].Arguments; got != `{"target":"tokyo","command":"system_status"}` {
		t.Fatalf("function_call arguments = %s", got)
	}
	if output.Done {
		t.Fatalf("Done = true, want false")
	}
	if !output.Streamed {
		t.Fatalf("Streamed = false, want true")
	}
	if captured.ToolChoice != "auto" || !captured.Stream || captured.ParallelToolCalls {
		t.Fatalf("captured request = %#v, want tool_choice=auto stream=true parallel_tool_calls=false", captured)
	}
	if len(captured.Tools) != 1 || captured.Tools[0].Function.Name != "run_on_node" {
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
	if len(output.Items) != 1 || agentapi.MessageText(output.Items[0]) != "列出了 home 目录内容。" {
		t.Fatalf("Items = %#v", output.Items)
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
	if len(output.Items) != 1 || agentapi.MessageText(output.Items[0]) != "推断当前需要先确认目标节点。" {
		t.Fatalf("Items = %#v", output.Items)
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
