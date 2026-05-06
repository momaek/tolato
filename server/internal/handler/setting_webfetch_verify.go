package handler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// VerifyWebFetchSettings probes the configured upstream (Jina Reader, for now)
// using the supplied API key — or the stored one if the client omitted it
// because it's still showing the masked value.
//
// We fetch a tiny known URL through Jina and consider the verify successful if
// we get a 2xx response. Any non-2xx (including 401/403 for a bad key) is
// surfaced verbatim so the user can tell the difference between "wrong key"
// and "Jina is down."
func VerifyWebFetchSettings(deps *Deps) gin.HandlerFunc {
	type verifyReq struct {
		Mode       string `json:"mode"`
		JinaAPIKey string `json:"jina_api_key"`
	}
	type verifyResp struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
		Sample  string `json:"sample,omitempty"` // first ~120 chars of fetched content
	}

	return func(c *gin.Context) {
		var req verifyReq
		_ = c.ShouldBindJSON(&req)

		stored := deps.Settings.WebFetch()
		mode := strings.ToLower(strings.TrimSpace(req.Mode))
		if mode == "" {
			mode = stored.Mode
		}
		if mode == "" {
			mode = "jina"
		}
		if mode != "jina" {
			c.JSON(http.StatusOK, verifyResp{Success: false, Error: "verify is only available for Jina mode"})
			return
		}

		apiKey := strings.TrimSpace(req.JinaAPIKey)
		// Empty or still-masked → fall back to stored key.
		if apiKey == "" || strings.Contains(apiKey, "****") {
			apiKey = stored.JinaAPIKey
		}
		if apiKey == "" {
			c.JSON(http.StatusOK, verifyResp{Success: false, Error: "Jina API key is missing"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		probeURL := "https://r.jina.ai/https://example.com"
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
		if err != nil {
			c.JSON(http.StatusOK, verifyResp{Success: false, Error: err.Error()})
			return
		}
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		httpReq.Header.Set("Accept", "text/markdown")
		httpReq.Header.Set("X-Return-Format", "markdown")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			c.JSON(http.StatusOK, verifyResp{Success: false, Error: "request failed: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		// Cap the body at 4KB just for the sample preview.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			c.JSON(http.StatusOK, verifyResp{Success: false, Error: "API key was rejected by Jina"})
			return
		}
		if resp.StatusCode >= 400 {
			c.JSON(http.StatusOK, verifyResp{Success: false, Error: "Jina returned status " + resp.Status})
			return
		}

		sample := strings.TrimSpace(string(body))
		if len(sample) > 120 {
			sample = sample[:120] + "…"
		}
		c.JSON(http.StatusOK, verifyResp{Success: true, Sample: sample})
	}
}
