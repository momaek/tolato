package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

type CreateAPIKeyRequest struct {
	Name       string `json:"name" binding:"required"`
	Permission string `json:"permission" binding:"required"` // readonly, standard, admin
}

type CreateAPIKeyResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Key        string `json:"key"` // shown only once
	KeyPrefix  string `json:"key_prefix"`
	Permission string `json:"permission"`
}

type APIKeyListItem struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	KeyPrefix  string  `json:"key_prefix"`
	Permission string  `json:"permission"`
	Status     string  `json:"status"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

// CreateAPIKey generates a new API key.
func CreateAPIKey(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateAPIKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_request", Message: err.Error()})
			return
		}

		if req.Permission != "readonly" && req.Permission != "standard" && req.Permission != "admin" {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid_permission", Message: "Permission must be readonly, standard, or admin"})
			return
		}

		// Generate random API key
		rawKey := make([]byte, 32)
		rand.Read(rawKey)
		keyStr := "tlat_" + hex.EncodeToString(rawKey)
		keyHash := middleware.HashAPIKey(keyStr)
		keyPrefix := keyStr[:12]

		apiKey := &model.APIKey{
			ID:         uuid.New().String(),
			Name:       req.Name,
			KeyHash:    keyHash,
			KeyPrefix:  keyPrefix,
			Permission: req.Permission,
			Status:     "active",
		}

		if err := store.CreateAPIKey(apiKey); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: "Failed to create API key"})
			return
		}

		c.JSON(http.StatusCreated, CreateAPIKeyResponse{
			ID:         apiKey.ID,
			Name:       apiKey.Name,
			Key:        keyStr,
			KeyPrefix:  keyPrefix,
			Permission: apiKey.Permission,
		})
	}
}

// ListAPIKeys returns all API keys.
func ListAPIKeys(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		keys, err := store.ListAPIKeys()
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: "Failed to list API keys"})
			return
		}

		items := make([]APIKeyListItem, 0, len(keys))
		for _, k := range keys {
			item := APIKeyListItem{
				ID:         k.ID,
				Name:       k.Name,
				KeyPrefix:  k.KeyPrefix,
				Permission: k.Permission,
				Status:     k.Status,
				CreatedAt:  k.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			if k.LastUsedAt != nil {
				s := k.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
				item.LastUsedAt = &s
			}
			items = append(items, item)
		}
		c.JSON(http.StatusOK, items)
	}
}

// DeleteAPIKey revokes an API key.
func DeleteAPIKey(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := store.UpdateAPIKey(id, map[string]any{"status": "revoked"}); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "db_error", Message: "Failed to revoke API key"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}
