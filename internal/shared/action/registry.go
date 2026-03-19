package action

import "github.com/momaek/tolato/internal/shared/types"

var registry = []types.ActionSpec{
	{Name: "system_status", RiskLevel: "low", ApprovalRequired: false, BroadcastAllowed: true, TimeoutSec: 10},
	{Name: "disk_usage", RiskLevel: "low", ApprovalRequired: false, BroadcastAllowed: true, TimeoutSec: 10},
	{Name: "memory_usage", RiskLevel: "low", ApprovalRequired: false, BroadcastAllowed: true, TimeoutSec: 10},
	{Name: "docker_ps", RiskLevel: "low", ApprovalRequired: false, BroadcastAllowed: true, TimeoutSec: 10},
	{Name: "service_status", RiskLevel: "low", ApprovalRequired: false, BroadcastAllowed: true, TimeoutSec: 10},
	{Name: "tail_log", RiskLevel: "low", ApprovalRequired: false, BroadcastAllowed: true, TimeoutSec: 15},
	{Name: "restart_service", RiskLevel: "medium", ApprovalRequired: true, BroadcastAllowed: false, TimeoutSec: 30},
	{Name: "reload_service", RiskLevel: "medium", ApprovalRequired: true, BroadcastAllowed: false, TimeoutSec: 30},
	{Name: "network_check", RiskLevel: "low", ApprovalRequired: false, BroadcastAllowed: true, TimeoutSec: 15},
}

func List() []types.ActionSpec {
	items := make([]types.ActionSpec, len(registry))
	copy(items, registry)
	return items
}

func Get(name string) (types.ActionSpec, bool) {
	for _, spec := range registry {
		if spec.Name == name {
			return spec, true
		}
	}
	return types.ActionSpec{}, false
}
