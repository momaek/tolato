package ginws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

const sendQueueCapacity = 128

type UIHandler interface {
	Connect(ctx context.Context, client infraws.Client) ([]byte, error)
	Disconnect(clientID string)
	Handle(ctx context.Context, clientID string, raw []byte) ([]byte, error)
}

type AgentHandler interface {
	Connect(ctx context.Context, client infraws.Client) error
	Disconnect(clientID string)
	Handle(ctx context.Context, clientID string, raw []byte) ([]byte, error)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func RegisterUIRoute(router gin.IRouter, path string, handler UIHandler) {
	router.GET(path, func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		serveUI(c.Request.Context(), conn, handler)
	})
}

func RegisterAgentRoute(router gin.IRouter, path string, handler AgentHandler) {
	router.GET(path, func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		serveAgent(c.Request.Context(), conn, handler)
	})
}

func serveUI(ctx context.Context, conn *websocket.Conn, handler UIHandler) {
	defer conn.Close()

	client := infraws.NewMemoryClient(newClientID("ui"), infraws.ClientKindUI, sendQueueCapacity)
	done := startWriter(conn, client)
	defer func() {
		handler.Disconnect(client.ID())
		client.Close(1000, "disconnected")
		<-done
	}()

	ready, err := handler.Connect(ctx, client)
	if err != nil {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()), noDeadline())
		return
	}
	if len(ready) > 0 {
		client.Send(ready)
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		resp, err := handler.Handle(ctx, client.ID(), raw)
		if err != nil {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()), noDeadline())
			return
		}
		if len(resp) > 0 {
			client.Send(resp)
		}
	}
}

func serveAgent(ctx context.Context, conn *websocket.Conn, handler AgentHandler) {
	defer conn.Close()

	client := infraws.NewMemoryClient(newClientID("agent"), infraws.ClientKindAgent, sendQueueCapacity)
	done := startWriter(conn, client)
	defer func() {
		handler.Disconnect(client.ID())
		client.Close(1000, "disconnected")
		<-done
	}()

	if err := handler.Connect(ctx, client); err != nil {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()), noDeadline())
		return
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		resp, err := handler.Handle(ctx, client.ID(), raw)
		if err != nil {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()), noDeadline())
			return
		}
		if len(resp) > 0 {
			client.Send(resp)
		}
	}
}

func startWriter(conn *websocket.Conn, client *infraws.MemoryClient) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for msg := range client.Messages() {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()
	return done
}

func newClientID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return prefix
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

func noDeadline() time.Time {
	return time.Time{}
}
