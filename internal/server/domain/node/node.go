package node

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/shared/types"
)

type Node = types.Node
type NodeSession = types.NodeSession

type Repository interface {
	List(ctx context.Context) ([]Node, error)
	Get(ctx context.Context, id string) (*Node, error)
	GetByAgentCredentials(ctx context.Context, id, secret string) (*Node, error)
	Upsert(ctx context.Context, node Node) error
	UpdatePresence(ctx context.Context, nodeID, version, status string, seenAt time.Time) error
}

type SessionStore interface {
	Upsert(ctx context.Context, session NodeSession) error
	Get(ctx context.Context, sessionID string) (*NodeSession, error)
	ListByNodeID(ctx context.Context, nodeID string) ([]NodeSession, error)
	MarkDisconnected(ctx context.Context, sessionID string, at time.Time) error
}
