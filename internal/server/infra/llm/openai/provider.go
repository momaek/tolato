package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/runtime"
	"github.com/momaek/tolato/internal/server/domain"
)

type Provider struct {
	Model       string
	Endpoint    string
	APIKey      string
	Temperature float64
	MaxTokens   int
	TimeoutSec  int
	HTTPClient  *http.Client
	Events      runtime.EventPublisher
}

type requestBody struct {
	Model             string              `json:"model"`
	Input             []agentapi.Item     `json:"input"`
	Instructions      string              `json:"instructions,omitempty"`
	Temperature       float64             `json:"temperature,omitempty"`
	MaxOutputTokens   int                 `json:"max_output_tokens,omitempty"`
	Tools             []agentapi.ToolSpec `json:"tools,omitempty"`
	ToolChoice        string              `json:"tool_choice,omitempty"`
	ParallelToolCalls bool                `json:"parallel_tool_calls"`
	Stream            bool                `json:"stream"`
}

type streamAccumulator struct {
	responseID         string
	sequenceNumber     int
	assistantText      strings.Builder
	reasoningText      strings.Builder
	toolName           string
	toolArguments      strings.Builder
	toolCallID         string
	completedResponse  json.RawMessage
	publishedStreaming bool
}

func (p Provider) RunTurn(ctx context.Context, input runtime.ModelTurnInput, tools []agentapi.ToolSpec) (runtime.ModelTurnOutput, error) {
	if strings.TrimSpace(p.Model) == "" || strings.TrimSpace(p.APIKey) == "" {
		return runtime.ModelTurnOutput{}, domain.ErrUnsupportedConfig
	}

	payload := requestBody{
		Model:             strings.TrimSpace(p.Model),
		Input:             agentapi.CloneItems(input.Conversation),
		Instructions:      instructions(input),
		Temperature:       p.Temperature,
		MaxOutputTokens:   p.MaxTokens,
		Tools:             cloneTools(tools),
		ToolChoice:        "auto",
		ParallelToolCalls: false,
		Stream:            true,
	}
	if len(payload.Tools) == 0 {
		payload.ToolChoice = "none"
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return runtime.ModelTurnOutput{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint()+"/responses", bytes.NewReader(raw))
	if err != nil {
		return runtime.ModelTurnOutput{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.APIKey))

	resp, err := p.client().Do(req)
	if err != nil {
		return runtime.ModelTurnOutput{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		if message := decodeErrorMessage(body); message != "" {
			return runtime.ModelTurnOutput{}, errors.New(message)
		}
		return runtime.ModelTurnOutput{}, fmt.Errorf("openai request failed with status %d", resp.StatusCode)
	}

	acc := &streamAccumulator{}
	if isEventStream(resp.Header.Get("Content-Type")) {
		if err := p.consumeStream(ctx, input.SessionID, resp.Body, acc); err != nil {
			return runtime.ModelTurnOutput{}, err
		}
	} else {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return runtime.ModelTurnOutput{}, err
		}
		acc.completedResponse = cloneRaw(body)
	}

	out := finalizeTurnOutput(acc)
	if len(out.Items) == 0 && !out.Done {
		return runtime.ModelTurnOutput{}, runtime.ErrEmptyModelOutput
	}
	return out, nil
}

func (p Provider) endpoint() string {
	if strings.TrimSpace(p.Endpoint) != "" {
		return strings.TrimRight(strings.TrimSpace(p.Endpoint), "/")
	}
	return "https://api.openai.com/v1"
}

func (p Provider) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	timeout := 60 * time.Second
	if p.TimeoutSec > 0 {
		timeout = time.Duration(p.TimeoutSec) * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func (p Provider) consumeStream(ctx context.Context, sessionID string, body io.Reader, acc *streamAccumulator) error {
	reader := bufio.NewReader(body)
	var eventType string
	var dataLines []string

	flush := func() error {
		if len(dataLines) == 0 {
			eventType = ""
			return nil
		}
		data := strings.Join(dataLines, "\n")
		eventType = strings.TrimSpace(eventType)
		dataLines = nil
		if strings.TrimSpace(data) == "[DONE]" {
			eventType = ""
			return nil
		}
		if eventType == "" {
			eventType = detectEventType([]byte(data))
		}
		if strings.TrimSpace(eventType) == "" {
			eventType = ""
			return nil
		}
		raw := cloneRaw([]byte(data))
		if err := p.consumeEvent(ctx, sessionID, eventType, raw, acc); err != nil {
			return err
		}
		eventType = ""
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if flushErr := flush(); flushErr != nil {
				return flushErr
			}
		} else if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}

	return flush()
}

func (p Provider) consumeEvent(ctx context.Context, sessionID string, eventType string, raw json.RawMessage, acc *streamAccumulator) error {
	if acc.responseID == "" {
		acc.responseID = firstNonEmpty(acc.responseID, detectResponseID(raw))
	}

	switch strings.TrimSpace(eventType) {
	case "response.output_text.delta":
		acc.assistantText.WriteString(readTextDelta(raw))
	case "response.reasoning_text.delta":
		acc.reasoningText.WriteString(readTextDelta(raw))
	case "response.function_call_arguments.delta":
		acc.toolArguments.WriteString(readTextDelta(raw))
		if name := readToolName(raw); name != "" {
			acc.toolName = name
		}
		if callID := readCallID(raw); callID != "" {
			acc.toolCallID = callID
		}
	case "response.function_call_arguments.done":
		if arguments := readJSONStringField(raw, "arguments"); arguments != "" {
			resetBuilder(&acc.toolArguments, arguments)
		}
		if name := readToolName(raw); name != "" {
			acc.toolName = name
		}
		if callID := readCallID(raw); callID != "" {
			acc.toolCallID = callID
		}
	case "response.output_item.added", "response.output_item.done":
		if name := readToolName(raw); name != "" {
			acc.toolName = name
		}
		if arguments := readNestedJSONStringField(raw, "item", "arguments"); arguments != "" && acc.toolArguments.Len() == 0 {
			resetBuilder(&acc.toolArguments, arguments)
		}
		if callID := readCallID(raw); callID != "" {
			acc.toolCallID = callID
		}
	case "response.completed":
		acc.completedResponse = cloneRaw(raw)
		if acc.responseID == "" {
			acc.responseID = detectResponseID(raw)
		}
		if p.Events != nil {
			if err := p.Events.LLMResponseCompleted(ctx, sessionID, acc.responseID, raw); err != nil {
				return err
			}
			acc.publishedStreaming = true
		}
		return nil
	}

	if p.Events != nil {
		acc.sequenceNumber++
		if err := p.Events.LLMSSEEvent(ctx, sessionID, acc.responseID, acc.sequenceNumber, eventType, raw); err != nil {
			return err
		}
		acc.publishedStreaming = true
	}
	return nil
}

func finalizeTurnOutput(acc *streamAccumulator) runtime.ModelTurnOutput {
	if acc.completedResponse != nil {
		if items, responseID := parseCompletedResponse(acc.completedResponse); len(items) > 0 {
			return runtime.ModelTurnOutput{
				ResponseID:    firstNonEmpty(acc.responseID, responseID),
				Items:         items,
				Done:          !containsFunctionCall(items),
				Streamed:      acc.publishedStreaming,
				ProviderState: nil,
			}
		}
	}

	out := runtime.ModelTurnOutput{
		ResponseID: acc.responseID,
		Done:       true,
		Streamed:   acc.publishedStreaming,
	}
	if strings.TrimSpace(acc.toolName) != "" {
		args := strings.TrimSpace(acc.toolArguments.String())
		if args == "" {
			args = `{}`
		}
		out.Items = []agentapi.Item{{
			Type:      "function_call",
			Name:      strings.TrimSpace(acc.toolName),
			Arguments: args,
			CallID:    firstNonEmpty(acc.toolCallID, "call_"+strings.TrimSpace(acc.toolName)),
		}}
		out.Done = false
		return out
	}

	text := strings.TrimSpace(acc.assistantText.String())
	if text == "" {
		text = strings.TrimSpace(acc.reasoningText.String())
	}
	if text != "" {
		out.Items = []agentapi.Item{messageItem(text)}
		return out
	}
	if acc.completedResponse == nil {
		out.Done = false
	}
	return out
}

func instructions(input runtime.ModelTurnInput) string {
	var builder strings.Builder
	builder.WriteString("You are the ToLaTo control-plane runtime.\n")
	builder.WriteString("Use the provided OpenAI function tools directly when lookup, planning, approval, target resolution, or execution is needed.\n")
	builder.WriteString("Call at most one function per turn.\n")
	builder.WriteString("If a function can execute the user's request, call it instead of narrating what you would do.\n")
	builder.WriteString("Ask for clarification only when required data is genuinely missing.\n")

	payload := map[string]any{
		"activeTargetContext": input.ActiveTargetContext,
		"pendingAction":       input.PendingAction,
		"currentTask":         input.CurrentTask,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return builder.String()
	}
	builder.WriteString("Runtime context JSON:\n")
	builder.Write(raw)
	return builder.String()
}

func isEventStream(contentType string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(contentType)), "text/event-stream")
}

func detectEventType(raw []byte) string {
	var payload struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw, &payload)
	return strings.TrimSpace(payload.Type)
}

func detectResponseID(raw []byte) string {
	for _, path := range [][]string{
		{"response", "id"},
		{"id"},
		{"response_id"},
	} {
		if value := readJSONPathString(raw, path...); value != "" {
			return value
		}
	}
	return ""
}

func readTextDelta(raw []byte) string {
	for _, path := range [][]string{
		{"delta"},
		{"text"},
		{"arguments"},
		{"item", "arguments"},
	} {
		if value := readJSONPathString(raw, path...); value != "" {
			return value
		}
	}
	return ""
}

func readToolName(raw []byte) string {
	for _, path := range [][]string{
		{"name"},
		{"item", "name"},
		{"output_item", "name"},
	} {
		if value := readJSONPathString(raw, path...); value != "" {
			return value
		}
	}
	return ""
}

func readCallID(raw []byte) string {
	for _, path := range [][]string{
		{"call_id"},
		{"item", "call_id"},
		{"output_item", "call_id"},
	} {
		if value := readJSONPathString(raw, path...); value != "" {
			return value
		}
	}
	return ""
}

func readJSONStringField(raw []byte, field string) string {
	return readJSONPathString(raw, field)
}

func readNestedJSONStringField(raw []byte, path ...string) string {
	return readJSONPathString(raw, path...)
}

func readJSONPathString(raw []byte, path ...string) string {
	if len(path) == 0 {
		return ""
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return ""
	}
	value := root
	for _, key := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value, ok = object[key]
		if !ok {
			return ""
		}
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			chunk, ok := item.(string)
			if !ok || strings.TrimSpace(chunk) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(chunk)
		}
		return builder.String()
	default:
		return ""
	}
}

func parseCompletedResponse(raw json.RawMessage) ([]agentapi.Item, string) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, ""
	}
	responseRaw := envelope["response"]
	if len(responseRaw) == 0 {
		responseRaw = raw
	}

	var response struct {
		ID         string            `json:"id"`
		Output     []json.RawMessage `json:"output"`
		OutputText string            `json:"output_text"`
	}
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		return nil, ""
	}

	items := make([]agentapi.Item, 0, len(response.Output)+1)
	for _, itemRaw := range response.Output {
		var item agentapi.Item
		if err := json.Unmarshal(itemRaw, &item); err != nil {
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 && strings.TrimSpace(response.OutputText) != "" {
		items = append(items, messageItem(strings.TrimSpace(response.OutputText)))
	}
	return items, response.ID
}

func decodeErrorMessage(raw []byte) string {
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Error == nil {
		return ""
	}
	return strings.TrimSpace(payload.Error.Message)
}

func resetBuilder(builder *strings.Builder, value string) {
	builder.Reset()
	builder.WriteString(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func containsFunctionCall(items []agentapi.Item) bool {
	for _, item := range items {
		if strings.TrimSpace(item.Type) == "function_call" {
			return true
		}
	}
	return false
}

func cloneRaw(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func strPtr(v string) *string {
	return &v
}

func cloneTools(tools []agentapi.ToolSpec) []agentapi.ToolSpec {
	if len(tools) == 0 {
		return nil
	}
	out := make([]agentapi.ToolSpec, len(tools))
	copy(out, tools)
	return out
}

func messageItem(text string) agentapi.Item {
	raw, err := json.Marshal([]agentapi.ContentPart{{
		Type: "output_text",
		Text: text,
	}})
	if err != nil {
		panic(err)
	}
	return agentapi.Item{
		Type:    "message",
		Role:    "assistant",
		Content: raw,
	}
}
