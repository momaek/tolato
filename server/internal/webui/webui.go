// Package webui serves the Vue SPA bundle embedded into the server binary.
//
// The Docker build drops `web/dist` into this package's `dist/` directory
// before `go build` runs, so //go:embed captures the real assets. Local
// builds see only the placeholder and will serve 404s for UI paths, which
// is fine because local dev uses `pnpm dev` directly.
package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"embed"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var distFS embed.FS

// Register mounts the embedded SPA as a NoRoute fallback on r. Requests to
// `/api/*`, `/ws/*`, and `/install.sh` keep their natural 404 so unknown API
// calls don't silently return HTML.
func Register(r *gin.Engine) error {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return err
	}

	fileServer := http.FileServer(http.FS(sub))

	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/ws/") || p == "/install.sh" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		requested := strings.TrimPrefix(p, "/")
		if requested == "" {
			requested = "index.html"
		}
		// Vue Router history mode: rewrite unknown paths to index.html so the
		// client-side router can resolve them.
		if _, err := fs.Stat(sub, path.Clean(requested)); err != nil {
			c.Request.URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	return nil
}
