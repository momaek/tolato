package policy

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// ConfirmTokenStore manages short-lived, single-use confirmation tokens.
// When a dangerous operation is requested, a token is generated and returned
// to the LLM. The user confirms, and on the next turn the LLM passes
// the token back. The token is consumed (deleted) on validation.
type ConfirmTokenStore struct {
	mu     sync.Mutex
	tokens map[string]*confirmEntry
	ttl    time.Duration
}

type confirmEntry struct {
	NodeIDs   []string
	Command   string
	Args      []string
	ExpiresAt time.Time
}

func NewConfirmTokenStore() *ConfirmTokenStore {
	return &ConfirmTokenStore{
		tokens: make(map[string]*confirmEntry),
		ttl:    5 * time.Minute,
	}
}

// Generate creates a new confirmation token for the given operation.
func (s *ConfirmTokenStore) Generate(nodeIDs []string, command string, args []string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked()

	token := uuid.New().String()
	s.tokens[token] = &confirmEntry{
		NodeIDs:   append([]string(nil), nodeIDs...),
		Command:   command,
		Args:      append([]string(nil), args...),
		ExpiresAt: time.Now().Add(s.ttl),
	}
	return token
}

// Validate consumes a token, returning the stored operation details.
// Returns ok=false if the token is invalid, expired, or already consumed.
func (s *ConfirmTokenStore) Validate(token string) (nodeIDs []string, command string, args []string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.tokens[token]
	if !exists {
		return nil, "", nil, false
	}
	delete(s.tokens, token)

	if time.Now().After(entry.ExpiresAt) {
		return nil, "", nil, false
	}
	return entry.NodeIDs, entry.Command, entry.Args, true
}

func (s *ConfirmTokenStore) cleanupLocked() {
	now := time.Now()
	for token, entry := range s.tokens {
		if now.After(entry.ExpiresAt) {
			delete(s.tokens, token)
		}
	}
}
