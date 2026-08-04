package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/auth"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
	"gorm.io/gorm"
)

// ListUsers handles GET /api/users (admin only).
func ListUsers(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := store.ListUsers()
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to list users",
			})
			return
		}

		items := make([]model.UserItem, 0, len(users))
		for i := range users {
			items = append(items, toUserItem(&users[i]))
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

// CreateUser handles POST /api/users (admin only).
func CreateUser(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.CreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "invalid request body",
			})
			return
		}

		username := strings.TrimSpace(req.Username)
		if username == "" {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "username is required",
			})
			return
		}
		role, ok := normalizeRole(req.Role)
		if !ok {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "role must be admin or member",
			})
			return
		}
		if err := auth.ValidatePassword(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "password must be at least 8 characters",
			})
			return
		}

		if _, err := store.GetUserByUsername(username); err == nil {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error:   "conflict",
				Message: "username already taken",
			})
			return
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to check username",
			})
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to hash password",
			})
			return
		}

		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" {
			displayName = username
		}

		u := &model.User{
			ID:           uuid.New().String(),
			Username:     username,
			PasswordHash: hash,
			DisplayName:  displayName,
			Email:        strings.TrimSpace(req.Email),
			Role:         role,
			Status:       model.UserStatusActive,
			AuthSource:   model.AuthSourceLocal,
		}
		if err := store.CreateUser(u); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to create user",
			})
			return
		}

		c.JSON(http.StatusCreated, toUserItem(u))
	}
}

// UpdateUser handles PUT /api/users/:id (admin only).
func UpdateUser(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		var req model.UpdateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "invalid request body",
			})
			return
		}

		target, err := store.GetUserByID(id)
		if err != nil {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "user not found",
			})
			return
		}

		updates := make(map[string]any)
		if req.DisplayName != nil {
			updates["display_name"] = strings.TrimSpace(*req.DisplayName)
		}
		if req.Email != nil {
			updates["email"] = strings.TrimSpace(*req.Email)
		}

		if req.Role != nil {
			role, ok := normalizeRole(*req.Role)
			if !ok {
				c.JSON(http.StatusBadRequest, model.ErrorResponse{
					Error:   "bad_request",
					Message: "role must be admin or member",
				})
				return
			}
			if target.Role == model.RoleAdmin && role != model.RoleAdmin {
				if msg, ok := blockedByLastAdmin(target); !ok {
					c.JSON(http.StatusConflict, model.ErrorResponse{Error: "conflict", Message: msg})
					return
				}
			}
			updates["role"] = role
		}

		if req.Status != nil {
			status := *req.Status
			if status != model.UserStatusActive && status != model.UserStatusDisabled {
				c.JSON(http.StatusBadRequest, model.ErrorResponse{
					Error:   "bad_request",
					Message: "status must be active or disabled",
				})
				return
			}
			if status == model.UserStatusDisabled {
				if target.ID == middleware.CurrentUserID(c) {
					c.JSON(http.StatusConflict, model.ErrorResponse{
						Error:   "conflict",
						Message: "you cannot disable your own account",
					})
					return
				}
				if target.Role == model.RoleAdmin {
					if msg, ok := blockedByLastAdmin(target); !ok {
						c.JSON(http.StatusConflict, model.ErrorResponse{Error: "conflict", Message: msg})
						return
					}
				}
			}
			updates["status"] = status
		}

		if req.Password != nil {
			if target.AuthSource != model.AuthSourceLocal {
				c.JSON(http.StatusBadRequest, model.ErrorResponse{
					Error:   "bad_request",
					Message: "single sign-on accounts have no local password",
				})
				return
			}
			if err := auth.ValidatePassword(*req.Password); err != nil {
				c.JSON(http.StatusBadRequest, model.ErrorResponse{
					Error:   "bad_request",
					Message: "password must be at least 8 characters",
				})
				return
			}
			hash, err := auth.HashPassword(*req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, model.ErrorResponse{
					Error:   "internal_error",
					Message: "failed to hash password",
				})
				return
			}
			updates["password_hash"] = hash
		}

		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "no fields to update",
			})
			return
		}

		if err := store.UpdateUser(id, updates); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to update user",
			})
			return
		}

		updated, err := store.GetUserByID(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to reload user",
			})
			return
		}
		c.JSON(http.StatusOK, toUserItem(updated))
	}
}

// DeleteUser handles DELETE /api/users/:id (admin only).
//
// The user's conversations go with them (they're private and meaningless to
// anyone else). Audit logs stay: they carry an actor username snapshot, so the
// history of what was run on which node survives the account.
func DeleteUser(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")

		if id == middleware.CurrentUserID(c) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error:   "conflict",
				Message: "you cannot delete your own account",
			})
			return
		}

		target, err := store.GetUserByID(id)
		if err != nil {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "user not found",
			})
			return
		}
		if target.Role == model.RoleAdmin {
			if msg, ok := blockedByLastAdmin(target); !ok {
				c.JSON(http.StatusConflict, model.ErrorResponse{Error: "conflict", Message: msg})
				return
			}
		}

		if err := store.DeleteUserCascade(id); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to delete user",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}

// normalizeRole validates a role string, defaulting an empty value to member.
func normalizeRole(role string) (string, bool) {
	switch role {
	case "":
		return model.RoleMember, true
	case model.RoleAdmin, model.RoleMember:
		return role, true
	default:
		return "", false
	}
}

// blockedByLastAdmin guards the operations that would leave the instance with
// no active admin — nobody could then manage users or settings again. Returns
// the message to surface when the operation must be refused.
func blockedByLastAdmin(target *model.User) (string, bool) {
	if target.Status != model.UserStatusActive {
		// Already inactive, so it isn't one of the admins keeping the lights on.
		return "", true
	}
	n, err := store.CountAdmins()
	if err != nil {
		return "failed to verify remaining administrators", false
	}
	if n <= 1 {
		return "this is the last active administrator", false
	}
	return "", true
}
