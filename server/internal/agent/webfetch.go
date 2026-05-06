package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/momaek/tolato/server/internal/model"
)

// jinaReaderBase is the public Jina Reader endpoint. Reader takes a target URL
// appended to its path and returns LLM-friendly Markdown.
//
// Anonymous calls are heavily rate-limited; we always require a key here so the
// behavior is predictable and the user gets a clear error when it's missing.
const jinaReaderBase = "https://r.jina.ai/"

func (te *ToolExecutor) executeWebFetch(ctx context.Context, target string) *model.ToolResultItem {
	target = strings.TrimSpace(target)
	if target == "" {
		return webFetchError("url is required")
	}
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return webFetchError("url must be an absolute http(s) URL")
	}

	cfg := te.settings.WebFetch()

	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "", "jina":
		return te.fetchViaJina(ctx, parsed.String(), cfg)
	case "local":
		return webFetchError("local mode is not implemented yet — switch to Jina mode under Settings → Web Fetch")
	default:
		return webFetchError(fmt.Sprintf("unknown web_fetch mode %q", cfg.Mode))
	}
}

func (te *ToolExecutor) fetchViaJina(ctx context.Context, target string, cfg model.WebFetchSettings) *model.ToolResultItem {
	if strings.TrimSpace(cfg.JinaAPIKey) == "" {
		return webFetchError("Jina API key is missing — configure it under Settings → Web Fetch")
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxBytes := int64(cfg.MaxKB) * 1024
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, jinaReaderBase+target, nil)
	if err != nil {
		return webFetchError("invalid request: " + err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+cfg.JinaAPIKey)
	req.Header.Set("Accept", "text/markdown, text/plain;q=0.8, */*;q=0.5")
	req.Header.Set("X-Return-Format", "markdown")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return webFetchError("Jina request failed: " + err.Error())
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	truncated := int64(len(body)) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return webFetchError(fmt.Sprintf("Jina rejected the API key (status %d)", resp.StatusCode))
	}
	if resp.StatusCode >= 400 {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return webFetchError(fmt.Sprintf("Jina returned status %d: %s", resp.StatusCode, strings.TrimSpace(snippet)))
	}
	if readErr != nil && !truncated {
		return webFetchError("read body failed: " + readErr.Error())
	}

	return &model.ToolResultItem{
		Data: map[string]any{
			"url":          target,
			"source":       "jina",
			"status":       resp.StatusCode,
			"content_type": resp.Header.Get("Content-Type"),
			"content":      string(body),
			"bytes":        len(body),
			"truncated":    truncated,
		},
	}
}

func webFetchError(msg string) *model.ToolResultItem {
	return &model.ToolResultItem{Data: map[string]any{"error": msg}}
}
