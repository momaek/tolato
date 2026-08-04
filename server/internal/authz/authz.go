// Package authz answers one question — what may this user do to this node —
// and is the only place that answer is computed.
//
// Every surface funnels through here: the REST handlers, the terminal
// WebSocket, the chat tool executor, and the API-key endpoints. Keeping the
// rule in one place is what makes it auditable; a second implementation
// somewhere else is how a permission system quietly grows a hole.
package authz

import (
	"fmt"

	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// Subject is the identity a request acts as.
type Subject struct {
	UserID  string
	IsAdmin bool
}

// SubjectOf builds a Subject from a user record.
func SubjectOf(u *model.User) Subject {
	return Subject{UserID: u.ID, IsAdmin: u.IsAdmin()}
}

// NodeLevel returns the subject's effective level on one node, folding together
// every grant that matches — held personally or through a group, and pointing
// at the node, one of its groups, or all nodes. The most permissive wins.
//
// The bool is false when no grant matches at all, which callers must treat as
// "this node does not exist" rather than "access denied": saying "denied"
// confirms the node is real to someone who should not know that.
//
// Admins short-circuit to manager. Their reach is a property of the role, not
// of grant rows, so an admin never has to grant themselves access to fix a box.
func NodeLevel(s Subject, nodeID string) (string, bool, error) {
	if s.IsAdmin {
		return model.LevelManager, true, nil
	}
	if s.UserID == "" || nodeID == "" {
		return "", false, nil
	}

	grants, err := subjectGrants(s)
	if err != nil {
		return "", false, err
	}
	if len(grants) == 0 {
		return "", false, nil
	}

	nodeGroups, err := store.ListGroupIDsForNode(nodeID)
	if err != nil {
		return "", false, fmt.Errorf("node groups: %w", err)
	}
	inGroup := make(map[string]bool, len(nodeGroups))
	for _, g := range nodeGroups {
		inGroup[g] = true
	}

	best := ""
	for _, g := range grants {
		switch g.ObjectType {
		case model.ObjectAll:
		case model.ObjectNode:
			if g.ObjectID != nodeID {
				continue
			}
		case model.ObjectNodeGroup:
			if !inGroup[g.ObjectID] {
				continue
			}
		default:
			continue
		}
		best = model.HigherLevel(best, g.Level)
	}

	if best == "" {
		return "", false, nil
	}
	return best, true, nil
}

// Can reports whether the subject holds at least `want` on the node.
func Can(s Subject, nodeID, want string) (bool, error) {
	level, ok, err := NodeLevel(s, nodeID)
	if err != nil || !ok {
		return false, err
	}
	return model.LevelAtLeast(level, want), nil
}

// VisibleNodeIDs returns the nodes the subject may see, and whether that set is
// unrestricted. When unrestricted is true the id slice is nil and callers
// should skip filtering entirely — that is the admin and grant-on-all case,
// where materializing every id would be pure waste.
func VisibleNodeIDs(s Subject) (ids []string, unrestricted bool, err error) {
	if s.IsAdmin {
		return nil, true, nil
	}
	if s.UserID == "" {
		return nil, false, nil
	}

	grants, err := subjectGrants(s)
	if err != nil {
		return nil, false, err
	}

	set := map[string]struct{}{}
	for _, g := range grants {
		switch g.ObjectType {
		case model.ObjectAll:
			return nil, true, nil
		case model.ObjectNode:
			set[g.ObjectID] = struct{}{}
		case model.ObjectNodeGroup:
			members, err := store.ListNodeGroupMemberIDs(g.ObjectID)
			if err != nil {
				return nil, false, fmt.Errorf("node group members: %w", err)
			}
			for _, id := range members {
				set[id] = struct{}{}
			}
		}
	}

	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out, false, nil
}

// LevelsForNodes resolves the subject's level on each of the given nodes in one
// pass, for list views that want to show what the viewer may do with each row.
// Nodes with no access are absent from the map.
func LevelsForNodes(s Subject, nodeIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(nodeIDs))
	if s.IsAdmin {
		for _, id := range nodeIDs {
			out[id] = model.LevelManager
		}
		return out, nil
	}

	grants, err := subjectGrants(s)
	if err != nil {
		return nil, err
	}
	if len(grants) == 0 {
		return out, nil
	}

	// One pass over the membership table beats a per-node query.
	groupsByNode, err := store.GroupIDsByNode()
	if err != nil {
		return nil, err
	}

	for _, nodeID := range nodeIDs {
		inGroup := make(map[string]bool)
		for _, g := range groupsByNode[nodeID] {
			inGroup[g] = true
		}
		best := ""
		for _, g := range grants {
			switch g.ObjectType {
			case model.ObjectAll:
			case model.ObjectNode:
				if g.ObjectID != nodeID {
					continue
				}
			case model.ObjectNodeGroup:
				if !inGroup[g.ObjectID] {
					continue
				}
			default:
				continue
			}
			best = model.HigherLevel(best, g.Level)
		}
		if best != "" {
			out[nodeID] = best
		}
	}
	return out, nil
}

// subjectGrants loads every grant the subject holds, personally or by group.
func subjectGrants(s Subject) ([]model.Grant, error) {
	groupIDs, err := store.ListGroupIDsForUser(s.UserID)
	if err != nil {
		return nil, fmt.Errorf("user groups: %w", err)
	}
	grants, err := store.GrantsForSubjects(s.UserID, groupIDs)
	if err != nil {
		return nil, fmt.Errorf("grants: %w", err)
	}
	return grants, nil
}
