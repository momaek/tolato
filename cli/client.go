package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Node mirrors the fields /api/v1/nodes returns that the CLI displays.
type Node struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Alias         *string  `json:"alias,omitempty"`
	IP            string   `json:"ip"`
	Status        string   `json:"status"`
	OS            string   `json:"os"`
	Kernel        string   `json:"kernel,omitempty"`
	AgentVersion  string   `json:"agent_version,omitempty"`
	CPUCores      int      `json:"cpu_cores,omitempty"`
	MemoryTotalMB int      `json:"memory_total_mb,omitempty"`
	DiskTotalGB   int      `json:"disk_total_gb,omitempty"`
	CPU           *float64 `json:"cpu,omitempty"`
	Memory        *float64 `json:"memory,omitempty"`
	Disk          *float64 `json:"disk,omitempty"`
}

// DisplayName is the alias when set, else the reported hostname.
func (n Node) DisplayName() string {
	if n.Alias != nil && *n.Alias != "" {
		return *n.Alias
	}
	return n.Name
}

// ExecResult is the outcome of a command run on a node.
type ExecResult struct {
	NodeID     string `json:"node_id"`
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
}

type client struct {
	cfg  Config
	http *http.Client
}

func newClient(cfg Config) *client {
	return &client{cfg: cfg, http: &http.Client{Timeout: 5 * time.Minute}}
}

// apiError carries a server-reported failure.
type apiError struct {
	Status  int
	Code    string `json:"error"`
	Message string `json:"message"`
}

func (e *apiError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("server returned %d", e.Status)
}

// IsSensitive reports whether the server refused because the command needs
// explicit confirmation.
func (e *apiError) IsSensitive() bool { return e.Code == "sensitive_operation" }

func (c *client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.URL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	// The server tags audit rows by this, so CLI activity is distinguishable
	// from other API clients in the audit view.
	req.Header.Set("User-Agent", "tolato-cli/"+version)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		apiErr := &apiError{Status: resp.StatusCode}
		_ = json.Unmarshal(raw, apiErr)
		if apiErr.Message == "" && resp.StatusCode == http.StatusUnauthorized {
			apiErr.Message = "unauthorized — check TOLATO_API_KEY"
		}
		return apiErr
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// RevokeOwnKey revokes the key this client is authenticating with. Used by
// `tolato auth logout`, which holds a key rather than a session and so cannot
// reach the browser-facing key management endpoints.
func (c *client) RevokeOwnKey(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/auth/key", nil, nil)
}

func (c *client) ListNodes(ctx context.Context) ([]Node, error) {
	var nodes []Node
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes", nil, &nodes)
	return nodes, err
}

func (c *client) GetNode(ctx context.Context, id string) (*Node, error) {
	var n Node
	if err := c.do(ctx, http.MethodGet, "/api/v1/nodes/"+id, nil, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

func (c *client) Exec(ctx context.Context, nodeID, command string, timeout int, confirm bool) (*ExecResult, error) {
	body := map[string]any{"command": command, "timeout": timeout, "confirm": confirm}
	var res ExecResult
	if err := c.do(ctx, http.MethodPost, "/api/v1/nodes/"+nodeID+"/execute", body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// resolveNode turns a user-supplied id, alias or hostname into a node id.
//
// An exact id match wins without a lookup. Otherwise the visible nodes are
// searched case-insensitively, and an ambiguous name is an error rather than a
// guess — running a command on the wrong machine is not a recoverable mistake.
func (c *client) resolveNode(ctx context.Context, ref string) (string, error) {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return "", err
	}

	var matches []Node
	for _, n := range nodes {
		if n.ID == ref {
			return n.ID, nil
		}
		if strings.EqualFold(n.DisplayName(), ref) || strings.EqualFold(n.Name, ref) {
			matches = append(matches, n)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no node matches %q", ref)
	case 1:
		return matches[0].ID, nil
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, fmt.Sprintf("%s (%s)", m.DisplayName(), m.ID))
		}
		return "", fmt.Errorf("%q is ambiguous: %s — use the id", ref, strings.Join(names, ", "))
	}
}
