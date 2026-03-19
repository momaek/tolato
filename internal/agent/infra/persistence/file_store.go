package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/momaek/tolato/internal/shared/types"
)

type IdentityStore interface {
	Load(ctx context.Context) (types.AgentIdentity, error)
	Save(ctx context.Context, identity types.AgentIdentity) error
}

type FileStore struct {
	path string
}

func NewFileStore(path string) FileStore {
	return FileStore{path: path}
}

func (s FileStore) Load(ctx context.Context) (types.AgentIdentity, error) {
	_ = ctx
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return types.AgentIdentity{}, err
	}

	var identity types.AgentIdentity
	if err := json.Unmarshal(raw, &identity); err != nil {
		return types.AgentIdentity{}, err
	}

	if identity.NodeID == "" || identity.Secret == "" {
		return types.AgentIdentity{}, errors.New("identity file is incomplete")
	}

	return identity, nil
}

func (s FileStore) Save(ctx context.Context, identity types.AgentIdentity) error {
	_ = ctx
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, raw, 0o600)
}
