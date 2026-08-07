package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
	"gorm.io/gorm"
)

// Output preview caps applied on the history GET path. Command output is stored
// in full, but the conversation payload only carries a head preview so a single
// chat with many large outputs stays small; the full text is fetched on demand
// via GetToolCallOutput when the user expands and clicks "view all".
const (
	previewMaxLines = 80
	previewMaxBytes = 12000
)

// ownsConversation guards the sub-resource routes, whose store lookups key on
// the conversation ID alone. It writes the 404 itself and reports whether the
// caller may continue — a conversation owned by somebody else is indistinguishable
// from one that doesn't exist.
func ownsConversation(c *gin.Context, convID string) bool {
	ok, err := store.ConversationBelongsTo(middleware.CurrentUserID(c), convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to verify conversation",
		})
		return false
	}
	if !ok {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "not_found",
			Message: "conversation not found",
		})
		return false
	}
	return true
}

// CreateConversation handles POST /api/conversations.
func CreateConversation(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.CreateConversationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "invalid request body",
			})
			return
		}

		// A conversation can pin a default node; validate it, or the picker
		// becomes a way to confirm which node ids exist.
		if req.DefaultNodeID != nil && *req.DefaultNodeID != "" {
			if !requireNodeLevel(c, *req.DefaultNodeID, model.LevelViewer) {
				return
			}
		}

		conv := &model.Conversation{
			ID:            uuid.New().String(),
			UserID:        middleware.CurrentUserID(c),
			Title:         req.Title,
			Model:         req.Model,
			DefaultNodeID: req.DefaultNodeID,
		}
		if conv.Title == "" {
			conv.Title = "新对话"
		}

		if err := store.CreateConversation(conv); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to create conversation",
			})
			return
		}

		c.JSON(http.StatusCreated, conv)
	}
}

// ListConversations handles GET /api/conversations.
func ListConversations(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.PaginationQuery
		if err := c.ShouldBindQuery(&q); err != nil {
			q = model.PaginationQuery{}
		}
		if q.Page <= 0 {
			q.Page = 1
		}
		if q.PageSize <= 0 || q.PageSize > 100 {
			q.PageSize = 20
		}

		convs, total, err := store.ListConversations(middleware.CurrentUserID(c), q.Page, q.PageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to list conversations",
			})
			return
		}

		items := make([]model.ConversationSummary, 0, len(convs))
		for _, conv := range convs {
			items = append(items, model.ConversationSummary{
				ID:        conv.ID,
				Title:     conv.Title,
				Model:     conv.Model,
				CreatedAt: conv.CreatedAt,
				UpdatedAt: conv.UpdatedAt,
			})
		}

		totalPages := int(total) / q.PageSize
		if int(total)%q.PageSize > 0 {
			totalPages++
		}

		c.JSON(http.StatusOK, model.PaginatedResponse{
			Items:      items,
			Total:      int(total),
			Page:       q.Page,
			PageSize:   q.PageSize,
			TotalPages: totalPages,
		})
	}
}

// GetConversation handles GET /api/conversations/:id.
func GetConversation(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		conv, err := store.GetConversationByID(middleware.CurrentUserID(c), id)
		if err != nil {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "conversation not found",
			})
			return
		}

		messages := buildMessageItems(conv.Messages)

		detail := model.ConversationDetail{
			ID:            conv.ID,
			Title:         conv.Title,
			Model:         conv.Model,
			DefaultNodeID: conv.DefaultNodeID,
			Messages:      messages,
			CreatedAt:     conv.CreatedAt,
			UpdatedAt:     conv.UpdatedAt,
		}

		c.JSON(http.StatusOK, detail)
	}
}

// UpdateConversation handles PUT /api/conversations/:id.
func UpdateConversation(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		var req model.UpdateConversationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "invalid request body",
			})
			return
		}

		updates := make(map[string]any)
		if req.Title != nil {
			updates["title"] = *req.Title
		}
		if req.Model != nil {
			updates["model"] = *req.Model
		}
		if req.DefaultNodeID != nil {
			if *req.DefaultNodeID != "" && !requireNodeLevel(c, *req.DefaultNodeID, model.LevelViewer) {
				return
			}
			updates["default_node_id"] = *req.DefaultNodeID
		}

		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "no fields to update",
			})
			return
		}

		userID := middleware.CurrentUserID(c)
		if err := store.UpdateConversation(userID, id, updates); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to update conversation",
			})
			return
		}

		conv, err := store.GetConversationByID(userID, id)
		if err != nil {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "conversation not found",
			})
			return
		}
		c.JSON(http.StatusOK, conv)
	}
}

// DeleteConversation handles DELETE /api/conversations/:id.
func DeleteConversation(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		if err := store.DeleteConversation(middleware.CurrentUserID(c), id); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to delete conversation",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}

// DeleteMessage handles DELETE /api/conversations/:id/messages/:messageId.
// Removes a single message (and, for assistant messages, the tool-role
// messages that hold its tool results) from the conversation history.
func DeleteMessage(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		convID := c.Param("id")
		msgID := c.Param("messageId")

		if !ownsConversation(c, convID) {
			return
		}

		if err := store.DeleteMessage(convID, msgID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, model.ErrorResponse{
					Error:   "not_found",
					Message: "message not found",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to delete message",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}

// GetToolCallOutput handles GET /api/conversations/:id/tool-calls/:toolCallId/output.
// It returns the full, untruncated stdout/stderr for a single tool call — the
// lazy-load counterpart to the head preview embedded in GetConversation.
func GetToolCallOutput(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		convID := c.Param("id")
		toolCallID := c.Param("toolCallId")

		if !ownsConversation(c, convID) {
			return
		}

		m, err := store.GetToolResultByCallID(convID, toolCallID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, model.ErrorResponse{
					Error:   "not_found",
					Message: "tool call output not found",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to load tool call output",
			})
			return
		}

		resp := model.ToolCallOutputResponse{}
		if m.Content != nil {
			var r model.ToolResultItem
			if err := json.Unmarshal([]byte(*m.Content), &r); err == nil {
				resp.Stdout = r.Stdout
				resp.Stderr = r.Stderr
			} else {
				// Malformed row — hand back the raw content so the UI shows
				// something rather than an empty body.
				raw := *m.Content
				resp.Stdout = &raw
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}

// previewText returns a head preview of s capped at previewMaxLines lines and
// previewMaxBytes bytes (whichever hits first), the full line count, and whether
// any capping happened. The byte cap is backed off to a valid UTF-8 boundary so
// a multibyte rune is never split.
func previewText(s string) (preview string, totalLines int, truncated bool) {
	if s == "" {
		return "", 0, false
	}
	totalLines = strings.Count(s, "\n") + 1
	preview = s

	if totalLines > previewMaxLines {
		if idx := nthByteIndex(s, '\n', previewMaxLines); idx >= 0 {
			preview = s[:idx]
			truncated = true
		}
	}
	if len(preview) > previewMaxBytes {
		preview = preview[:previewMaxBytes]
		for len(preview) > 0 && !utf8.ValidString(preview) {
			preview = preview[:len(preview)-1]
		}
		truncated = true
	}
	return preview, totalLines, truncated
}

// nthByteIndex returns the byte index of the nth occurrence of c in s, or -1.
func nthByteIndex(s string, c byte, n int) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			if count++; count == n {
				return i
			}
		}
	}
	return -1
}

// previewToolResult returns a copy of r with stdout/stderr replaced by head
// previews plus full line counts, so large outputs don't bloat the conversation
// payload. nil-safe; results without stdout/stderr (e.g. non-command tools) pass
// through untouched.
func previewToolResult(r *model.ToolResultItem) *model.ToolResultItem {
	if r == nil {
		return nil
	}
	out := *r
	if r.Stdout != nil {
		p, lines, tr := previewText(*r.Stdout)
		out.Stdout = &p
		ln := lines
		out.StdoutLines = &ln
		out.Truncated = out.Truncated || tr
	}
	if r.Stderr != nil {
		p, lines, tr := previewText(*r.Stderr)
		out.Stderr = &p
		ln := lines
		out.StderrLines = &ln
		out.Truncated = out.Truncated || tr
	}
	return &out
}

// storedToolCall matches the JSON shape produced by agent/engine.marshalToolCalls
// (which marshals []llm.ToolCall — that struct has no JSON tags, so keys are
// capitalized). Decoupled from llm.ToolCall so handler doesn't import llm.
type storedToolCall struct {
	ID   string         `json:"ID"`
	Name string         `json:"Name"`
	Args map[string]any `json:"Args"`
}

// buildMessageItems converts stored DB messages into API MessageItems for the
// frontend. It parses the tool_calls JSON column on assistant messages and
// inlines each tool-role message's result onto its originating tool_call,
// dropping the tool-role messages from the output (they're purely transport
// to the LLM and have no standalone UI representation).
func buildMessageItems(dbMsgs []model.Message) []model.MessageItem {
	// Index tool results by tool_call_id for O(1) attachment.
	toolResults := make(map[string]*model.ToolResultItem, len(dbMsgs))
	for _, m := range dbMsgs {
		if m.Role != "tool" || m.ToolCallID == nil || m.Content == nil {
			continue
		}
		var r model.ToolResultItem
		if err := json.Unmarshal([]byte(*m.Content), &r); err != nil {
			// Malformed (older rows, manual edits) — expose the raw string so
			// the UI at least shows something instead of silently dropping it.
			raw := *m.Content
			toolResults[*m.ToolCallID] = &model.ToolResultItem{
				Data: map[string]any{"raw": raw},
			}
			continue
		}
		toolResults[*m.ToolCallID] = &r
	}

	items := make([]model.MessageItem, 0, len(dbMsgs))
	for _, msg := range dbMsgs {
		// Tool-role messages are inlined into the assistant's tool_calls above.
		if msg.Role == "tool" {
			continue
		}

		item := model.MessageItem{
			ID:         msg.ID,
			Role:       msg.Role,
			Content:    msg.Content,
			Reasoning:  msg.Reasoning,
			ToolCallID: msg.ToolCallID,
			CreatedAt:  msg.CreatedAt,
		}

		if msg.ToolCalls != nil && *msg.ToolCalls != "" {
			var stored []storedToolCall
			if err := json.Unmarshal([]byte(*msg.ToolCalls), &stored); err != nil {
				log.Printf("[conversation] failed to parse tool_calls for msg %s: %v", msg.ID, err)
			} else if len(stored) > 0 {
				calls := make([]model.ToolCallItem, 0, len(stored))
				for _, s := range stored {
					calls = append(calls, model.ToolCallItem{
						ID:     s.ID,
						Tool:   s.Name, // stored Name → API tool
						Args:   s.Args,
						Result: previewToolResult(toolResults[s.ID]),
					})
				}
				item.ToolCalls = calls
			}
		}

		items = append(items, item)
	}
	return items
}
