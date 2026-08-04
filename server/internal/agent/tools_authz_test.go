package agent

import "testing"

// A member's conversation must not even be offered the node-mutating tools.
// Withholding the definition is stronger than refusing the call: the model
// cannot invoke a tool it was never told exists, so there is no refusal for it
// to retry, rephrase, or argue with.
func TestToolDefsForTrimsMutatingToolsFromMembers(t *testing.T) {
	mutating := map[string]bool{"edit_node_info": true, "update_agent": true}

	memberTools := map[string]bool{}
	for _, d := range ToolDefsFor(false) {
		memberTools[d.Name] = true
	}
	for name := range mutating {
		if memberTools[name] {
			t.Errorf("member tool set includes %q, which can modify a node", name)
		}
	}

	// The read and execute tools must survive the trim, or members lose the
	// assistant entirely.
	for _, want := range []string{"list_nodes", "get_node_info", "execute_command", "web_fetch"} {
		if !memberTools[want] {
			t.Errorf("member tool set is missing %q", want)
		}
	}

	adminTools := map[string]bool{}
	for _, d := range ToolDefsFor(true) {
		adminTools[d.Name] = true
	}
	for name := range mutating {
		if !adminTools[name] {
			t.Errorf("admin tool set is missing %q", name)
		}
	}

	if len(adminTools) <= len(memberTools) {
		t.Errorf("expected the admin tool set (%d) to be larger than the member set (%d)",
			len(adminTools), len(memberTools))
	}
}

// ToolDefs is the legacy entry point and must keep returning everything, so
// nothing silently loses a capability.
func TestToolDefsReturnsFullSet(t *testing.T) {
	if len(ToolDefs()) != len(ToolDefsFor(true)) {
		t.Error("ToolDefs and ToolDefsFor(true) disagree on the full tool set")
	}
}
