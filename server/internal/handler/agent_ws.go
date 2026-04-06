package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for agent connections
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
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
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

			deps.NodeManager.RegisterConn(existingNode.ID, conn)
			defer func() {
				deps.NodeManager.RemoveConn(existingNode.ID)
				_ = store.SetNodeStatus(existingNode.ID, "offline")
				log.Printf("Agent disconnected (reconnect): node=%s", existingNode.ID)
			}()

			log.Printf("Agent reconnected: node=%s", existingNode.ID)
			_ = store.UpdateHeartbeat(existingNode.ID)

			// Push probe config
			pushProbeConfig(deps, existingNode.ID, conn)

			agentReadLoop(deps, existingNode.ID, conn)
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

			var msg model.WSMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				log.Printf("Failed to parse agent message: %v", err)
				continue
			}

			if msg.Type != model.AgentTypeRegister {
				log.Printf("Expected register message, got: %s", msg.Type)
				continue
			}

			// Parse register payload
			payloadBytes, _ := json.Marshal(msg.Payload)
			var reg model.AgentRegisterPayload
			if err := json.Unmarshal(payloadBytes, &reg); err != nil {
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
			deps.NodeManager.RegisterConn(node.ID, conn)
			defer func() {
				deps.NodeManager.RemoveConn(node.ID)
				_ = store.SetNodeStatus(node.ID, "offline")
				log.Printf("Agent disconnected (new): node=%s", node.ID)
			}()

			log.Printf("Agent registered: node=%s hostname=%s os=%s ip=%s", node.ID, reg.Hostname, reg.OS, reg.IP)

			// Push probe config if available
			pushProbeConfig(deps, node.ID, conn)

			// Enter normal read loop
			agentReadLoop(deps, node.ID, conn)
			return
		}
	}
}

// agentReadLoop handles ongoing heartbeat and command messages from a connected agent.
func agentReadLoop(deps *Deps, nodeID string, conn *websocket.Conn) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("Agent WebSocket error: %v", err)
			}
			break
		}

		var msg model.WSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("Failed to parse agent message: %v", err)
			continue
		}

		switch msg.Type {
		case model.AgentTypeRegister:
			// Already registered, update info
			handleAgentReRegister(nodeID, msg.Payload)

		case model.AgentTypeHeartbeat:
			handleAgentHeartbeat(deps, nodeID, msg.Payload)

		default:
			log.Printf("Unknown agent message type: %s", msg.Type)
		}
	}
}

func handleAgentReRegister(nodeID string, payload any) {
	payloadBytes, _ := json.Marshal(payload)
	var reg model.AgentRegisterPayload
	if err := json.Unmarshal(payloadBytes, &reg); err != nil {
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

func handleAgentHeartbeat(deps *Deps, nodeID string, payload any) {
	payloadBytes, _ := json.Marshal(payload)
	var hb model.AgentHeartbeatPayload
	if err := json.Unmarshal(payloadBytes, &hb); err != nil {
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

// pushProbeConfig sends the current probe configuration to an agent.
// This is called after agent registration/reconnection.
func pushProbeConfig(deps *Deps, nodeID string, conn *websocket.Conn) {
	if !deps.Config.Probe.Enabled {
		return
	}

	// Build probe targets from links where this node is the source
	// For simplicity, read all links and filter
	probeStore := store.DB
	var links []model.ProbeLink
	probeStore.Where("source_id = ?", nodeID).Preload("Target").Find(&links)

	if len(links) == 0 {
		return
	}

	targets := make([]model.ProbeTargetConfig, 0, len(links))
	for _, link := range links {
		if link.Target == nil {
			continue
		}
		targets = append(targets, model.ProbeTargetConfig{
			ID:        link.TargetID,
			Name:      link.Target.Name,
			Host:      link.Target.IP,
			PingCount: 10,
			TCPPort:   443,
		})
	}

	serverAddr := fmt.Sprintf("http://%s:%d", deps.Config.Server.Host, deps.Config.Server.Port)
	if deps.Config.Server.Host == "0.0.0.0" {
		serverAddr = fmt.Sprintf("http://127.0.0.1:%d", deps.Config.Server.Port)
	}

	msg := model.WSMessage{
		Type: model.AgentTypeProbeConfig,
		Payload: model.AgentProbeConfigPayload{
			Enabled:   true,
			ReportURL: serverAddr + "/api/v1/probe/report",
			Targets:   targets,
		},
	}

	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("Failed to push probe_config to node %s: %v", nodeID, err)
	} else {
		log.Printf("Pushed probe_config to node %s (%d targets)", nodeID, len(targets))
	}
}
