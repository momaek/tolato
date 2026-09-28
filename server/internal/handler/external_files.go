package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// externalFileOps are the agent file ops the v1 API forwards. They are the
// same ones the web terminal's file browser uses; nothing here reaches the
// agent that the browser could not already send it.
var externalFileOps = map[string]bool{
	"list": true, "stat": true, "read": true, "write": true, "mkdir": true, "delete": true,
}

// ExternalFileOp forwards one file op to a node's agent. It is what `tolato
// cp` is built on: the CLI splits a transfer into chunked reads or writes and
// sends them here one at a time.
//
// Reading or writing any file as the agent's user is as strong as running a
// command, so the gate is the same as ExternalExecuteCommand's: a writable key
// whose owner holds operator on the node. The command blacklist is not applied
// because there is no command to match it against.
func ExternalFileOp(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")
		apiKeyID := c.GetString("api_key_id")

		if c.GetString("api_key_permission") != model.APIKeyWritable {
			c.JSON(http.StatusForbidden, model.ErrorResponse{
				Error:   "forbidden",
				Message: "Read-only API keys cannot access node files",
			})
			return
		}
		if !requireNodeLevel(c, nodeID, model.LevelOperator) {
			return
		}

		var req model.AgentFileOpPayload
		if err := c.ShouldBindJSON(&req); err != nil {
			badRequest(c, err.Error())
			return
		}
		if !externalFileOps[req.Op] {
			badRequest(c, fmt.Sprintf("unknown op %q", req.Op))
			return
		}
		if req.Path == "" {
			badRequest(c, "path is required")
			return
		}

		n, err := store.GetNodeByID(nodeID)
		if err != nil {
			notFound(c, "node not found")
			return
		}
		ac, ok := deps.NodeManager.GetConn(nodeID)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: "node_offline", Message: "Node is offline"})
			return
		}

		// A transfer is many requests, one per chunk. Auditing only the first
		// (offset 0) keeps it to one row per file instead of hundreds, and
		// stat, which `tolato cp` sends around every transfer, only reads
		// metadata.
		audit := func(errMsg string) {
			if req.Op == "stat" || (req.Op == "read" || req.Op == "write") && req.Offset != 0 {
				return
			}
			entry := &model.AuditLog{
				NodeID:   nodeID,
				NodeName: n.Name,
				UserID:   middleware.CurrentUserID(c),
				Actor:    middleware.CurrentUsername(c),
				Command:  fmt.Sprintf("file:%s %s", req.Op, req.Path),
				Source:   externalSource(c),
				APIKeyID: &apiKeyID,
			}
			if errMsg != "" {
				entry.Stderr = &errMsg
			}
			_ = store.CreateAuditLog(entry)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		reply, err := ac.Request(ctx, model.AgentTypeFileOp, req, 30*time.Second)
		if err != nil {
			audit(err.Error())
			c.JSON(http.StatusBadGateway, model.ErrorResponse{Error: "agent_error", Message: err.Error()})
			return
		}

		var res model.AgentFileResultPayload
		if err := reply.Decode(&res); err != nil {
			audit(err.Error())
			c.JSON(http.StatusBadGateway, model.ErrorResponse{Error: "agent_error", Message: "malformed agent reply"})
			return
		}
		audit(res.Error)

		// A failed op (missing file, permission denied) is still a successful
		// round trip; the caller reads OK/Error the same way the browser does.
		c.JSON(http.StatusOK, res)
	}
}
