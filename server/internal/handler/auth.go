package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
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

		// Validate credentials from config
		if req.Username != deps.Config.Auth.Username || req.Password != deps.Config.Auth.Password {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid username or password",
			})
			return
		}

		token, expiresAt, err := middleware.GenerateToken(req.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to generate token",
			})
			return
		}

		c.JSON(http.StatusOK, model.LoginResponse{
			Token:     token,
			ExpiresAt: expiresAt,
		})
	}
}
