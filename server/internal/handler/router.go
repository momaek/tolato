package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/auth"
	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/geoip"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/notify"
	"github.com/momaek/tolato/server/internal/settings"
	"github.com/momaek/tolato/server/internal/store"
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
	GeoIP       *geoip.Service     // may be nil when geoip is disabled
	Notifier    *notify.Dispatcher // node offline/online notifications
	OIDC        *auth.OIDCManager  // caches the discovered SSO provider
	Version     string             // server build version (e.g. "v0.8.10"), "dev" in local builds
}

// errAuth is returned for every WebSocket authentication failure. The specific
// cause is logged server-side but never told to the client.
var errAuth = errors.New("invalid or expired token")

// AuthenticateToken validates a JWT carried in a WebSocket handshake and
// returns the live user behind it.
//
// The WebSocket handlers can't run through JWTAuth (the token arrives in the
// first frame, not a header), so this is the equivalent gate: signature, then a
// database lookup that re-checks the account still exists and is active. A
// long-lived socket is only authorized at connect time, so a user disabled
// mid-session keeps their current socket until it drops — new ones are refused.
func (d *Deps) AuthenticateToken(tokenString string) (*model.User, error) {
	claims, err := middleware.ParseTokenString(tokenString)
	if err != nil {
		return nil, errAuth
	}
	if claims.UserID == "" {
		return nil, errAuth
	}
	user, err := store.GetUserByID(claims.UserID)
	if err != nil {
		return nil, errAuth
	}
	if user.Status != model.UserStatusActive {
		return nil, errAuth
	}
	return user, nil
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

	// Auth (no JWT required). The SSO endpoints are necessarily public: they
	// are what a not-yet-signed-in browser walks through.
	api.POST("/auth/login", LoginHandler(deps))
	api.GET("/auth/oidc/status", OIDCStatus(deps))
	api.GET("/auth/oidc/login", OIDCLogin(deps))
	api.GET("/auth/oidc/callback", OIDCCallback(deps))

	// JWT-protected routes
	protected := api.Group("")
	protected.Use(middleware.JWTAuth())

	// Admin-only routes. Everything that configures the instance lives here;
	// members get the chat, the nodes they've been granted, and their own keys.
	admin := api.Group("")
	admin.Use(middleware.JWTAuth(), middleware.RequireAdmin())

	// Session / self-service
	protected.GET("/auth/me", CurrentUser(deps))
	protected.PUT("/auth/password", ChangeOwnPassword(deps))

	// User groups, node groups and grants — the permission model's admin surface
	admin.GET("/user-groups", ListUserGroups(deps))
	admin.POST("/user-groups", CreateUserGroup(deps))
	admin.PUT("/user-groups/:id", UpdateUserGroup(deps))
	admin.DELETE("/user-groups/:id", DeleteUserGroup(deps))
	admin.GET("/node-groups", ListNodeGroups(deps))
	admin.POST("/node-groups", CreateNodeGroup(deps))
	admin.PUT("/node-groups/:id", UpdateNodeGroup(deps))
	admin.DELETE("/node-groups/:id", DeleteNodeGroup(deps))
	admin.GET("/grants", ListGrants(deps))
	admin.POST("/grants", CreateGrant(deps))
	admin.DELETE("/grants/:id", DeleteGrant(deps))

	// User management
	admin.GET("/users", ListUsers(deps))
	admin.POST("/users", CreateUser(deps))
	admin.PUT("/users/:id", UpdateUser(deps))
	admin.DELETE("/users/:id", DeleteUser(deps))

	// The grant list resolved into effective access, from either direction.
	admin.GET("/users/:id/access", UserAccess(deps))
	admin.GET("/nodes/:id/access", NodeAccess(deps))

	// Conversations
	protected.POST("/conversations", CreateConversation(deps))
	protected.GET("/conversations", ListConversations(deps))
	protected.GET("/conversations/:id", GetConversation(deps))
	protected.PUT("/conversations/:id", UpdateConversation(deps))
	protected.DELETE("/conversations/:id", DeleteConversation(deps))
	protected.DELETE("/conversations/:id/messages/:messageId", DeleteMessage(deps))
	protected.GET("/conversations/:id/tool-calls/:toolCallId/output", GetToolCallOutput(deps))

	// Nodes
	admin.POST("/nodes", CreateNode(deps))
	protected.GET("/nodes", ListNodes(deps))
	protected.GET("/nodes/:id", GetNode(deps))
	protected.PUT("/nodes/:id", UpdateNode(deps))
	protected.POST("/nodes/:id/update", UpdateNodeAgent(deps))
	protected.DELETE("/nodes/:id", DeleteNode(deps))

	// Settings
	admin.GET("/settings/llm", GetLLMSettings(deps))
	admin.PUT("/settings/llm", PutLLMSettings(deps))
	admin.GET("/settings/security", GetSecuritySettings(deps))
	admin.PUT("/settings/security", PutSecuritySettings(deps))
	admin.GET("/settings/agent", GetAgentSettings(deps))
	admin.PUT("/settings/agent", PutAgentSettings(deps))
	admin.GET("/settings/chat", GetChatSettings(deps))
	admin.PUT("/settings/chat", PutChatSettings(deps))
	admin.GET("/settings/webfetch", GetWebFetchSettings(deps))
	admin.PUT("/settings/webfetch", PutWebFetchSettings(deps))
	admin.POST("/settings/webfetch/verify", VerifyWebFetchSettings(deps))
	admin.GET("/settings/notify", GetNotifySettings(deps))
	admin.PUT("/settings/notify", PutNotifySettings(deps))
	admin.GET("/settings/notify/presets", GetNotifyPresets(deps))
	admin.POST("/settings/notify/test", TestNotifyChannel(deps))
	admin.GET("/settings/oidc", GetOIDCSettings(deps))
	admin.PUT("/settings/oidc", PutOIDCSettings(deps))
	admin.POST("/settings/oidc/verify", VerifyOIDCSettings(deps))

	// Audit Logs
	protected.GET("/audit-logs", ListAuditLogs(deps))

	// Node command history
	protected.GET("/nodes/:id/commands", ListNodeCommands(deps))

	// LLM verify + cached models
	admin.POST("/settings/llm/verify", VerifyLLMSettings(deps))
	admin.GET("/settings/llm/models", GetLLMModels(deps))

	// API Keys management
	protected.POST("/api-keys", CreateAPIKey(deps))
	protected.GET("/api-keys", ListAPIKeys(deps))
	protected.DELETE("/api-keys/:id", DeleteAPIKey(deps))

	// Version / update check
	protected.GET("/version", VersionInfo(deps))

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
				// Same-origin: compare the origin's host against the request
				// Host exactly. A substring match here would let
				// `https://<host>.evil.com` pass, so parse and compare hosts.
				if u, err := url.Parse(origin); err == nil && strings.EqualFold(u.Host, c.Request.Host) {
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
