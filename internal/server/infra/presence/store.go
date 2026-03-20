package presence

import (
	"sync"
	"time"

	"github.com/momaek/tolato/internal/shared/types"
)

type Snapshot struct {
	Busy       bool
	Metrics    types.NodeMetrics
	LastSeenAt time.Time
}

type Store struct {
	mu        sync.RWMutex
	snapshots map[string]Snapshot
}

func NewStore() *Store {
	return &Store{
		snapshots: make(map[string]Snapshot),
	}
}

func (s *Store) Upsert(nodeID string, snapshot Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[nodeID] = snapshot
}

func (s *Store) Get(nodeID string) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, ok := s.snapshots[nodeID]
	return snapshot, ok
}
