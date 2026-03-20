package cancellation

import "sync"

type Store struct {
	mu      sync.RWMutex
	reasons map[string]string
}

func NewStore() *Store {
	return &Store{
		reasons: make(map[string]string),
	}
}

func (s *Store) Mark(executionID, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reasons[executionID] = reason
}

func (s *Store) Consume(executionID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reason, ok := s.reasons[executionID]
	if ok {
		delete(s.reasons, executionID)
	}
	return reason, ok
}

func (s *Store) Peek(executionID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	reason, ok := s.reasons[executionID]
	return reason, ok
}
