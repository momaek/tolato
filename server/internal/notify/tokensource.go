package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/momaek/tolato/server/internal/model"
)

// tokenSource exchanges credentials for a bearer token and caches it until it
// expires. Safe for concurrent use; one in-flight fetch at a time per source.
type tokenSource struct {
	cfg    model.NotifyTokenSource
	client *http.Client

	mu    sync.Mutex
	token string
	exp   time.Time
}

func newTokenSource(cfg model.NotifyTokenSource, client *http.Client) *tokenSource {
	return &tokenSource{cfg: cfg, client: client}
}

// Get returns a cached token if still valid, otherwise fetches a fresh one.
func (t *tokenSource) Get(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && time.Now().Before(t.exp) {
		return t.token, nil
	}
	return t.fetchLocked(ctx)
}

// Invalidate drops the cached token so the next Get re-fetches. Called when a
// send hits the configured invalidate_on condition (server revoked the token
// before its advertised expiry).
func (t *tokenSource) Invalidate() {
	t.mu.Lock()
	t.token = ""
	t.exp = time.Time{}
	t.mu.Unlock()
}

// matchInvalidate reports whether a send response indicates the token is stale.
func (t *tokenSource) matchInvalidate(body []byte) bool {
	return matchJSON(body, t.cfg.InvalidateOn)
}

func (t *tokenSource) fetchLocked(ctx context.Context) (string, error) {
	vars := map[string]string{}
	for k, v := range t.cfg.Params {
		vars[k] = v
	}

	method := strings.ToUpper(t.cfg.Request.Method)
	if method == "" {
		method = http.MethodPost
	}
	url := renderPlain(t.cfg.Request.URL, vars)
	body := renderPlain(t.cfg.Request.Body, vars)

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("token request build: %w", err)
	}
	for k, v := range t.cfg.Request.Headers {
		req.Header.Set(k, renderPlain(v, vars))
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", fmt.Errorf("token response not JSON (status %d): %s", resp.StatusCode, trunc(raw))
	}

	tokVal, ok := getPath(data, t.cfg.Extract.TokenPath)
	tok, _ := tokVal.(string)
	if !ok || tok == "" {
		return "", fmt.Errorf("token not found at %q (status %d): %s", t.cfg.Extract.TokenPath, resp.StatusCode, trunc(raw))
	}

	ttl := t.cfg.Extract.TTLFallbackSeconds
	if ttl <= 0 {
		ttl = 3600
	}
	if t.cfg.Extract.ExpiresPath != "" {
		if v, ok := getPath(data, t.cfg.Extract.ExpiresPath); ok {
			if f, ok := v.(float64); ok && f > 0 {
				ttl = int(f)
			}
		}
	}
	// Refresh a minute early to avoid using a token that expires mid-flight.
	skew := 60
	if ttl <= skew {
		skew = ttl / 2
	}
	t.token = tok
	t.exp = time.Now().Add(time.Duration(ttl-skew) * time.Second)
	return tok, nil
}
