package runtime

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/momaek/tolato/internal/server/agentapi"
	"github.com/momaek/tolato/internal/server/domain"
)

type conversationState struct {
	Conversation []agentapi.Item `json:"conversation,omitempty"`
	Provider     json.RawMessage `json:"providerState,omitempty"`
}

func (r *Runtime) loadConversationState(ctx context.Context, session domain.Session) ([]agentapi.Item, json.RawMessage, error) {
	if len(session.ProviderStateBlob) > 0 && string(session.ProviderStateBlob) != "null" {
		var state conversationState
		if err := json.Unmarshal(session.ProviderStateBlob, &state); err == nil {
			return agentapi.CloneItems(state.Conversation), cloneRaw(state.Provider), nil
		}
	}

	conversation, err := r.rebuildConversation(ctx, session.ID)
	if err != nil {
		return nil, nil, err
	}
	return conversation, nil, nil
}

func (r *Runtime) persistConversationState(ctx context.Context, session *domain.Session, conversation []agentapi.Item, providerState json.RawMessage) error {
	if session == nil {
		return nil
	}
	session.ProviderStateBlob = mustMarshalJSON(conversationState{
		Conversation: agentapi.CloneItems(conversation),
		Provider:     cloneRaw(providerState),
	})
	return r.bumpSession(ctx, session)
}

func (r *Runtime) rebuildConversation(ctx context.Context, sessionID string) ([]agentapi.Item, error) {
	messages, err := r.repos.Messages.ListBySession(ctx, sessionID, domain.CursorPage{})
	if err != nil {
		return nil, err
	}
	items := make([]agentapi.Item, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case domain.MessageRoleUser:
			items = append(items, agentapi.UserMessage(msg.Content))
		case domain.MessageRoleAssistant:
			items = append(items, agentapi.AssistantMessage(msg.Content))
		}
	}
	return items, nil
}

func firstFunctionCall(items []agentapi.Item) (agentapi.Item, bool) {
	for _, item := range items {
		if strings.TrimSpace(item.Type) == "function_call" {
			return item, true
		}
	}
	return agentapi.Item{}, false
}

func outputMessageText(items []agentapi.Item) string {
	var builder strings.Builder
	for _, item := range items {
		if strings.TrimSpace(item.Type) != "message" && strings.TrimSpace(item.Role) == "" {
			continue
		}
		builder.WriteString(agentapi.MessageText(item))
	}
	return strings.TrimSpace(builder.String())
}
