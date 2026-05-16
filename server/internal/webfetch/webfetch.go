// Package webfetch fetches a remote URL through a configured upstream (Jina
// Reader today) and returns its readable Markdown rendering. The package is
// settings-driven and has no transport-specific dependencies, so it is shared
// by the agent tool executor and the MCP handler.
package webfetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/settings"
)

// jinaReaderBase is the public Jina Reader endpoint. Reader takes a target URL
// appended to its path and returns LLM-friendly Markdown.
//
// Anonymous calls are heavily rate-limited; we always require a key here so the
// behavior is predictable and the user gets a clear error when it's missing.
const jinaReaderBase = "https://r.jina.ai/"

// Result is the structured outcome of a fetch. Callers serialize it into their
// transport-specific result type (ToolResultItem for the LLM loop, JSON for MCP).
type Result struct {
	URL         string `json:"url"`
	Source      string `json:"source"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
	Bytes       int    `json:"bytes"`
	Truncated   bool   `json:"truncated"`
}

// Fetch resolves target through the upstream configured in settings. Returns a
// non-nil error on any failure that callers should surface to the user (bad
// URL, missing key, upstream rejected the request, etc.).
func Fetch(ctx context.Context, sc *settings.Cache, target string) (*Result, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("url is required")
	}
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("url must be an absolute http(s) URL")
	}

	cfg := sc.WebFetch()
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "", "jina":
		return fetchViaJina(ctx, parsed.String(), cfg)
	case "local":
		return nil, errors.New("local mode is not implemented yet — switch to Jina mode under Settings → Web Fetch")
	default:
		return nil, fmt.Errorf("unknown web_fetch mode %q", cfg.Mode)
	}
}

func fetchViaJina(ctx context.Context, target string, cfg model.WebFetchSettings) (*Result, error) {
	if strings.TrimSpace(cfg.JinaAPIKey) == "" {
		return nil, errors.New("Jina API key is missing — configure it under Settings → Web Fetch")
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
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.JinaAPIKey)
	req.Header.Set("Accept", "text/markdown, text/plain;q=0.8, */*;q=0.5")
	req.Header.Set("X-Return-Format", "markdown")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Jina request failed: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	truncated := int64(len(body)) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("Jina rejected the API key (status %d)", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("Jina returned status %d: %s", resp.StatusCode, strings.TrimSpace(snippet))
	}
	if readErr != nil && !truncated {
		return nil, fmt.Errorf("read body failed: %w", readErr)
	}

	return &Result{
		URL:         target,
		Source:      "jina",
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Content:     string(body),
		Bytes:       len(body),
		Truncated:   truncated,
	}, nil
}
