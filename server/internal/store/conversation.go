package store

import (
	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/gorm"
)

// Conversations are private to their owner — including from admins, who manage
// machines rather than read other people's chats. Every accessor below is
// therefore scoped by userID, with no unscoped variant to reach for by mistake.

// CreateConversation creates a new conversation.
func CreateConversation(conv *model.Conversation) error {
	return DB.Create(conv).Error
}

// ListConversations returns one page of the user's conversations (no messages).
func ListConversations(userID string, page, pageSize int) ([]model.Conversation, int64, error) {
	var total int64
	if err := DB.Model(&model.Conversation{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var convs []model.Conversation
	offset := (page - 1) * pageSize
	err := DB.Where("user_id = ?", userID).
		Order("updated_at DESC").Offset(offset).Limit(pageSize).Find(&convs).Error
	return convs, total, err
}

// GetConversationByID returns the user's conversation with its messages. A
// conversation belonging to somebody else reads as not found.
func GetConversationByID(userID, id string) (*model.Conversation, error) {
	var conv model.Conversation
	err := DB.Preload("Messages", func(db *gorm.DB) *gorm.DB {
		return db.Order("seq ASC")
	}).Where("user_id = ?", userID).First(&conv, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// ConversationBelongsTo reports whether the conversation exists and is owned by
// the user. Used where the messages aren't needed, e.g. the WebSocket handshake.
func ConversationBelongsTo(userID, id string) (bool, error) {
	var n int64
	err := DB.Model(&model.Conversation{}).
		Where("id = ? AND user_id = ?", id, userID).Count(&n).Error
	return n > 0, err
}

// UpdateConversation updates a conversation's fields.
func UpdateConversation(userID, id string, updates map[string]any) error {
	return DB.Model(&model.Conversation{}).
		Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error
}

// DeleteConversation deletes a conversation and its messages (via CASCADE).
func DeleteConversation(userID, id string) error {
	return DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.Conversation{}).Error
}
