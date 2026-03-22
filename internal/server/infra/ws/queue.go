package ws

import "sync"

const defaultSendQueueCapacity = 16

type SendQueue interface {
	Offer(msg []byte) bool
	Len() int
	Cap() int
	Close()
	Messages() <-chan []byte
}

type BoundedSendQueue struct {
	mu        sync.Mutex
	closed    bool
	ch        chan []byte
	closeOnce sync.Once
}

func NewBoundedSendQueue(capacity int) *BoundedSendQueue {
	if capacity < 1 {
		capacity = defaultSendQueueCapacity
	}
	return &BoundedSendQueue{
		ch: make(chan []byte, capacity),
	}
}

func (q *BoundedSendQueue) Offer(msg []byte) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return false
	}

	payload := cloneBytes(msg)
	select {
	case q.ch <- payload:
		return true
	default:
		return false
	}
}

func (q *BoundedSendQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.ch)
}

func (q *BoundedSendQueue) Cap() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return cap(q.ch)
}

func (q *BoundedSendQueue) Close() {
	q.closeOnce.Do(func() {
		q.mu.Lock()
		q.closed = true
		close(q.ch)
		q.mu.Unlock()
	})
}

func (q *BoundedSendQueue) Messages() <-chan []byte {
	return q.ch
}

func cloneBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

