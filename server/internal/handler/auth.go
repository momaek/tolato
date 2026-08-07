package handler

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/auth"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// LoginHandler handles POST /api/auth/login.
func LoginHandler(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "invalid request body",
			})
			return
		}

		// A wrong username and a wrong password are reported identically so the
		// response can't be used to enumerate accounts.
		user, err := store.GetUserByUsername(req.Username)
		if err != nil || !auth.CheckPassword(user.PasswordHash, req.Password) {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid username or password",
			})
			return
		}
		if user.Status != model.UserStatusActive {
			c.JSON(http.StatusForbidden, model.ErrorResponse{
				Error:   "forbidden",
				Message: "account is disabled",
			})
			return
		}

		issueSession(c, user)
	}
}

// issueSession mints a JWT for the user, stamps last_login_at, and writes the
// login response. Shared by the password and (later) OIDC login paths.
func issueSession(c *gin.Context, user *model.User) {
	token, expiresAt, err := middleware.GenerateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to generate token",
		})
		return
	}

	now := time.Now()
	if err := store.UpdateUser(user.ID, map[string]any{"last_login_at": &now}); err != nil {
		// Cosmetic field — a failure here shouldn't cost the user their login.
		log.Printf("[auth] failed to record last_login_at for %s: %v", user.Username, err)
	}

	c.JSON(http.StatusOK, model.LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      toUserItem(user),
	})
}

// CurrentUser handles GET /api/auth/me — lets a reloaded frontend recover the
// identity behind its stored token without decoding the JWT itself.
func CurrentUser(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := store.GetUserByID(middleware.CurrentUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "user not found",
			})
			return
		}
		c.JSON(http.StatusOK, toUserItem(user))
	}
}

// ChangeOwnPassword handles PUT /api/auth/password — self-service, requires the
// current password. Separate from the admin reset path, which does not.
func ChangeOwnPassword(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.ChangePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "invalid request body",
			})
			return
		}

		user, err := store.GetUserByID(middleware.CurrentUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "user not found",
			})
			return
		}
		if user.AuthSource != model.AuthSourceLocal {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "single sign-on accounts have no local password",
			})
			return
		}
		if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "current password is incorrect",
			})
			return
		}
		if err := auth.ValidatePassword(req.NewPassword); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "password must be at least 8 characters",
			})
			return
		}

		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to hash password",
			})
			return
		}
		if err := store.UpdateUser(user.ID, map[string]any{"password_hash": hash}); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to update password",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "password updated"})
	}
}

func toUserItem(u *model.User) model.UserItem {
	return model.UserItem{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		Role:        u.Role,
		Status:      u.Status,
		AuthSource:  u.AuthSource,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
	}
}
