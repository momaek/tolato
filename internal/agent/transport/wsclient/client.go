package wsclient

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/internal/shared/protocol"
	"go.uber.org/zap"
)

type Client interface {
	Connect(ctx context.Context, url string) error
	Send(ctx context.Context, env protocol.Envelope) error
	Incoming() <-chan protocol.Envelope
	Errors() <-chan error
	Close() error
	Connected() bool
}

type GorillaClient struct {
	logger   *zap.Logger
	incoming chan protocol.Envelope
	errs     chan error

	mu   sync.RWMutex
	conn *websocket.Conn
}

func NewClient(logger *zap.Logger) *GorillaClient {
	return &GorillaClient{
		logger:   logger,
		incoming: make(chan protocol.Envelope, 32),
		errs:     make(chan error, 8),
	}
}

func (c *GorillaClient) Connect(ctx context.Context, url string) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	go c.readPump()
	return nil
}

func (c *GorillaClient) Send(ctx context.Context, env protocol.Envelope) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return websocket.ErrCloseSent
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
	} else {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}

	return conn.WriteJSON(env)
}

func (c *GorillaClient) Incoming() <-chan protocol.Envelope {
	return c.incoming
}

func (c *GorillaClient) Errors() <-chan error {
	return c.errs
}

func (c *GorillaClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *GorillaClient) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil
}

func (c *GorillaClient) readPump() {
	for {
		var env protocol.Envelope
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn == nil {
			return
		}

		if err := conn.ReadJSON(&env); err != nil {
			select {
			case c.errs <- err:
			default:
				c.logger.Warn("dropping wsclient error", zap.Error(err))
			}
			return
		}

		select {
		case c.incoming <- env:
		default:
			c.logger.Warn("dropping wsclient incoming message", zap.String("type", env.Type))
		}
	}
}
