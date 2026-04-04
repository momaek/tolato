package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/node"
)

// Deps holds shared dependencies for all handlers.
type Deps struct {
	Config      *config.Config
	NodeManager *node.NodeManager
}

// SetupRouter creates and configures the Gin router with all routes.
func SetupRouter(deps *Deps) *gin.Engine {
	r := gin.Default()

	// CORS middleware
	r.Use(corsMiddleware())

	api := r.Group("/api")

	// Auth (no JWT required)
	api.POST("/auth/login", LoginHandler(deps))

	// JWT-protected routes
	protected := api.Group("")
	protected.Use(middleware.JWTAuth())

	// Conversations
	protected.POST("/conversations", CreateConversation(deps))
	protected.GET("/conversations", ListConversations(deps))
	protected.GET("/conversations/:id", GetConversation(deps))
	protected.PUT("/conversations/:id", UpdateConversation(deps))
	protected.DELETE("/conversations/:id", DeleteConversation(deps))

	// Nodes
	protected.POST("/nodes", CreateNode(deps))
	protected.GET("/nodes", ListNodes(deps))
	protected.GET("/nodes/:id", GetNode(deps))
	protected.PUT("/nodes/:id", UpdateNode(deps))
	protected.DELETE("/nodes/:id", DeleteNode(deps))

	// Settings
	protected.GET("/settings/llm", GetLLMSettings(deps))
	protected.PUT("/settings/llm", PutLLMSettings(deps))
	protected.GET("/settings/security", GetSecuritySettings(deps))
	protected.PUT("/settings/security", PutSecuritySettings(deps))
	protected.GET("/settings/agent", GetAgentSettings(deps))
	protected.PUT("/settings/agent", PutAgentSettings(deps))
	protected.GET("/settings/chat", GetChatSettings(deps))
	protected.PUT("/settings/chat", PutChatSettings(deps))

	// Audit Logs
	protected.GET("/audit-logs", ListAuditLogs(deps))

	// WebSocket: Agent connection (token-based auth, not JWT)
	r.GET("/ws/agent", AgentWSHandler(deps))

	return r
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
