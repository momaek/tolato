package ws

import (
	"sync"
	"sync/atomic"
)

type MemoryClient struct {
	id   string
	kind ClientKind
	send *BoundedSendQueue

	closed     atomic.Bool
	closeOnce  sync.Once
	closeCode  int
	closeReason string
}

func NewMemoryClient(id string, kind ClientKind, sendQueueCapacity int) *MemoryClient {
	return &MemoryClient{
		id:   id,
		kind: kind,
		send: NewBoundedSendQueue(sendQueueCapacity),
	}
}

func (c *MemoryClient) ID() string {
	return c.id
}

func (c *MemoryClient) Kind() ClientKind {
	return c.kind
}

func (c *MemoryClient) Send(msg []byte) bool {
	if c.closed.Load() {
		return false
	}
	if c.send.Offer(msg) {
		return true
	}
	c.Close(1001, "send queue full")
	return false
}

func (c *MemoryClient) Close(code int, reason string) {
	c.closeOnce.Do(func() {
		c.closeCode = code
		c.closeReason = reason
		c.closed.Store(true)
		c.send.Close()
	})
}

func (c *MemoryClient) Messages() <-chan []byte {
	return c.send.Messages()
}

func (c *MemoryClient) Closed() bool {
	return c.closed.Load()
}

func (c *MemoryClient) CloseCode() int {
	return c.closeCode
}

func (c *MemoryClient) CloseReason() string {
	return c.closeReason
}

func (c *MemoryClient) SendQueueLen() int {
	return c.send.Len()
}

