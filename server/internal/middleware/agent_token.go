package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// AgentTokenAuth validates Agent requests using "Bearer <node_id>:<secret>" header.
// On success, sets "agent_node_id" in the Gin context.
func AgentTokenAuth() gin.HandlerFunc {
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

		// Expect "node_id:secret"
		credentials := strings.SplitN(parts[1], ":", 2)
		if len(credentials) != 2 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid agent token format, expected node_id:secret",
			})
			return
		}

		nodeID := credentials[0]
		secret := credentials[1]

		if _, err := store.GetNodeBySecret(nodeID, secret); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid node_id or secret",
			})
			return
		}

		c.Set("agent_node_id", nodeID)
		c.Next()
	}
}
