package store

import (
	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/gorm"
)

// CreateConversation creates a new conversation.
func CreateConversation(conv *model.Conversation) error {
	return DB.Create(conv).Error
}

// ListConversations returns paginated conversations (without messages).
func ListConversations(page, pageSize int) ([]model.Conversation, int64, error) {
	var total int64
	if err := DB.Model(&model.Conversation{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var convs []model.Conversation
	offset := (page - 1) * pageSize
	err := DB.Order("updated_at DESC").Offset(offset).Limit(pageSize).Find(&convs).Error
	return convs, total, err
}

// GetConversationByID returns a conversation with its messages.
func GetConversationByID(id string) (*model.Conversation, error) {
	var conv model.Conversation
	err := DB.Preload("Messages", func(db *gorm.DB) *gorm.DB {
		return db.Order("seq ASC")
	}).First(&conv, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// UpdateConversation updates a conversation's fields.
func UpdateConversation(id string, updates map[string]any) error {
	return DB.Model(&model.Conversation{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteConversation deletes a conversation and its messages (via CASCADE).
func DeleteConversation(id string) error {
	return DB.Where("id = ?", id).Delete(&model.Conversation{}).Error
}
