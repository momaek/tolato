package client

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/agent/internal/services"
)

func (c *Client) EnableServices(manager *services.Manager) { c.services = manager }

// Bind the reply to the initiating connection: a reconnect may have an unrelated
// request with the same caller-supplied id, and must never receive a stale reply.
func (c *Client) handleService(conn *websocket.Conn, msg WSMessage) {
	var req struct {
		Service string `json:"service"`
		services.Request
	}
	var status services.Status
	var errText string
	if c.services == nil {
		errText = "managed services are disabled"
	} else if len(msg.Payload) > 73728 || json.Unmarshal(msg.Payload, &req) != nil || req.Service != services.Name {
		errText = "invalid service request"
	} else {
		ctx, cancel := context.WithTimeout(c.ctx, 140*time.Second)
		var err error
		status, err = c.services.Apply(ctx, req.Request)
		cancel()
		if err != nil {
			errText = err.Error()
		}
	}
	raw, _ := json.Marshal(struct {
		OK     bool            `json:"ok"`
		Error  string          `json:"error,omitempty"`
		Status services.Status `json:"status"`
	}{errText == "", errText, status})
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn != conn {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := conn.WriteJSON(WSMessage{Type: "service_result", ID: msg.ID, Payload: raw}); err != nil {
		_ = conn.Close()
	}
}
