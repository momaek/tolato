package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	openai "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/shared"
)

// ToolDefinition defines a tool the AI can call.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema
}

// ToolCall represents a tool call from the AI response.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// StreamDelta represents a single streaming delta from the AI.
type StreamDelta struct {
	Type      string // "reasoning", "content", "tool_calls_delta"
	Reasoning string
	Content   string
}

// ChatMessage represents a message in the conversation.
type ChatMessage struct {
	Role       string     // "system", "user", "assistant", "tool"
	Content    string
	Reasoning  string     // for assistant messages with reasoning
	ToolCalls  []ToolCall // for assistant messages
	ToolCallID string     // for tool result messages
}

// StreamCallback is called for each streaming delta.
type StreamCallback func(delta StreamDelta)

// CompletionResult is the final result after streaming completes.
type CompletionResult struct {
	Content   string
	Reasoning string
	ToolCalls []ToolCall
}

// ClientConfig holds the configuration for the LLM client.
type ClientConfig struct {
	APIBaseURL  string
	APIKey      string
	Model       string
	Temperature float64
}

// Client is the LLM client for OpenAI-compatible APIs.
type Client struct {
	mu     sync.RWMutex
	config ClientConfig
	tools  []ToolDefinition
	sdk    *openai.Client
}

// NewClient creates a new LLM client.
func NewClient(cfg ClientConfig, tools []ToolDefinition) *Client {
	c := &Client{
		config: cfg,
		tools:  tools,
	}
	c.sdk = c.buildSDKClient(cfg)
	return c
}

// UpdateConfig updates the client configuration (called when settings change).
func (c *Client) UpdateConfig(cfg ClientConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = cfg
	c.sdk = c.buildSDKClient(cfg)
}

func (c *Client) buildSDKClient(cfg ClientConfig) *openai.Client {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	baseURL := strings.TrimSpace(cfg.APIBaseURL)
	if baseURL != "" {
		baseURL = strings.TrimRight(baseURL, "/")
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)
	return &client
}

// ChatStream sends a streaming chat completion request.
func (c *Client) ChatStream(ctx context.Context, messages []ChatMessage, callback StreamCallback) (*CompletionResult, error) {
	c.mu.RLock()
	sdk := c.sdk
	model := c.config.Model
	temperature := c.config.Temperature
	tools := c.tools
	c.mu.RUnlock()

	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(model),
		Messages:    convertMessages(messages),
		Temperature: param.NewOpt(temperature),
	}

	if len(tools) > 0 {
		params.Tools = convertTools(tools)
	}

	stream := sdk.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	var (
		contentBuf   strings.Builder
		reasoningBuf strings.Builder
		toolCallAccs = make(map[int64]*toolCallAccumulator)
	)

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// Content delta
		if delta.Content != "" {
			contentBuf.WriteString(delta.Content)
			if callback != nil {
				callback(StreamDelta{Type: "content", Content: delta.Content})
			}
		}

		// Reasoning delta (extracted from raw JSON for compatible providers)
		if reasoning := extractReasoningFromRaw(delta.RawJSON()); reasoning != "" {
			reasoningBuf.WriteString(reasoning)
			if callback != nil {
				callback(StreamDelta{Type: "reasoning", Reasoning: reasoning})
			}
		}

		// Tool call deltas
		for _, tc := range delta.ToolCalls {
			acc, ok := toolCallAccs[tc.Index]
			if !ok {
				acc = &toolCallAccumulator{}
				toolCallAccs[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			acc.argsBuf.WriteString(tc.Function.Arguments)

			if callback != nil {
				callback(StreamDelta{Type: "tool_calls_delta"})
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("llm stream error: %w", err)
	}

	result := &CompletionResult{
		Content:   contentBuf.String(),
		Reasoning: reasoningBuf.String(),
	}

	if len(toolCallAccs) > 0 {
		result.ToolCalls = assembleToolCalls(toolCallAccs)
	}

	return result, nil
}

type toolCallAccumulator struct {
	id      string
	name    string
	argsBuf strings.Builder
}

func assembleToolCalls(accs map[int64]*toolCallAccumulator) []ToolCall {
	calls := make([]ToolCall, 0, len(accs))
	for idx := int64(0); idx < int64(len(accs)); idx++ {
		acc, ok := accs[idx]
		if !ok {
			continue
		}
		argsStr := strings.TrimSpace(acc.argsBuf.String())
		var args map[string]any
		if argsStr != "" {
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				args = map[string]any{"_raw": argsStr}
			}
		}
		calls = append(calls, ToolCall{
			ID:   acc.id,
			Name: acc.name,
			Args: args,
		})
	}
	return calls
}

func convertMessages(messages []ChatMessage) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			out = append(out, openai.SystemMessage(msg.Content))
		case "user":
			out = append(out, openai.UserMessage(msg.Content))
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					argsJSON, err := json.Marshal(tc.Args)
					if err != nil {
						argsJSON = []byte("{}")
					}
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: string(argsJSON),
							},
						},
					})
				}
				out = append(out, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						Content:   openai.ChatCompletionAssistantMessageParamContentUnion{OfString: param.NewOpt(msg.Content)},
						ToolCalls: toolCalls,
					},
				})
			} else {
				out = append(out, openai.AssistantMessage(msg.Content))
			}
		case "tool":
			out = append(out, openai.ToolMessage(msg.Content, msg.ToolCallID))
		default:
			out = append(out, openai.UserMessage(msg.Content))
		}
	}
	return out
}

func convertTools(tools []ToolDefinition) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        t.Name,
			Description: param.NewOpt(t.Description),
			Parameters:  shared.FunctionParameters(t.Parameters),
		}))
	}
	return out
}

func extractReasoningFromRaw(raw string) string {
	if raw == "" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ""
	}
	for _, key := range []string{"reasoning_content", "reasoning"} {
		if val, ok := parsed[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
