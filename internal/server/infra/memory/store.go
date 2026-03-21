package memory

import (
	"context"
	"errors"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/momaek/tolato/internal/server/domain/audit"
	"github.com/momaek/tolato/internal/server/domain/node"
	"github.com/momaek/tolato/internal/server/domain/outbox"
	"github.com/momaek/tolato/internal/server/domain/task"
)

type backend struct {
	mu         sync.RWMutex
	nodes      map[string]node.Node
	sessions   map[string]node.NodeSession
	tasks      map[string]task.Task
	executions map[string][]task.TaskExecution
	audits     []audit.AuditEvent
	outbox     map[string]outbox.Message
}

type NodeRepo struct{ b *backend }
type SessionRepo struct{ b *backend }
type TaskRepo struct{ b *backend }
type AuditRepo struct{ b *backend }
type OutboxRepo struct{ b *backend }

func NewStores() (*NodeRepo, *SessionRepo, *TaskRepo, *AuditRepo, *OutboxRepo) {
	b := &backend{
		nodes:      make(map[string]node.Node),
		sessions:   make(map[string]node.NodeSession),
		tasks:      make(map[string]task.Task),
		executions: make(map[string][]task.TaskExecution),
		audits:     make([]audit.AuditEvent, 0),
		outbox:     make(map[string]outbox.Message),
	}
	return &NodeRepo{b: b}, &SessionRepo{b: b}, &TaskRepo{b: b}, &AuditRepo{b: b}, &OutboxRepo{b: b}
}

func (r *NodeRepo) List(ctx context.Context) ([]node.Node, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()

	items := make([]node.Node, 0, len(r.b.nodes))
	for _, n := range r.b.nodes {
		items = append(items, n)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (r *NodeRepo) Get(ctx context.Context, id string) (*node.Node, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()

	n, ok := r.b.nodes[id]
	if !ok {
		return nil, errors.New("node not found")
	}
	copyNode := n
	return &copyNode, nil
}

func (r *NodeRepo) GetByAgentCredentials(ctx context.Context, id, secret string) (*node.Node, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()

	n, ok := r.b.nodes[id]
	if !ok || n.AgentSecret != secret {
		return nil, errors.New("agent authentication failed")
	}
	copyNode := n
	return &copyNode, nil
}

func (r *NodeRepo) Upsert(ctx context.Context, n node.Node) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()

	if existing, ok := r.b.nodes[n.ID]; ok {
		if n.CreatedAt.IsZero() {
			n.CreatedAt = existing.CreatedAt
		}
	}
	r.b.nodes[n.ID] = n
	return nil
}

func (r *NodeRepo) UpdatePresence(ctx context.Context, nodeID, version, status string, seenAt time.Time) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()

	n, ok := r.b.nodes[nodeID]
	if !ok {
		return errors.New("node not found")
	}
	n.Status = status
	n.LastSeenAt = seenAt
	n.UpdatedAt = seenAt
	if version != "" {
		n.Version = version
	}
	r.b.nodes[nodeID] = n
	return nil
}

func (r *SessionRepo) Upsert(ctx context.Context, session node.NodeSession) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	r.b.sessions[session.SessionID] = session
	return nil
}

func (r *SessionRepo) Get(ctx context.Context, sessionID string) (*node.NodeSession, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()

	item, ok := r.b.sessions[sessionID]
	if !ok {
		return nil, errors.New("session not found")
	}
	copyItem := item
	return &copyItem, nil
}

func (r *SessionRepo) ListByNodeID(ctx context.Context, nodeID string) ([]node.NodeSession, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()

	items := make([]node.NodeSession, 0)
	for _, session := range r.b.sessions {
		if session.NodeID == nodeID {
			items = append(items, session)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ConnectedAt.Before(items[j].ConnectedAt)
	})
	return items, nil
}

func (r *SessionRepo) MarkDisconnected(ctx context.Context, sessionID string, at time.Time) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()

	item, ok := r.b.sessions[sessionID]
	if !ok {
		return errors.New("session not found")
	}
	item.Status = "disconnected"
	item.DisconnectedAt = at.UTC()
	if item.LastHeartbeatAt.IsZero() {
		item.LastHeartbeatAt = at.UTC()
	}
	r.b.sessions[sessionID] = item
	return nil
}

func (r *TaskRepo) Create(ctx context.Context, t task.Task) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	r.b.tasks[t.ID] = t
	return nil
}

func (r *TaskRepo) Get(ctx context.Context, id string) (*task.Task, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()

	t, ok := r.b.tasks[id]
	if !ok {
		return nil, errors.New("task not found")
	}
	copyTask := t
	return &copyTask, nil
}

func (r *TaskRepo) List(ctx context.Context) ([]task.Task, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()

	items := make([]task.Task, 0, len(r.b.tasks))
	for _, item := range r.b.tasks {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func (r *TaskRepo) Update(ctx context.Context, t task.Task) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()

	if _, ok := r.b.tasks[t.ID]; !ok {
		return errors.New("task not found")
	}
	r.b.tasks[t.ID] = t
	return nil
}

func (r *TaskRepo) ListExecutions(ctx context.Context, taskID string) ([]task.TaskExecution, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()
	return slices.Clone(r.b.executions[taskID]), nil
}

func (r *TaskRepo) UpsertExecution(ctx context.Context, execution task.TaskExecution) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()

	items := slices.Clone(r.b.executions[execution.TaskID])
	for idx, existing := range items {
		if existing.ID == execution.ID {
			items[idx] = execution
			r.b.executions[execution.TaskID] = items
			return nil
		}
	}

	items = append(items, execution)
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].ID < items[j].ID
		}
		if items[i].StartedAt.IsZero() {
			return false
		}
		if items[j].StartedAt.IsZero() {
			return true
		}
		return items[i].StartedAt.Before(items[j].StartedAt)
	})
	r.b.executions[execution.TaskID] = items
	return nil
}

func (r *AuditRepo) Create(ctx context.Context, event audit.AuditEvent) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()

	r.b.audits = append(r.b.audits, event)
	sort.Slice(r.b.audits, func(i, j int) bool {
		return r.b.audits[i].CreatedAt.Before(r.b.audits[j].CreatedAt)
	})
	return nil
}

func (r *AuditRepo) ListByTaskID(ctx context.Context, taskID string) ([]audit.AuditEvent, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()

	events := make([]audit.AuditEvent, 0)
	for _, event := range r.b.audits {
		if taskID == "" || event.TaskID == taskID {
			events = append(events, event)
		}
	}
	return events, nil
}

func (r *OutboxRepo) Create(ctx context.Context, message outbox.Message) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	r.b.outbox[message.ID] = message
	return nil
}

func (r *OutboxRepo) ListPending(ctx context.Context, limit int) ([]outbox.Message, error) {
	_ = ctx
	r.b.mu.RLock()
	defer r.b.mu.RUnlock()

	items := make([]outbox.Message, 0)
	for _, item := range r.b.outbox {
		if item.PublishedAt.IsZero() {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *OutboxRepo) MarkPublished(ctx context.Context, id string, at time.Time) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()

	item, ok := r.b.outbox[id]
	if !ok {
		return errors.New("outbox message not found")
	}
	item.PublishedAt = at.UTC()
	r.b.outbox[id] = item
	return nil
}

func (r *OutboxRepo) IncrementAttempts(ctx context.Context, id string) error {
	_ = ctx
	r.b.mu.Lock()
	defer r.b.mu.Unlock()

	item, ok := r.b.outbox[id]
	if !ok {
		return errors.New("outbox message not found")
	}
	item.Attempts++
	r.b.outbox[id] = item
	return nil
}
