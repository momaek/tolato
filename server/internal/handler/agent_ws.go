package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/store"
)

// agentUpgrader is initialized by InitUpgraders with origin checking.
var agentUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Agent connections have no Origin header (non-browser), allow by default.
		// This will be overridden by InitUpgraders.
		return r.Header.Get("Origin") == ""
	},
}

// AgentWSHandler handles /ws/agent WebSocket connections from node agents.
//
// Two connection modes:
//   - First time:  ?token=xxx           → validate registration token, wait for register message to create Node
//   - Reconnect:   ?node_id=xxx&secret=xxx  → validate existing node identity
func AgentWSHandler(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("token")
		nodeID := c.Query("node_id")
		secret := c.Query("secret")

		// Determine connection mode
		var (
			regToken    *model.RegistrationToken // non-nil = first-time registration
			existingNode *model.Node              // non-nil = reconnection
		)

		if nodeID != "" && secret != "" {
			// Reconnection mode: validate node_id + secret
			node, err := store.GetNodeBySecret(nodeID, secret)
			if err != nil {
				c.JSON(http.StatusUnauthorized, model.ErrorResponse{
					Error:   "unauthorized",
					Message: "invalid node_id or secret",
				})
				return
			}
			existingNode = node
		} else if token != "" {
			// First-time registration mode: validate registration token
			rt, err := store.GetRegistrationToken(token)
			if err != nil {
				c.JSON(http.StatusUnauthorized, model.ErrorResponse{
					Error:   "unauthorized",
					Message: "invalid or expired registration token",
				})
				return
			}
			regToken = rt
		} else {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "provide either ?token=xxx or ?node_id=xxx&secret=xxx",
			})
			return
		}

		// Upgrade to WebSocket
		conn, err := agentUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// If reconnecting, send ack immediately and register connection
		if existingNode != nil {
			ack := model.WSMessage{
				Type: "register_ack",
				Payload: model.AgentAuthResponse{
					NodeID: existingNode.ID,
					Secret: existingNode.AgentSecret,
				},
			}
			if err := conn.WriteJSON(ack); err != nil {
				log.Printf("Failed to send register_ack: %v", err)
				return
			}

			ac := deps.NodeManager.RegisterConn(existingNode.ID, conn)
			installSystemHandlers(deps, existingNode.ID, ac)
			defer func() {
				deps.NodeManager.RemoveConn(existingNode.ID)
				_ = store.SetNodeStatus(existingNode.ID, "offline")
				log.Printf("Agent disconnected (reconnect): node=%s", existingNode.ID)
			}()

			log.Printf("Agent reconnected: node=%s", existingNode.ID)
			_ = store.UpdateHeartbeat(existingNode.ID)

			// Block until the router goroutine finishes.
			<-ac.Done()
			return
		}

		// First-time registration: wait for the register message to create the Node
		log.Printf("Agent connected with token, waiting for register message...")

		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Agent disconnected before register: %v", err)
				return
			}

			var msg node.AgentFrame
			if err := json.Unmarshal(raw, &msg); err != nil {
				log.Printf("Failed to parse agent message: %v", err)
				continue
			}

			if msg.Type != model.AgentTypeRegister {
				log.Printf("Expected register message, got: %s", msg.Type)
				continue
			}

			var reg model.AgentRegisterPayload
			if err := msg.Decode(&reg); err != nil {
				log.Printf("Failed to parse register payload: %v", err)
				continue
			}

			// Create node in database
			agentSecret := uuid.New().String()
			node, err := store.CreateNodeFromRegistration(reg, regToken.AliasPrefix, agentSecret)
			if err != nil {
				log.Printf("Failed to create node: %v", err)
				errMsg := model.WSMessage{Type: "error", Payload: map[string]string{"message": "registration failed"}}
				_ = conn.WriteJSON(errMsg)
				return
			}

			// Send register_ack with node_id + secret
			ack := model.WSMessage{
				Type: "register_ack",
				Payload: model.AgentAuthResponse{
					NodeID: node.ID,
					Secret: agentSecret,
				},
			}
			if err := conn.WriteJSON(ack); err != nil {
				log.Printf("Failed to send register_ack: %v", err)
				return
			}

			// Register connection
			ac := deps.NodeManager.RegisterConn(node.ID, conn)
			installSystemHandlers(deps, node.ID, ac)
			defer func() {
				deps.NodeManager.RemoveConn(node.ID)
				_ = store.SetNodeStatus(node.ID, "offline")
				log.Printf("Agent disconnected (new): node=%s", node.ID)
			}()

			log.Printf("Agent registered: node=%s hostname=%s os=%s ip=%s", node.ID, reg.Hostname, reg.OS, reg.IP)

			// Block until the router goroutine finishes.
			<-ac.Done()
			return
		}
	}
}

// installSystemHandlers wires heartbeat / re-register callbacks on the AgentConn
// router. All message reading is owned by the router goroutine started by
// NodeManager.RegisterConn.
func installSystemHandlers(deps *Deps, nodeID string, ac *node.AgentConn) {
	ac.SetSystemHandlers(node.SystemHandlers{
		OnHeartbeat: func(payload json.RawMessage) {
			handleAgentHeartbeat(deps, nodeID, payload)
		},
		OnReRegister: func(payload json.RawMessage) {
			handleAgentReRegister(nodeID, payload)
		},
	})
}

func handleAgentReRegister(nodeID string, payload json.RawMessage) {
	var reg model.AgentRegisterPayload
	if err := json.Unmarshal(payload, &reg); err != nil {
		return
	}
	// Update node info (hostname, os, etc. may have changed)
	_ = store.UpdateNode(nodeID, map[string]any{
		"name":            reg.Hostname,
		"os":              reg.OS,
		"kernel":          reg.Kernel,
		"ip":              reg.IP,
		"agent_version":   reg.AgentVersion,
		"cpu_cores":       reg.CPUCores,
		"memory_total_mb": reg.MemoryTotalMB,
		"disk_total_gb":   reg.DiskTotalGB,
		"status":          "online",
	})
}

func handleAgentHeartbeat(deps *Deps, nodeID string, payload json.RawMessage) {
	var hb model.AgentHeartbeatPayload
	if err := json.Unmarshal(payload, &hb); err != nil {
		log.Printf("Failed to unmarshal heartbeat payload: %v", err)
		return
	}

	if err := store.UpdateHeartbeat(nodeID); err != nil {
		log.Printf("Failed to update heartbeat: %v", err)
	}

	deps.NodeManager.UpdateMetrics(nodeID, &model.NodeMetrics{
		CPU:     hb.CPU,
		Memory:  hb.Memory,
		Disk:    hb.Disk,
		Uptime:  hb.Uptime,
		LoadAvg: hb.LoadAvg,
	})
}

