package handler

import "github.com/momaek/tolato/server/internal/middleware"

// InitUpgraders configures the WebSocket upgraders with origin checking
// based on the allowed origins from config.
func InitUpgraders(allowedOrigins []string) {
	checkFn := middleware.CheckOrigin(allowedOrigins)
	chatUpgrader.CheckOrigin = checkFn
	agentUpgrader.CheckOrigin = checkFn
}
