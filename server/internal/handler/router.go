package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/probe"
	"github.com/momaek/tolato/server/internal/settings"
	"github.com/momaek/tolato/server/internal/webui"
)

// Deps holds shared dependencies for all handlers.
//
// Handlers should reach for data through these injected collaborators rather
// than the package-level `store.*` globals where practical — it keeps the
// dependency graph explicit and the seam available for tests.
type Deps struct {
	Config         *config.Config
	NodeManager    *node.NodeManager
	SessionManager *SessionManager
	Settings       *settings.Cache
	// Probe is nil when Probe.Enabled=false in config. Handlers that need it
	// (pushProbeConfig, probe REST routes) are only wired up in that case, so
	// a nil-check isn't necessary at the call site.
	Probe *probe.Store
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

// SetupProbeRoutes registers NodeProbe API routes on the given engine.
func SetupProbeRoutes(r *gin.Engine, deps *Deps, probeStore *probe.Store, alertEngine *probe.AlertEngine) {
	probeAPI := r.Group("/api/v1/probe")

	// Agent report (authenticated via node_id:secret)
	probeAPI.POST("/report", middleware.AgentTokenAuth(), ProbeReportHandler(deps, probeStore, alertEngine))

	// Protected probe routes
	probeProtected := probeAPI.Group("")
	probeProtected.Use(middleware.JWTAuth())

	probeProtected.GET("/nodes", ProbeListNodes(deps))
	probeProtected.PUT("/nodes/:id", ProbeUpdateNodePosition(deps, probeStore))
	probeProtected.GET("/links", ProbeListLinks(deps, probeStore))
	probeProtected.POST("/links", ProbeCreateLink(deps, probeStore))
	probeProtected.DELETE("/links/:id", ProbeDeleteLink(deps, probeStore))
	probeProtected.GET("/links/:id/metrics", ProbeGetLinkMetrics(deps, probeStore))
	probeProtected.GET("/alerts", ProbeListAlerts(deps, probeStore))
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
