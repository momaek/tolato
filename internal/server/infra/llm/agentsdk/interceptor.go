package agentsdk

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/momaek/tolato/internal/server/agentapi"
)

// InterceptedCall carries a tool invocation from agent-sdk-go to the Runtime.
type InterceptedCall struct {
	Name      string
	Arguments string
}

// ToolCallResult carries the Runtime's answer back to the blocked Execute().
type ToolCallResult struct {
	Output string
	Error  error
}

// interceptedTool wraps a ToLaTo tool spec as an interfaces.Tool whose Execute
// blocks on a channel, giving the Runtime full control over tool execution.
type interceptedTool struct {
	name        string
	description string
	parameters  map[string]interfaces.ParameterSpec
	callChan    chan<- InterceptedCall
	resultChan  <-chan ToolCallResult
}

var _ interfaces.Tool = (*interceptedTool)(nil)

func (t *interceptedTool) Name() string                                    { return t.name }
func (t *interceptedTool) Description() string                             { return t.description }
func (t *interceptedTool) Parameters() map[string]interfaces.ParameterSpec { return t.parameters }

func (t *interceptedTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute sends the tool call to the Runtime via callChan and blocks until
// the Runtime provides a result via resultChan. This is the key mechanism
// that gives ToLaTo's Runtime full control over each tool invocation.
func (t *interceptedTool) Execute(ctx context.Context, args string) (string, error) {
	select {
	case t.callChan <- InterceptedCall{Name: t.name, Arguments: args}:
	case <-ctx.Done():
		return "", fmt.Errorf("intercepted tool %q: context cancelled before send: %w", t.name, ctx.Err())
	}

	select {
	case result := <-t.resultChan:
		if result.Error != nil {
			return "", result.Error
		}
		return result.Output, nil
	case <-ctx.Done():
		return "", fmt.Errorf("intercepted tool %q: context cancelled waiting for result: %w", t.name, ctx.Err())
	}
}

// wrapToolSpecs converts ToLaTo agentapi.ToolSpec slice into interceptedTool
// instances that share the given channels.
func wrapToolSpecs(specs []agentapi.ToolSpec, callChan chan<- InterceptedCall, resultChan <-chan ToolCallResult) []interfaces.Tool {
	tools := make([]interfaces.Tool, 0, len(specs))
	for _, spec := range specs {
		tools = append(tools, &interceptedTool{
			name:        spec.Function.Name,
			description: spec.Function.Description,
			parameters:  convertParameters(spec.Function.Parameters),
			callChan:    callChan,
			resultChan:  resultChan,
		})
	}
	return tools
}

// convertParameters converts agentapi parameter schema to interfaces.ParameterSpec.
func convertParameters(params map[string]any) map[string]interfaces.ParameterSpec {
	if len(params) == 0 {
		return nil
	}
	properties, _ := params["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil
	}

	requiredSlice, _ := params["required"].([]any)
	requiredSet := make(map[string]bool, len(requiredSlice))
	for _, r := range requiredSlice {
		if s, ok := r.(string); ok {
			requiredSet[s] = true
		}
	}

	result := make(map[string]interfaces.ParameterSpec, len(properties))
	for name, raw := range properties {
		prop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		spec := interfaces.ParameterSpec{
			Required: requiredSet[name],
		}
		if t, ok := prop["type"].(string); ok {
			spec.Type = t
		}
		if d, ok := prop["description"].(string); ok {
			spec.Description = d
		}
		result[name] = spec
	}
	return result
}
