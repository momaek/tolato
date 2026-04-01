package agentapi

import (
	"encoding/json"
	"strings"
)

type ToolSpec struct {
	Type     string       `json:"type"`
	Function FunctionSpec `json:"function"`
}

type FunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
}

type Item struct {
	ID        string          `json:"id,omitempty"`
	Type      string          `json:"type,omitempty"`
	Role      string          `json:"role,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Output    string          `json:"output,omitempty"`
	Status    string          `json:"status,omitempty"`
	Summary   json.RawMessage `json:"summary,omitempty"`
}

type ContentPart struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

func NewFunctionTool(name, desc string, parameters map[string]any) ToolSpec {
	return ToolSpec{
		Type: "function",
		Function: FunctionSpec{
			Name:        strings.TrimSpace(name),
			Description: strings.TrimSpace(desc),
			Parameters:  parameters,
		},
	}
}

func UserMessage(text string) Item {
	return Item{
		Role:    "user",
		Content: mustMarshalRaw(text),
	}
}

func AssistantMessage(text string) Item {
	return Item{
		Role:    "assistant",
		Content: mustMarshalRaw(text),
	}
}

func FunctionCallOutput(callID string, output string) Item {
	return Item{
		Type:   "function_call_output",
		CallID: strings.TrimSpace(callID),
		Output: output,
	}
}

func FunctionCall(name string, args any) Item {
	return Item{
		Type:      "function_call",
		Name:      strings.TrimSpace(name),
		Arguments: jsonString(args),
		CallID:    "call_" + strings.TrimSpace(name),
	}
}

func MessageText(item Item) string {
	if len(item.Content) == 0 {
		return ""
	}

	var plain string
	if err := json.Unmarshal(item.Content, &plain); err == nil {
		return plain
	}

	var parts []ContentPart
	if err := json.Unmarshal(item.Content, &parts); err != nil {
		return ""
	}

	var builder strings.Builder
	for _, part := range parts {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		builder.WriteString(part.Text)
	}
	return builder.String()
}

func ArgumentsJSON(item Item) json.RawMessage {
	if strings.TrimSpace(item.Arguments) == "" {
		return nil
	}
	return json.RawMessage(item.Arguments)
}

func CloneItems(items []Item) []Item {
	if len(items) == 0 {
		return nil
	}
	out := make([]Item, 0, len(items))
	for _, item := range items {
		out = append(out, cloneItem(item))
	}
	return out
}

func cloneItem(in Item) Item {
	return Item{
		ID:        in.ID,
		Type:      in.Type,
		Role:      in.Role,
		Content:   cloneRaw(in.Content),
		Name:      in.Name,
		Arguments: in.Arguments,
		CallID:    in.CallID,
		Output:    in.Output,
		Status:    in.Status,
		Summary:   cloneRaw(in.Summary),
	}
}

func cloneRaw(in []byte) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func mustMarshalRaw(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func jsonString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}
