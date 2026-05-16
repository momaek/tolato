package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/geoip"
	"github.com/momaek/tolato/server/internal/mcp"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/settings"
	"github.com/momaek/tolato/server/internal/webui"
)

// Deps holds shared dependencies for all handlers.
//
// Handlers should reach for data through these injected collaborators rather
// than the package-level `store.*` globals where practical — it keeps the
// dependency graph explicit and the seam available for tests.
type Deps struct {
	Config      *config.Config
	NodeManager *node.NodeManager
	Settings    *settings.Cache
	GeoIP       *geoip.Service // may be nil when geoip is disabled
}

// ValidateToken validates a JWT token string and returns the claims.
func (d *Deps) ValidateToken(tokenString string) (*middleware.Claims, error) {
	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return []byte(middleware.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, err
	}
	return claims, nil
}

// SetupRouter creates and configures the Gin router with all routes.
func SetupRouter(deps *Deps) *gin.Engine {
	r := gin.Default()

	// Trust loopback + RFC1918 ranges. The server binds to 127.0.0.1 on the
	// host (see docker-compose.yaml) and only to the container interface
	// inside Docker, so the only callers reaching it from a private address
	// are the host-side reverse proxy (often arriving as the docker bridge
	// gateway, e.g. 172.22.0.1) or sibling containers — both trusted.
	// Public callers can't reach the listening port without traversing the
	// proxy, so X-Forwarded-For stays unspoofable.
	_ = r.SetTrustedProxies([]string{
		"127.0.0.1", "::1",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	})

	// CORS middleware with origin whitelist
	r.Use(corsMiddleware(deps.Config.Server.AllowedOrigins))

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
	protected.DELETE("/conversations/:id/messages/:messageId", DeleteMessage(deps))

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
	protected.GET("/settings/webfetch", GetWebFetchSettings(deps))
	protected.PUT("/settings/webfetch", PutWebFetchSettings(deps))
	protected.POST("/settings/webfetch/verify", VerifyWebFetchSettings(deps))

	// Audit Logs
	protected.GET("/audit-logs", ListAuditLogs(deps))

	// Node command history
	protected.GET("/nodes/:id/commands", ListNodeCommands(deps))

	// LLM verify + cached models
	protected.POST("/settings/llm/verify", VerifyLLMSettings(deps))
	protected.GET("/settings/llm/models", GetLLMModels(deps))

	// API Keys management
	protected.POST("/api-keys", CreateAPIKey(deps))
	protected.GET("/api-keys", ListAPIKeys(deps))
	protected.DELETE("/api-keys/:id", DeleteAPIKey(deps))

	// External API (API Key auth)
	v1 := r.Group("/api/v1")
	v1.Use(middleware.APIKeyAuth())
	v1.GET("/nodes", ExternalListNodes(deps))
	v1.GET("/nodes/:id", ExternalGetNode(deps))
	v1.POST("/nodes/:id/execute", ExternalExecuteCommand(deps))

	// MCP endpoint for Claude Code / other MCP clients. Streamable HTTP
	// transport over JSON-RPC 2.0; reuses the same API-key auth as /api/v1.
	mcpGroup := r.Group("/mcp")
	mcpGroup.Use(middleware.APIKeyAuth())
	mcpHandler := mcp.Handler(deps.NodeManager, deps.Settings)
	mcpGroup.POST("", mcpHandler)
	mcpGroup.GET("", mcpHandler) // returns 405 with a useful body

	// Agent install script: 302 → GitHub raw (configurable via server.install_script_url).
	// curl -fsSL follows the redirect transparently.
	r.GET("/install.sh", func(c *gin.Context) {
		url := deps.Config.Server.InstallScriptURL
		if url == "" {
			c.String(http.StatusNotFound, "install script url not configured")
			return
		}
		c.Redirect(http.StatusFound, url)
	})

	// Agent binary mirror: streams GitHub release assets through this server so
	// agents in regions that can't reach github.com can still install.
	r.GET("/releases/*path", ReleaseProxy(deps))

	// WebSocket: Agent connection (token-based auth, not JWT)
	r.GET("/ws/agent", AgentWSHandler(deps))

	// WebSocket: Frontend chat connection (JWT via query param)
	r.GET("/ws/chat", ChatWSHandler(deps))

	// WebSocket: Frontend interactive terminal (JWT via first-message auth)
	r.GET("/ws/terminal", TerminalWSHandler(deps))

	// Embedded SPA — falls through as NoRoute so API/WS paths aren't shadowed.
	if err := webui.Register(r); err != nil {
		panic(err)
	}

	return r
}

func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			allowed := false

			// Check wildcard
			if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
				allowed = true
			} else {
				// Same-origin: always allowed
				if strings.Contains(origin, c.Request.Host) {
					allowed = true
				}
				// Check whitelist
				for _, o := range allowedOrigins {
					if strings.TrimRight(o, "/") == strings.TrimRight(origin, "/") {
						allowed = true
						break
					}
				}
			}

			if allowed {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
