package store

import (
	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/gorm"
)

// CreateUser inserts a new user.
func CreateUser(u *model.User) error {
	return DB.Create(u).Error
}

// GetUserByID returns a user by primary key.
func GetUserByID(id string) (*model.User, error) {
	var u model.User
	if err := DB.First(&u, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByUsername returns a user by username.
func GetUserByUsername(username string) (*model.User, error) {
	var u model.User
	if err := DB.First(&u, "username = ?", username).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByOIDCSubject returns a user by their IdP subject claim.
func GetUserByOIDCSubject(sub string) (*model.User, error) {
	var u model.User
	if err := DB.First(&u, "oidc_subject = ?", sub).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUsers returns all users, newest first.
func ListUsers() ([]model.User, error) {
	var users []model.User
	err := DB.Order("created_at ASC").Find(&users).Error
	return users, err
}

// UpdateUser applies a partial update.
func UpdateUser(id string, updates map[string]any) error {
	return DB.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteUser removes a user row.
func DeleteUser(id string) error {
	return DB.Where("id = ?", id).Delete(&model.User{}).Error
}

// DeleteUserCascade removes a user together with the resources that only make
// sense while the account exists: their private conversations (and, by FK
// cascade, the messages inside them) and the API keys that acted as them.
//
// Audit logs are deliberately kept — they carry an actor username snapshot, so
// the record of what ran on which node outlives the account.
func DeleteUserCascade(id string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&model.Conversation{}).Error; err != nil {
			return err
		}
		if err := tx.Where("owner_user_id = ?", id).Delete(&model.APIKey{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.User{}).Error
	})
}

// CountUsers returns the total number of users. Used by the bootstrap path to
// decide whether the config credentials should seed the first admin.
func CountUsers() (int64, error) {
	var n int64
	err := DB.Model(&model.User{}).Count(&n).Error
	return n, err
}

// CountAdmins returns the number of active admins. Used to refuse the last
// admin being deleted, demoted, or disabled.
func CountAdmins() (int64, error) {
	var n int64
	err := DB.Model(&model.User{}).
		Where("role = ? AND status = ?", model.RoleAdmin, model.UserStatusActive).
		Count(&n).Error
	return n, err
}
