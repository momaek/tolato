package handler

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ReleaseProxy reverse-proxies GET /releases/<path> to <upstream>/<path>,
// streaming the response body back to the client.
//
// GitHub release downloads issue a 302 to objects.githubusercontent.com, which
// is also blocked in the same regions where github.com is blocked — so we
// can't just send the redirect to the client. We let Go's default http.Client
// follow redirects (up to 10) and copy the final body through, turning this
// server into a transparent mirror.
func ReleaseProxy(deps *Deps) gin.HandlerFunc {
	upstream := strings.TrimRight(deps.Config.Server.ReleaseProxyUpstream, "/")
	client := &http.Client{Timeout: 10 * time.Minute}
	return func(c *gin.Context) {
		if upstream == "" {
			c.String(http.StatusNotFound, "release proxy not configured")
			return
		}
		// gin's *path catch-all keeps the leading slash.
		sub := c.Param("path")
		if sub == "" || sub == "/" {
			c.String(http.StatusNotFound, "missing release path")
			return
		}
		target := upstream + sub

		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
		if err != nil {
			c.String(http.StatusBadGateway, "failed to build upstream request: %v", err)
			return
		}
		req.Header.Set("User-Agent", "tolato-release-proxy")

		resp, err := client.Do(req)
		if err != nil {
			c.String(http.StatusBadGateway, "upstream fetch failed: %v", err)
			return
		}
		defer resp.Body.Close()

		for _, h := range []string{"Content-Type", "Content-Length", "Last-Modified", "ETag"} {
			if v := resp.Header.Get(h); v != "" {
				c.Header(h, v)
			}
		}
		c.Status(resp.StatusCode)
		_, _ = io.Copy(c.Writer, resp.Body)
	}
}
