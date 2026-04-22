package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

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

		conv := &model.Conversation{
			ID:            uuid.New().String(),
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

		convs, total, err := store.ListConversations(q.Page, q.PageSize)
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

		conv, err := store.GetConversationByID(id)
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
			updates["default_node_id"] = *req.DefaultNodeID
		}

		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "no fields to update",
			})
			return
		}

		if err := store.UpdateConversation(id, updates); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to update conversation",
			})
			return
		}

		conv, _ := store.GetConversationByID(id)
		c.JSON(http.StatusOK, conv)
	}
}

// DeleteConversation handles DELETE /api/conversations/:id.
func DeleteConversation(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		if err := store.DeleteConversation(id); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to delete conversation",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
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
						Result: toolResults[s.ID],
					})
				}
				item.ToolCalls = calls
			}
		}

		items = append(items, item)
	}
	return items
}
