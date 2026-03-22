package mock

import (
	"context"
	"testing"

	"github.com/momaek/tolato/internal/server/app/runtime"
)

func TestProviderRunTurn(t *testing.T) {
	provider := New([]runtime.ModelTurnOutput{{
		AssistantText: strPtr("hello"),
		Done:          true,
	}})

	out, err := provider.RunTurn(context.Background(), runtime.ModelTurnInput{}, nil)
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if out.AssistantText == nil || *out.AssistantText != "hello" {
		t.Fatalf("output = %#v, want assistant text", out)
	}
}

func strPtr(v string) *string { return &v }
