package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Identity holds the persistent agent identity assigned by the server.
type Identity struct {
	NodeID   string `json:"node_id"`
	Secret   string `json:"secret"`
	Hostname string `json:"hostname"`
	Region   string `json:"region,omitempty"`
	OS       string `json:"os"`
	Version  string `json:"version"`
}

// Store manages reading and writing identity to disk.
type Store struct {
	path string // full path to identity.json
}

// NewStore creates a Store that persists identity at dataDir/identity.json.
func NewStore(dataDir string) *Store {
	return &Store{
		path: filepath.Join(dataDir, "identity.json"),
	}
}

// Load reads the identity file. Returns nil if the file does not exist.
func (s *Store) Load() (*Identity, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, err
	}
	return &id, nil
}

// Save writes the identity to disk, creating the directory if needed.
func (s *Store) Save(id *Identity) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

// Path returns the identity file path (for logging).
func (s *Store) Path() string {
	return s.path
}
