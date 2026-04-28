package handler

import (
	"encoding/json"
	"log"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/geoip"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/store"
)

// resolvePublicIP returns connIP only when it parses to a routable public
// address. Used to filter out the docker bridge gateway / loopback /
// Cloudflare-edge cases where c.ClientIP() doesn't reflect the real client.
func resolvePublicIP(connIP string) string {
	ip := net.ParseIP(connIP)
	if ip == nil || !geoip.IsPublicIP(ip) {
		return ""
	}
	return connIP
}

// preferredIP picks the better of an agent-reported IP and a server-detected
// connection IP.
//
// Default to trusting the agent's self-report — getLocalIP() works correctly
// for the vast majority of nodes (single-NIC public VPS). Only fall back to
// the connection IP when the agent reported something private/unparseable
// (NAT'd hosts that picked the wrong interface). This deliberately avoids
// overwriting working public IPs with whatever the connection happens to be —
// behind Cloudflare/Caddy that connection IP might be a CDN edge address, not
// the real client.
func preferredIP(agentIP, connPublicIP string) string {
	if ip := net.ParseIP(agentIP); ip != nil && geoip.IsPublicIP(ip) {
		return agentIP
	}
	if connPublicIP != "" {
		return connPublicIP
	}
	return agentIP
}

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

		// Capture the real client IP from the connection (honors X-Forwarded-For
		// only for trusted proxies set in router.go). Empty when the apparent
		// source is private/loopback — usually means the reverse proxy isn't
		// forwarding X-Forwarded-For, in which case we'd be writing the docker
		// bridge gateway (e.g. 172.22.0.1) into every node row. Better to fall
		// back to the agent's self-report than to corrupt every row.
		clientIP := resolvePublicIP(c.ClientIP())

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
			installSystemHandlers(deps, existingNode.ID, clientIP, ac)
			defer func() {
				deps.NodeManager.RemoveConn(existingNode.ID)
				_ = store.SetNodeStatus(existingNode.ID, "offline")
				log.Printf("Agent disconnected (reconnect): node=%s", existingNode.ID)
			}()

			// Heal nodes whose stored IP is private/garbage (NAT'd agent picked
			// the wrong interface) by replacing with the public connection IP
			// when available. Don't disturb already-good public IPs.
			if newIP := preferredIP(existingNode.IP, clientIP); newIP != existingNode.IP {
				updates := map[string]any{"ip": newIP}
				if geo, _ := deps.GeoIP.Lookup(newIP); !geo.IsZero() {
					updates["country_code"] = geo.CountryCode
					updates["city"] = geo.City
					updates["asn"] = geo.ASN
				}
				_ = store.UpdateNode(existingNode.ID, updates)
			}

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

			reg.IP = preferredIP(reg.IP, clientIP)

			// Create node in database (with best-effort GeoIP lookup)
			agentSecret := uuid.New().String()
			geo, _ := deps.GeoIP.Lookup(reg.IP)
			node, err := store.CreateNodeFromRegistration(reg, regToken.AliasPrefix, agentSecret, geo)
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
			installSystemHandlers(deps, node.ID, clientIP, ac)
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
//
// clientIP is the public source IP captured at WebSocket upgrade time, or
// empty when the connection came from a private/loopback address.
func installSystemHandlers(deps *Deps, nodeID, clientIP string, ac *node.AgentConn) {
	ac.SetSystemHandlers(node.SystemHandlers{
		OnHeartbeat: func(payload json.RawMessage) {
			handleAgentHeartbeat(deps, nodeID, payload)
		},
		OnReRegister: func(payload json.RawMessage) {
			handleAgentReRegister(deps, nodeID, clientIP, payload)
		},
	})
}

func handleAgentReRegister(deps *Deps, nodeID, clientIP string, payload json.RawMessage) {
	var reg model.AgentRegisterPayload
	if err := json.Unmarshal(payload, &reg); err != nil {
		return
	}
	ip := preferredIP(reg.IP, clientIP)
	updates := map[string]any{
		"name":            reg.Hostname,
		"os":              reg.OS,
		"kernel":          reg.Kernel,
		"ip":              ip,
		"agent_version":   reg.AgentVersion,
		"cpu_cores":       reg.CPUCores,
		"memory_total_mb": reg.MemoryTotalMB,
		"disk_total_gb":   reg.DiskTotalGB,
		"status":          "online",
	}
	if geo, _ := deps.GeoIP.Lookup(ip); !geo.IsZero() {
		updates["country_code"] = geo.CountryCode
		updates["city"] = geo.City
		updates["asn"] = geo.ASN
	}
	_ = store.UpdateNode(nodeID, updates)
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

