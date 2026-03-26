package mock

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/app/runtime"
)

func TestProviderRunTurn(t *testing.T) {
	provider := New([]runtime.ModelTurnOutput{{
		Items: []agentapi.Item{assistantMessage("hello")},
		Done:  true,
	}})

	out, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{}, nil)
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(out.Items) != 1 || agentapi.MessageText(out.Items[0]) != "hello" {
		t.Fatalf("output = %#v, want assistant text", out)
	}
}

func assistantMessage(text string) agentapi.Item {
	raw, err := json.Marshal([]agentapi.ContentPart{{Type: "output_text", Text: text}})
	if err != nil {
		panic(err)
	}
	return agentapi.Item{Type: "message", Role: "assistant", Content: raw}
}
