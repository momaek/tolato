package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/settings"
)

// server holds the collaborators every tool handler needs. Constructed once
// and reused per request — no per-request mutable state lives here.
type server struct {
	nodes    *node.NodeManager
	settings *settings.Cache
}

// Handler returns a Gin handler that serves the MCP protocol on a single POST
// endpoint. Auth is the caller's responsibility — mount middleware.APIKeyAuth
// (or equivalent) before this so c.Get("api_key_*") is populated.
func Handler(nm *node.NodeManager, sc *settings.Cache) gin.HandlerFunc {
	s := &server{nodes: nm, settings: sc}
	return s.handle
}

func (s *server) handle(c *gin.Context) {
	// MCP spec allows GET for server→client SSE streaming. We don't push any
	// server-initiated events, so a 405 with a clear body is more useful than
	// pretending to support it.
	if c.Request.Method != http.MethodPost {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"error": "this MCP endpoint only accepts POST",
		})
		return
	}

	caller := callerContext{
		APIKeyID:   c.GetString("api_key_id"),
		Permission: c.GetString("api_key_permission"),
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeError(c, nil, -32700, "failed to read request body: "+err.Error())
		return
	}

	// A batch is a JSON array; a single request is an object. We support both
	// per the JSON-RPC 2.0 spec, though MCP clients rarely send batches.
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		s.handleBatch(c, caller, trimmed)
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(trimmed, &req); err != nil {
		writeError(c, nil, -32700, "parse error: "+err.Error())
		return
	}
	resp := s.handleOne(c, caller, req)
	if resp == nil {
		// Notification: spec says respond with 202 Accepted and no body.
		c.Status(http.StatusAccepted)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *server) handleBatch(c *gin.Context, caller callerContext, raw []byte) {
	var batch []rpcRequest
	if err := json.Unmarshal(raw, &batch); err != nil {
		writeError(c, nil, -32700, "parse error: "+err.Error())
		return
	}
	out := make([]rpcResponse, 0, len(batch))
	for _, req := range batch {
		if resp := s.handleOne(c, caller, req); resp != nil {
			out = append(out, *resp)
		}
	}
	if len(out) == 0 {
		c.Status(http.StatusAccepted)
		return
	}
	c.JSON(http.StatusOK, out)
}

// handleOne dispatches a single JSON-RPC request. Returns nil for
// notifications (no id) — caller turns that into 202 Accepted.
func (s *server) handleOne(c *gin.Context, caller callerContext, req rpcRequest) *rpcResponse {
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: initializeResult{
				ProtocolVersion: protocolVersion,
				Capabilities: serverCapabilities{
					Tools: &toolsCapability{ListChanged: false},
				},
				ServerInfo: serverInfo{Name: serverName, Version: serverVersion},
			},
		}

	case "notifications/initialized", "notifications/cancelled", "notifications/progress":
		// Fire-and-forget client → server signals. Nothing to do.
		return nil

	case "ping":
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}

	case "tools/list":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  toolsListResult{Tools: catalog()},
		}

	case "tools/call":
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return rpcErr(req.ID, -32602, "invalid params: "+err.Error())
		}
		payload, isErr, dispatchErr := s.dispatch(c.Request.Context(), caller, params)
		if dispatchErr != nil {
			return rpcErr(req.ID, -32602, dispatchErr.Error())
		}
		text, _ := json.Marshal(payload)
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: toolCallResult{
				Content: []contentBlock{{Type: "text", Text: string(text)}},
				IsError: isErr,
			},
		}

	default:
		if isNotification {
			return nil
		}
		return rpcErr(req.ID, -32601, "method not found: "+req.Method)
	}
}

func rpcErr(id json.RawMessage, code int, msg string) *rpcResponse {
	return &rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	}
}

func writeError(c *gin.Context, id json.RawMessage, code int, msg string) {
	c.JSON(http.StatusOK, rpcErr(id, code, msg))
}
