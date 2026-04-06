package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// APIKeyAuth is a Gin middleware that validates API keys.
func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "missing authorization header",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid authorization header format",
			})
			return
		}

		apiKey := parts[1]
		keyHash := HashAPIKey(apiKey)

		key, err := store.GetAPIKeyByHash(keyHash)
		if err != nil || key.Status != "active" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid or revoked API key",
			})
			return
		}

		// Update last_used_at
		now := time.Now()
		store.UpdateAPIKey(key.ID, map[string]any{"last_used_at": &now})

		c.Set("api_key_id", key.ID)
		c.Set("api_key_permission", key.Permission)
		c.Next()
	}
}

// HashAPIKey returns the SHA-256 hash of an API key.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
