package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// ExternalService permits only metadata reads to readonly keys. Service changes
// can execute a downloaded binary, so require a writable key and operator grant.
func ExternalService(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		write := c.Request.Method != http.MethodGet
		level := model.LevelViewer
		if write {
			if c.GetString("api_key_permission") != model.APIKeyWritable {
				c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "forbidden", Message: "Service changes require a writable API key"})
				return
			}
			level = model.LevelOperator
		}
		id := c.Param("id")
		if !requireNodeLevel(c, id, level) {
			return
		}
		if c.Param("service") != "patato-node" {
			notFound(c, "unknown service")
			return
		}
		req := model.AgentServiceRequest{Service: "patato-node", Action: "status"}
		if write {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 73728)
			dec := json.NewDecoder(c.Request.Body)
			dec.DisallowUnknownFields()
			if dec.Decode(&req) != nil {
				badRequest(c, "invalid service request")
				return
			}
			req.Service = "patato-node"
			if req.Action != "install" && req.Action != "upgrade" && req.Action != "restart" && req.Action != "stop" {
				badRequest(c, "action must be install, upgrade, restart or stop")
				return
			}
		}
		ac, ok := deps.NodeManager.GetConn(id)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: "node_offline", Message: "Node is offline"})
			return
		}
		if ac.ServicesVersion() != 1 {
			c.JSON(http.StatusNotImplemented, model.ErrorResponse{Error: "service_unsupported", Message: "Upgrade the agent and enable --managed-services.enabled"})
			return
		}
		// Persist the attempt before forwarding it, including requests whose HTTP
		// caller disconnects mid-install. Never log config, URLs or tokens.
		if write {
			n, err := store.GetNodeByID(id)
			if err != nil {
				notFound(c, "node not found")
				return
			}
			key := c.GetString("api_key_id")
			entry := &model.AuditLog{NodeID: id, NodeName: n.Name, UserID: middleware.CurrentUserID(c), Actor: middleware.CurrentUsername(c), APIKeyID: &key, Source: externalSource(c), Command: fmt.Sprintf("service:%s patato-node", req.Action)}
			if err := store.CreateAuditLog(entry); err != nil {
				internalError(c, "failed to audit service change")
				return
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 150*time.Second)
		defer cancel()
		reply, err := ac.Request(ctx, "service", req, 150*time.Second)
		if err != nil {
			c.JSON(http.StatusBadGateway, model.ErrorResponse{Error: "agent_error", Message: "Service request failed"})
			return
		}
		var res model.AgentServiceResult
		if reply.Type != "service_result" || reply.Decode(&res) != nil {
			c.JSON(http.StatusBadGateway, model.ErrorResponse{Error: "agent_error", Message: "Malformed service reply"})
			return
		}
		if !res.OK {
			c.JSON(http.StatusBadGateway, model.ErrorResponse{Error: "service_failed", Message: res.Error})
			return
		}
		c.JSON(http.StatusOK, res.Status)
	}
}
