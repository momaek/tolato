package nodeview

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/domain"
)

type NodeSource interface {
	ListNodes(ctx context.Context) ([]policy.NodeSummary, error)
}

type Service interface {
	ListNodes(ctx context.Context, filter ListFilter) ([]NodeSummary, error)
	GetNode(ctx context.Context, nodeID string) (NodeDetail, error)
}

type Repositories struct {
	Sessions   domain.SessionRepository
	Tasks      domain.TaskRepository
	Executions domain.ExecutionRepository
}

type ListFilter struct {
	Query  string
	Status string
	Busy   *bool
	Region string
	Tag    string
	Limit  int
}

type Metrics struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
	Disk   float64 `json:"disk"`
}

type NodeSummary struct {
	ID         string   `json:"id"`
	Hostname   string   `json:"hostname"`
	Region     string   `json:"region"`
	OS         string   `json:"os"`
	Version    string   `json:"version"`
	Tags       []string `json:"tags"`
	Status     string   `json:"status"`
	Busy       bool     `json:"busy"`
	LastSeenAt string   `json:"last_seen_at"`
	Metrics    Metrics  `json:"metrics"`
}

type NodeTaskSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type NodeDetail struct {
	NodeSummary
	IPAddress  string            `json:"ip_address"`
	Provider   string            `json:"provider"`
	Kernel     string            `json:"kernel"`
	Uptime     string            `json:"uptime"`
	AgentVer   string            `json:"agent_version"`
	RiskSignal []string          `json:"risk_signals"`
	RecentTask []NodeTaskSummary `json:"recent_tasks"`
}

type service struct {
	source NodeSource
	repos  Repositories
}

func NewService(source NodeSource, repos Repositories) Service {
	return &service{source: source, repos: repos}
}

func (s *service) ListNodes(ctx context.Context, filter ListFilter) ([]NodeSummary, error) {
	if s.source == nil {
		return nil, domain.ErrUnsupportedConfig
	}

	nodes, err := s.source.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]NodeSummary, 0, len(nodes))
	for _, node := range nodes {
		if !matchesFilter(node, filter) {
			continue
		}
		items = append(items, summaryFromPolicy(node))
	}

	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func (s *service) GetNode(ctx context.Context, nodeID string) (NodeDetail, error) {
	if nodeID == "" {
		return NodeDetail{}, domain.ErrInvalidArgument
	}
	if s.source == nil {
		return NodeDetail{}, domain.ErrUnsupportedConfig
	}

	nodes, err := s.source.ListNodes(ctx)
	if err != nil {
		return NodeDetail{}, err
	}
	for _, node := range nodes {
		if node.ID != nodeID {
			continue
		}
		return s.detailFromPolicy(ctx, node)
	}
	return NodeDetail{}, domain.ErrNotFound
}

func summaryFromPolicy(node policy.NodeSummary) NodeSummary {
	return NodeSummary{
		ID:         node.ID,
		Hostname:   node.Hostname,
		Region:     node.Region,
		OS:         node.OS,
		Version:    node.Version,
		Tags:       append([]string(nil), node.Tags...),
		Status:     node.Status,
		Busy:       node.Busy,
		LastSeenAt: node.LastSeen,
		Metrics: Metrics{
			CPU:    node.Metrics.CPU,
			Memory: node.Metrics.Memory,
			Disk:   node.Metrics.Disk,
		},
	}
}

func (s *service) detailFromPolicy(ctx context.Context, node policy.NodeSummary) (NodeDetail, error) {
	recentTasks, err := s.recentTasks(ctx, node.ID)
	if err != nil {
		return NodeDetail{}, err
	}
	meta := nodeDetailMetadata(node)
	return NodeDetail{
		NodeSummary: summaryFromPolicy(node),
		IPAddress:   meta.IPAddress,
		Provider:    meta.Provider,
		Kernel:      meta.Kernel,
		Uptime:      meta.Uptime,
		AgentVer:    meta.AgentVersion,
		RiskSignal:  buildRiskSignals(node, recentTasks),
		RecentTask:  recentTasks,
	}, nil
}

func (s *service) recentTasks(ctx context.Context, nodeID string) ([]NodeTaskSummary, error) {
	if s.repos.Sessions == nil || s.repos.Tasks == nil {
		return []NodeTaskSummary{}, nil
	}

	sessions, err := s.repos.Sessions.List(ctx, domain.SessionFilter{})
	if err != nil {
		return nil, err
	}

	items := make([]NodeTaskSummary, 0)
	for _, session := range sessions {
		tasks, err := s.repos.Tasks.ListBySession(ctx, session.ID)
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if !taskTouchesNode(task, nodeID) {
				continue
			}
			items = append(items, NodeTaskSummary{
				ID:        task.ID,
				Title:     taskTitle(task),
				Status:    mapNodeTaskStatus(task.Status),
				CreatedAt: task.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	if len(items) > 5 {
		items = items[:5]
	}
	return items, nil
}

func buildRiskSignals(node policy.NodeSummary, recentTasks []NodeTaskSummary) []string {
	signals := make([]string, 0, 4)
	if node.Status == "offline" || node.Status == "stale" {
		signals = append(signals, fmt.Sprintf("Node status is %s. Execution confidence is degraded until heartbeat recovers.", node.Status))
	}
	if node.Busy || node.Metrics.CPU >= 0.8 {
		signals = append(signals, "Node is under active workload. Schedule disruptive changes carefully.")
	}
	if node.Metrics.Disk >= 0.85 {
		signals = append(signals, "Disk utilization is above 85%. Avoid log-heavy or package operations before cleanup.")
	}
	for _, task := range recentTasks {
		if task.Status == "failed" {
			signals = append(signals, "Recent execution history includes failures on this node. Review the latest task before retrying.")
			break
		}
	}
	if len(signals) == 0 {
		signals = append(signals, "No elevated runtime risk detected from the current control-plane projection.")
	}
	return signals
}

type detailMetadata struct {
	IPAddress    string
	Provider     string
	Kernel       string
	Uptime       string
	AgentVersion string
}

func nodeDetailMetadata(node policy.NodeSummary) detailMetadata {
	switch node.ID {
	case "jp-tokyo-01":
		return detailMetadata{
			IPAddress:    "10.10.1.21",
			Provider:     "aws-apne1",
			Kernel:       "Linux 6.8.0-31-generic",
			Uptime:       "18d 4h",
			AgentVersion: node.Version,
		}
	case "jp-tokyo-02":
		return detailMetadata{
			IPAddress:    "10.10.1.22",
			Provider:     "aws-apne1",
			Kernel:       "Linux 6.8.0-31-generic",
			Uptime:       "26d 9h",
			AgentVersion: node.Version,
		}
	case "us-sfo-01":
		return detailMetadata{
			IPAddress:    "10.30.4.11",
			Provider:     "gcp-usw1",
			Kernel:       "Linux 6.1.0-27-cloud-amd64",
			Uptime:       "11d 2h",
			AgentVersion: node.Version,
		}
	case "eu-fra-01":
		return detailMetadata{
			IPAddress:    "10.40.8.17",
			Provider:     "hetzner-fsn1",
			Kernel:       "Linux 5.15.0-113-generic",
			Uptime:       "42d 7h",
			AgentVersion: node.Version,
		}
	case "sg-edge-01":
		return detailMetadata{
			IPAddress:    "10.50.2.31",
			Provider:     "do-sgp1",
			Kernel:       "Linux 6.8.0-31-generic",
			Uptime:       "3d 5h",
			AgentVersion: node.Version,
		}
	default:
		return detailMetadata{
			IPAddress:    "-",
			Provider:     "unknown",
			Kernel:       "-",
			Uptime:       "-",
			AgentVersion: node.Version,
		}
	}
}

func taskTouchesNode(task domain.Task, nodeID string) bool {
	for _, candidate := range task.OperationTargetSnapshot.NodeIDs {
		if candidate == nodeID {
			return true
		}
	}
	return strings.TrimSpace(task.OperationTargetSnapshot.DisplayLabel) == nodeID
}

func taskTitle(task domain.Task) string {
	if strings.TrimSpace(task.InputText) != "" {
		return task.InputText
	}
	if strings.TrimSpace(task.OperationTargetSnapshot.DisplayLabel) != "" {
		return task.OperationTargetSnapshot.DisplayLabel
	}
	return task.ID
}

func mapNodeTaskStatus(status domain.TaskStatus) string {
	switch status {
	case domain.TaskStatusSuccess:
		return "success"
	case domain.TaskStatusFailed, domain.TaskStatusPartialFailed, domain.TaskStatusCancelled, domain.TaskStatusTimeout:
		return "failed"
	case domain.TaskStatusWaitingApproval, domain.TaskStatusApproved:
		return "waiting_approval"
	default:
		return "running"
	}
}

func matchesFilter(node policy.NodeSummary, filter ListFilter) bool {
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	status := strings.ToLower(strings.TrimSpace(filter.Status))
	region := strings.ToLower(strings.TrimSpace(filter.Region))
	tag := strings.ToLower(strings.TrimSpace(filter.Tag))

	if query != "" && !matchesQuery(node, query) {
		return false
	}
	if status != "" && strings.ToLower(node.Status) != status {
		return false
	}
	if filter.Busy != nil && node.Busy != *filter.Busy {
		return false
	}
	if region != "" && strings.ToLower(node.Region) != region {
		return false
	}
	if tag != "" && !hasTag(node.Tags, tag) {
		return false
	}
	return true
}

func matchesQuery(node policy.NodeSummary, query string) bool {
	if strings.Contains(strings.ToLower(node.ID), query) {
		return true
	}
	if strings.Contains(strings.ToLower(node.Hostname), query) {
		return true
	}
	if strings.Contains(strings.ToLower(node.Region), query) {
		return true
	}
	for _, tag := range node.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func hasTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if strings.ToLower(tag) == wanted {
			return true
		}
	}
	return false
}
