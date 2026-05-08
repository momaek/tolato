package store

import (
	"encoding/json"

	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/gorm"
)

// BatchCreateMessages inserts multiple messages at once.
func BatchCreateMessages(messages []model.Message) error {
	if len(messages) == 0 {
		return nil
	}
	return DB.Create(&messages).Error
}

// ListMessagesByConversation returns all messages for a conversation ordered by seq.
func ListMessagesByConversation(conversationID string) ([]model.Message, error) {
	var messages []model.Message
	err := DB.Where("conversation_id = ?", conversationID).
		Order("seq ASC").
		Find(&messages).Error
	return messages, err
}

// GetMaxSeq returns the current max seq number for a conversation.
func GetMaxSeq(conversationID string) (int, error) {
	var maxSeq int
	err := DB.Model(&model.Message{}).
		Where("conversation_id = ?", conversationID).
		Select("COALESCE(MAX(seq), 0)").
		Scan(&maxSeq).Error
	return maxSeq, err
}

// GetMessage returns a single message by id, scoped to a conversation so the
// handler can refuse cross-conversation deletes from a forged URL.
func GetMessage(conversationID, messageID string) (*model.Message, error) {
	var m model.Message
	if err := DB.Where("id = ? AND conversation_id = ?", messageID, conversationID).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// DeleteMessage removes a single message. For assistant messages, any tool-role
// messages whose tool_call_id matches one of the assistant's stored tool_calls
// are deleted in the same transaction — leaving them behind would surface
// orphan tool results in the UI and confuse the LLM if the conversation is
// re-sent.
func DeleteMessage(conversationID, messageID string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var m model.Message
		if err := tx.Where("id = ? AND conversation_id = ?", messageID, conversationID).First(&m).Error; err != nil {
			return err
		}

		if m.Role == "assistant" && m.ToolCalls != nil && *m.ToolCalls != "" {
			// Mirror the loose shape produced by agent/engine.marshalToolCalls
			// (capitalized keys, since llm.ToolCall has no JSON tags). Same
			// approach as handler/conversation.go's storedToolCall.
			var stored []struct {
				ID string `json:"ID"`
			}
			if err := json.Unmarshal([]byte(*m.ToolCalls), &stored); err == nil && len(stored) > 0 {
				ids := make([]string, 0, len(stored))
				for _, s := range stored {
					if s.ID != "" {
						ids = append(ids, s.ID)
					}
				}
				if len(ids) > 0 {
					if err := tx.Where("conversation_id = ? AND role = ? AND tool_call_id IN ?", conversationID, "tool", ids).
						Delete(&model.Message{}).Error; err != nil {
						return err
					}
				}
			}
		}

		return tx.Delete(&m).Error
	})
}
