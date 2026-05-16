// Package mcp implements a minimal Streamable-HTTP Model Context Protocol
// server. It speaks JSON-RPC 2.0 over a single POST endpoint and reuses the
// existing API-key middleware for auth. Stateless: no Mcp-Session-Id is issued.
//
// Supported methods: initialize, notifications/initialized, tools/list,
// tools/call, ping. Everything else returns JSON-RPC error -32601.
package mcp

import "encoding/json"

const (
	// protocolVersion is the MCP revision we advertise. We accept whatever the
	// client sends in `initialize.params.protocolVersion` — but echo this one
	// back so clients without strict negotiation behave.
	protocolVersion = "2025-06-18"
	serverName      = "tolato"
	serverVersion   = "0.1.0"
)

// JSON-RPC 2.0 envelope. ID is a raw message so we can echo back whatever the
// client sent (string, number, or null) without converting.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// initializeResult is what the server returns to the client's `initialize`
// request. capabilities.tools.listChanged is false because the catalog is
// static for the lifetime of the process.
type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapabilities `json:"capabilities"`
	ServerInfo      serverInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	Tools *toolsCapability `json:"tools,omitempty"`
}

type toolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// tool is the public catalog entry returned by tools/list.
type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []tool `json:"tools"`
}

// toolCallResult is what a single tools/call returns. We always return one
// content block of type "text" carrying JSON — Claude Code parses it natively.
type toolCallResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
