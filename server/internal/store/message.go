package store

import (
	"github.com/momaek/tolato/server/internal/model"
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
