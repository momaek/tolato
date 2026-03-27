package nodes

import (
	"context"
	"time"

	"github.com/momaek/tolato/internal/server/app/policy"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

const (
	defaultOnlineTTL  = 15 * time.Second
	defaultOfflineTTL = 30 * time.Second
)

type baseSource interface {
	ListNodes(ctx context.Context) ([]policy.NodeSummary, error)
}

type ObservedSource struct {
	Base       baseSource
	Presence   infraws.AgentRegistry
	Now        func() time.Time
	OnlineTTL  time.Duration
	OfflineTTL time.Duration
}

func NewObservedSource(base baseSource, presence infraws.AgentRegistry) *ObservedSource {
	return &ObservedSource{
		Base:     base,
		Presence: presence,
	}
}

func (s *ObservedSource) ListNodes(ctx context.Context) ([]policy.NodeSummary, error) {
	var baseNodes []policy.NodeSummary
	if s.Base != nil {
		nodes, err := s.Base.ListNodes(ctx)
		if err != nil {
			return nil, err
		}
		baseNodes = nodes
	}
	if s.Presence == nil {
		return append([]policy.NodeSummary(nil), baseNodes...), nil
	}

	now := s.now()
	snapshots := s.Presence.Snapshots()
	byNodeID := make(map[string]infraws.AgentPresenceSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byNodeID[snapshot.NodeID] = snapshot
	}

	out := make([]policy.NodeSummary, 0, len(baseNodes)+len(snapshots))
	seen := make(map[string]struct{}, len(baseNodes)+len(snapshots))
	for _, node := range baseNodes {
		if snapshot, ok := byNodeID[node.ID]; ok {
			node = overlayPresence(node, snapshot, now, s.onlineTTL(), s.offlineTTL())
		}
		out = append(out, node)
		seen[node.ID] = struct{}{}
	}

	for _, snapshot := range snapshots {
		if _, ok := seen[snapshot.NodeID]; ok {
			continue
		}
		out = append(out, syntheticNode(snapshot, now, s.onlineTTL(), s.offlineTTL()))
	}

	return out, nil
}

func (s *ObservedSource) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *ObservedSource) onlineTTL() time.Duration {
	if s.OnlineTTL > 0 {
		return s.OnlineTTL
	}
	return defaultOnlineTTL
}

func (s *ObservedSource) offlineTTL() time.Duration {
	if s.OfflineTTL > 0 {
		return s.OfflineTTL
	}
	return defaultOfflineTTL
}

func overlayPresence(node policy.NodeSummary, snapshot infraws.AgentPresenceSnapshot, now time.Time, onlineTTL, offlineTTL time.Duration) policy.NodeSummary {
	if snapshot.Hostname != "" {
		node.Hostname = snapshot.Hostname
	}
	if snapshot.Region != "" {
		node.Region = snapshot.Region
	}
	if snapshot.OS != "" {
		node.OS = snapshot.OS
	}
	if snapshot.Version != "" {
		node.Version = snapshot.Version
	}
	if snapshot.IPAddress != "" {
		node.IPAddress = snapshot.IPAddress
	}
	if len(snapshot.Tags) > 0 {
		node.Tags = append([]string(nil), snapshot.Tags...)
	}
	node.Status = classifyPresence(node.Status, snapshot, now, onlineTTL, offlineTTL)
	node.Busy = snapshot.Busy
	node.Metrics = policy.Metrics{
		CPU:    snapshot.Metrics.CPU,
		Memory: snapshot.Metrics.Memory,
		Disk:   snapshot.Metrics.Disk,
	}
	if snapshot.LastHeartbeat != nil {
		node.LastSeen = snapshot.LastHeartbeat.UTC().Format(time.RFC3339)
	}
	return node
}

func syntheticNode(snapshot infraws.AgentPresenceSnapshot, now time.Time, onlineTTL, offlineTTL time.Duration) policy.NodeSummary {
	node := policy.NodeSummary{
		ID:        snapshot.NodeID,
		Hostname:  fallback(snapshot.Hostname, snapshot.NodeID),
		Region:    fallback(snapshot.Region, "Unknown"),
		OS:        fallback(snapshot.OS, "unknown"),
		Version:   fallback(snapshot.Version, "unknown"),
		IPAddress: snapshot.IPAddress,
		Tags:      append([]string(nil), snapshot.Tags...),
		Status:   classifyPresence("", snapshot, now, onlineTTL, offlineTTL),
		Busy:     snapshot.Busy,
		Metrics: policy.Metrics{
			CPU:    snapshot.Metrics.CPU,
			Memory: snapshot.Metrics.Memory,
			Disk:   snapshot.Metrics.Disk,
		},
	}
	if snapshot.LastHeartbeat != nil {
		node.LastSeen = snapshot.LastHeartbeat.UTC().Format(time.RFC3339)
	}
	return node
}

func fallback(value string, fallbackValue string) string {
	if value != "" {
		return value
	}
	return fallbackValue
}

func classifyPresence(baseStatus string, snapshot infraws.AgentPresenceSnapshot, now time.Time, onlineTTL, offlineTTL time.Duration) string {
	if snapshot.Bound && snapshot.LastHeartbeat == nil {
		return "online"
	}
	if snapshot.LastHeartbeat == nil {
		if baseStatus != "" {
			return baseStatus
		}
		return "offline"
	}

	age := now.Sub(snapshot.LastHeartbeat.UTC())
	switch {
	case snapshot.Bound && age <= onlineTTL:
		return "online"
	case age <= offlineTTL:
		return "stale"
	default:
		return "offline"
	}
}
