package agentstate

import "sync/atomic"

type BusyTracker struct {
	active atomic.Int32
}

func NewBusyTracker() *BusyTracker {
	return &BusyTracker{}
}

func (t *BusyTracker) Start() {
	t.active.Add(1)
}

func (t *BusyTracker) Done() {
	for {
		current := t.active.Load()
		if current <= 0 {
			return
		}
		if t.active.CompareAndSwap(current, current-1) {
			return
		}
	}
}

func (t *BusyTracker) IsBusy() bool {
	return t.active.Load() > 0
}
