package handler

import (
	"context"
	"log"
	"strings"

	"github.com/momaek/tolato/server/internal/agent"
	"github.com/momaek/tolato/server/internal/llm"
	"github.com/momaek/tolato/server/internal/store"
)

// defaultConversationTitle mirrors the literal used in CreateConversation. We
// only auto-overwrite this exact value (or empty) so a manual rename is never
// clobbered by a stale title-gen request.
const defaultConversationTitle = "新对话"

// titleGenSystemPrompt is intentionally terse: we want a single short label,
// not a paragraph. The model must reply in the same language the user wrote in.
const titleGenSystemPrompt = `你是一个会话标题生成器。根据用户的第一条消息，用与用户相同的语言生成一个不超过 20 字的简短标题。要求：只输出标题本身，不要带引号、句号、解释或前缀（如"标题："）。`

// isFirstUserMessage reports whether the conversation has zero persisted
// messages. Callers use this to decide whether to fire title generation —
// the answer must be read before the engine persists the incoming user
// message, otherwise it'll see seq>=1 and skip generation forever.
func isFirstUserMessage(convID string) bool {
	seq, err := store.GetMaxSeq(convID)
	if err != nil {
		return false
	}
	return seq == 0
}

// generateAndEmitTitle calls the LLM with a one-shot prompt to summarize the
// user's first message into a short title, persists it, and pushes a
// title_update event back to the frontend over eventCh. Runs in its own
// goroutine; failures are logged and swallowed (title generation is
// best-effort — chat must continue regardless).
func generateAndEmitTitle(ctx context.Context, baseCfg llm.ClientConfig, convID, userContent string, eventCh chan<- any) {
	// Re-check the conversation's current title at emit time. If the user
	// renamed during the LLM call, we must not overwrite their choice.
	conv, err := store.GetConversationByID(convID)
	if err != nil {
		log.Printf("[title-gen] conv=%s lookup failed: %v", convID, err)
		return
	}
	if conv.Title != "" && conv.Title != defaultConversationTitle {
		return
	}

	// Use a tools-less client so the model doesn't try to call list_nodes etc.
	// when generating a title.
	titleClient := llm.NewClient(baseCfg, nil)
	messages := []llm.ChatMessage{
		{Role: "system", Content: titleGenSystemPrompt},
		{Role: "user", Content: userContent},
	}

	result, err := titleClient.ChatStream(ctx, messages, nil)
	if err != nil {
		log.Printf("[title-gen] conv=%s LLM error: %v", convID, err)
		return
	}

	title := sanitizeTitle(result.Content)
	if title == "" {
		return
	}

	// Re-check before persist (LLM call can take seconds; user may have
	// renamed in the meantime).
	conv, err = store.GetConversationByID(convID)
	if err != nil {
		return
	}
	if conv.Title != "" && conv.Title != defaultConversationTitle {
		return
	}

	if err := store.UpdateConversation(convID, map[string]any{"title": title}); err != nil {
		log.Printf("[title-gen] conv=%s persist failed: %v", convID, err)
		return
	}

	select {
	case eventCh <- agent.TitleUpdateEvent{ConversationID: convID, Title: title}:
	case <-ctx.Done():
	}
}

// sanitizeTitle strips wrapping quotes, common "标题:" prefixes, surrounding
// whitespace, and clamps to 60 runes. Models occasionally ignore the "no
// quotes / no prefix" instruction in the system prompt.
func sanitizeTitle(s string) string {
	s = strings.TrimSpace(s)
	// Take only the first non-empty line — some models add a second line of
	// commentary despite instructions.
	if idx := strings.IndexAny(s, "\r\n"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	for _, prefix := range []string{"标题:", "标题：", "Title:", "title:"} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimSpace(s[len(prefix):])
		}
	}
	s = strings.Trim(s, "\"'`“”‘’《》")
	s = strings.TrimSpace(s)

	// Clamp by rune count, not byte count (Chinese is multi-byte in UTF-8).
	const maxRunes = 60
	runes := []rune(s)
	if len(runes) > maxRunes {
		s = string(runes[:maxRunes])
	}
	return s
}
