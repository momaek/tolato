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
	Model           string           `json:"model"`
	Input           []inputItem      `json:"input"`
	Temperature     float64          `json:"temperature,omitempty"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	Tools           []toolDefinition `json:"tools,omitempty"`
	ToolChoice      string           `json:"tool_choice,omitempty"`
	Stream          bool             `json:"stream"`
}

type inputItem struct {
	Role    string             `json:"role"`
	Content []inputContentItem `json:"content"`
}

type inputContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolDefinition struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
}

type streamAccumulator struct {
	responseID         string
	sequenceNumber     int
	assistantText      strings.Builder
	reasoningText      strings.Builder
	toolName           string
	toolArguments      strings.Builder
	completedResponse  json.RawMessage
	publishedStreaming bool
}

func (p Provider) RunTurn(ctx context.Context, input runtime.ModelTurnInput, tools []runtime.ToolDefinition) (runtime.ModelTurnOutput, error) {
	if strings.TrimSpace(p.Model) == "" || strings.TrimSpace(p.APIKey) == "" {
		return runtime.ModelTurnOutput{}, domain.ErrUnsupportedConfig
	}

	payload := requestBody{
		Model:           strings.TrimSpace(p.Model),
		Input:           p.input(input, tools),
		Temperature:     p.Temperature,
		MaxOutputTokens: p.MaxTokens,
		Tools:           openAITools(tools),
		Stream:          true,
	}
	if len(payload.Tools) > 0 {
		payload.ToolChoice = "auto"
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
	if out.AssistantText == nil && out.ToolCall == nil && !out.Done {
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

func (p Provider) input(input runtime.ModelTurnInput, tools []runtime.ToolDefinition) []inputItem {
	return []inputItem{
		{
			Role: "system",
			Content: []inputContentItem{{
				Type: "input_text",
				Text: systemPrompt(tools),
			}},
		},
		{
			Role: "user",
			Content: []inputContentItem{{
				Type: "input_text",
				Text: userPrompt(input),
			}},
		},
	}
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
	case "response.function_call_arguments.done":
		if arguments := readJSONStringField(raw, "arguments"); arguments != "" {
			resetBuilder(&acc.toolArguments, arguments)
		}
		if name := readToolName(raw); name != "" {
			acc.toolName = name
		}
	case "response.output_item.added", "response.output_item.done":
		if name := readToolName(raw); name != "" {
			acc.toolName = name
		}
		if arguments := readNestedJSONStringField(raw, "item", "arguments"); arguments != "" && acc.toolArguments.Len() == 0 {
			resetBuilder(&acc.toolArguments, arguments)
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
		if toolName, toolArgs, assistantText := parseCompletedResponse(acc.completedResponse); toolName != "" {
			acc.toolName = firstNonEmpty(acc.toolName, toolName)
			if toolArgs != "" && acc.toolArguments.Len() == 0 {
				resetBuilder(&acc.toolArguments, toolArgs)
			}
		} else if assistantText != "" && acc.assistantText.Len() == 0 && acc.reasoningText.Len() == 0 {
			acc.assistantText.WriteString(assistantText)
		}
	}

	out := runtime.ModelTurnOutput{
		Done:     true,
		Streamed: acc.publishedStreaming,
	}
	if strings.TrimSpace(acc.toolName) != "" {
		args := strings.TrimSpace(acc.toolArguments.String())
		if args == "" {
			args = `{}`
		}
		out.ToolCall = &runtime.ToolInvocation{
			Name: strings.TrimSpace(acc.toolName),
			Args: []byte(args),
		}
		out.Done = false
		return out
	}

	text := strings.TrimSpace(acc.assistantText.String())
	if text == "" {
		text = strings.TrimSpace(acc.reasoningText.String())
	}
	if text != "" {
		out.AssistantText = strPtr(text)
		return out
	}
	if acc.completedResponse == nil {
		out.Done = false
	}
	return out
}

func systemPrompt(tools []runtime.ToolDefinition) string {
	var builder strings.Builder
	builder.WriteString("You are the ToLaTo control-plane runtime.\n")
	builder.WriteString("Use native OpenAI function tools when execution, lookup, planning, approval, or target-resolution is needed.\n")
	builder.WriteString("Only call one tool at a time.\n")
	builder.WriteString("Prefer a tool call over assistant_text whenever the latest user intent can be executed with an available tool.\n")
	builder.WriteString("Never use assistant_text to say that you will run a command or inspect a node. Either emit the tool call now, or explain why execution cannot proceed.\n")
	builder.WriteString("If the user asks to inspect, query, execute, modify, or retrieve anything from nodes and the prompt already includes a confirmed or active target context, you must emit the matching tool call instead of a natural-language plan.\n")
	builder.WriteString("If the latest user message is a short confirmation such as 'start', 'continue', 'go ahead', '开始执行吧', '继续', or '确认', and the conversation already established a concrete node operation, emit the pending tool call now.\n")
	builder.WriteString("Use assistant_text only for final explanations, clarification requests when required context is missing, or summaries after tool results.\n")
	builder.WriteString("Available tools:\n")
	for _, tool := range tools {
		builder.WriteString("- ")
		builder.WriteString(tool.Name)
		if strings.TrimSpace(tool.Description) != "" {
			builder.WriteString(": ")
			builder.WriteString(tool.Description)
		}
		builder.WriteString("\n")
		if guidance := toolArgsGuidance(tool.Name); guidance != "" {
			builder.WriteString("  ")
			builder.WriteString(guidance)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func userPrompt(input runtime.ModelTurnInput) string {
	payload := map[string]any{
		"sessionId":           input.SessionID,
		"conversation":        input.Conversation,
		"activeTargetContext": input.ActiveTargetContext,
		"pendingAction":       input.PendingAction,
		"currentTask":         input.CurrentTask,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return `{"error":"failed to encode prompt"}`
	}
	return string(raw)
}

func openAITools(tools []runtime.ToolDefinition) []toolDefinition {
	out := make([]toolDefinition, 0, len(tools))
	for _, tool := range tools {
		out = append(out, toolDefinition{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  toolParameters(tool.Name),
			Strict:      false,
		})
	}
	return out
}

func toolParameters(name string) map[string]any {
	switch strings.TrimSpace(name) {
	case "list_nodes":
		return objectSchema(map[string]any{
			"query":  stringSchema(),
			"status": stringSchema(),
			"busy":   map[string]any{"type": "boolean"},
			"region": stringSchema(),
			"tag":    stringSchema(),
			"limit":  map[string]any{"type": "integer"},
		})
	case "resolve_target_nodes":
		return objectSchema(map[string]any{
			"query":                stringSchema(),
			"currentTargetContext": targetContextSchema(),
		})
	case "request_target_confirmation":
		return objectSchema(map[string]any{
			"targetContext": targetContextSchema(),
			"message":       stringSchema(),
		}, "targetContext")
	case "propose_plan":
		return objectSchema(map[string]any{
			"inputText":        stringSchema(),
			"targetContext":    targetContextSchema(),
			"riskLevel":        enumSchema("low", "medium", "high"),
			"requiresApproval": map[string]any{"type": "boolean"},
			"steps": map[string]any{
				"type": "array",
				"items": objectSchema(map[string]any{
					"action":           stringSchema(),
					"args":             map[string]any{"type": "object"},
					"risk":             enumSchema("low", "medium", "high"),
					"timeoutSec":       map[string]any{"type": "integer"},
					"broadcastAllowed": map[string]any{"type": "boolean"},
				}),
			},
		}, "inputText", "targetContext")
	case "request_approval":
		return objectSchema(map[string]any{
			"taskId":           stringSchema(),
			"riskLevel":        enumSchema("low", "medium", "high"),
			"message":          stringSchema(),
			"reason":           stringSchema(),
			"requiresApproval": map[string]any{"type": "boolean"},
		}, "taskId")
	case "exec_on_nodes":
		return objectSchema(map[string]any{
			"sessionId":     stringSchema(),
			"inputText":     stringSchema(),
			"command":       stringSchema(),
			"commandArgs":   stringArraySchema(),
			"targetContext": targetContextSchema(),
			"riskLevel":     enumSchema("low", "medium", "high"),
		}, "inputText")
	case "summarize_execution":
		return objectSchema(map[string]any{
			"taskId":      stringSchema(),
			"status":      stringSchema(),
			"aggregate":   executionAggregateSchema(),
			"targetLabel": stringSchema(),
		}, "taskId", "status", "aggregate")
	default:
		return objectSchema(map[string]any{})
	}
}

func targetContextSchema() map[string]any {
	return objectSchema(map[string]any{
		"scope":        stringSchema(),
		"nodeIds":      stringArraySchema(),
		"displayLabel": stringSchema(),
		"reason":       stringSchema(),
		"status":       stringSchema(),
		"source":       stringSchema(),
		"confidence":   map[string]any{"type": "number"},
	})
}

func executionAggregateSchema() map[string]any {
	return objectSchema(map[string]any{
		"total":       map[string]any{"type": "integer"},
		"queued":      map[string]any{"type": "integer"},
		"dispatched":  map[string]any{"type": "integer"},
		"running":     map[string]any{"type": "integer"},
		"succeeded":   map[string]any{"type": "integer"},
		"failed":      map[string]any{"type": "integer"},
		"canceled":    map[string]any{"type": "integer"},
		"skipped":     map[string]any{"type": "integer"},
		"pendingAck":  map[string]any{"type": "integer"},
		"unknown":     map[string]any{"type": "integer"},
		"lastUpdated": stringSchema(),
	})
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringSchema() map[string]any {
	return map[string]any{"type": "string"}
}

func stringArraySchema() map[string]any {
	return map[string]any{
		"type":  "array",
		"items": stringSchema(),
	}
}

func enumSchema(values ...string) map[string]any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return map[string]any{
		"type": "string",
		"enum": items,
	}
}

func toolArgsGuidance(name string) string {
	switch strings.TrimSpace(name) {
	case "exec_on_nodes":
		return "Function arguments must use canonical keys only: sessionId, inputText, command, commandArgs, targetContext, riskLevel. Never emit node_ids or task_text. Reuse activeTargetContext from the prompt as targetContext when dispatching the confirmed selection. For raw shell snippets, set command to \"bash\" and commandArgs to [\"-lc\", \"<snippet>\"]."
	default:
		return ""
	}
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
		{"item", "call_id"},
	} {
		if value := readJSONPathString(raw, path...); value != "" {
			if strings.Contains(value, "call_") && path[len(path)-1] == "call_id" {
				continue
			}
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

func parseCompletedResponse(raw json.RawMessage) (toolName string, toolArgs string, assistantText string) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", "", ""
	}
	responseRaw := envelope["response"]
	if len(responseRaw) == 0 {
		responseRaw = raw
	}

	var response struct {
		Output     []json.RawMessage `json:"output"`
		OutputText string            `json:"output_text"`
	}
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		return "", "", ""
	}
	if strings.TrimSpace(response.OutputText) != "" {
		assistantText = strings.TrimSpace(response.OutputText)
	}

	for _, itemRaw := range response.Output {
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(itemRaw, &header); err != nil {
			continue
		}
		switch strings.TrimSpace(header.Type) {
		case "function_call":
			var item struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if err := json.Unmarshal(itemRaw, &item); err == nil {
				toolName = firstNonEmpty(toolName, strings.TrimSpace(item.Name))
				toolArgs = firstNonEmpty(toolArgs, strings.TrimSpace(item.Arguments))
			}
		case "message":
			var item struct {
				Content []json.RawMessage `json:"content"`
			}
			if err := json.Unmarshal(itemRaw, &item); err != nil {
				continue
			}
			if assistantText == "" {
				assistantText = decodeContentParts(item.Content)
			}
		}
	}
	return toolName, toolArgs, assistantText
}

func decodeContentParts(parts []json.RawMessage) string {
	var builder strings.Builder
	for _, partRaw := range parts {
		var part struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(partRaw, &part); err != nil {
			continue
		}
		if !isTextLikePart(part.Type) {
			continue
		}
		chunk := strings.TrimSpace(part.Text)
		if chunk == "" {
			chunk = strings.TrimSpace(part.Content)
		}
		if chunk == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(chunk)
	}
	return builder.String()
}

func isTextLikePart(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "", "text", "output_text", "reasoning", "reasoning_text", "reasoning_content":
		return true
	default:
		return false
	}
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

func cloneRaw(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func strPtr(v string) *string {
	return &v
}
