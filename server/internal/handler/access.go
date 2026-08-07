package handler

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/authz"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// The grant list says what rules exist. These two endpoints say what those
// rules add up to — the questions an admin actually asks: "what can this
// person reach?" when onboarding or offboarding, and "who can touch this
// machine?" when something breaks. Both are admin-only.

// UserAccess handles GET /api/users/:id/access — every node the user can reach
// and the level they hold, resolved through their personal and group grants.
func UserAccess(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := store.GetUserByID(c.Param("id"))
		if err != nil {
			notFound(c, "user not found")
			return
		}

		nodes, _, err := store.ListNodes(1, 10000, "")
		if err != nil {
			internalError(c, "failed to list nodes")
			return
		}
		ids := make([]string, 0, len(nodes))
		byID := make(map[string]*model.Node, len(nodes))
		for i := range nodes {
			ids = append(ids, nodes[i].ID)
			byID[nodes[i].ID] = &nodes[i]
		}

		levels, err := authz.LevelsForNodes(authz.SubjectOf(user), ids)
		if err != nil {
			internalError(c, "failed to resolve access")
			return
		}

		items := make([]model.NodeAccessItem, 0, len(levels))
		for nodeID, level := range levels {
			items = append(items, model.NodeAccessItem{
				NodeID:   nodeID,
				NodeName: nodeDisplayName(byID[nodeID]),
				Level:    level,
			})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].NodeName < items[j].NodeName })

		c.JSON(http.StatusOK, model.UserAccessResponse{
			// Admins reach everything by role rather than by grant, which is why
			// the list can be long without a single grant existing.
			ViaAdminRole: user.IsAdmin(),
			Items:        items,
		})
	}
}

// NodeAccess handles GET /api/nodes/:id/access — who can reach one node, and
// how they came by it.
func NodeAccess(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")
		if _, err := store.GetNodeByID(nodeID); err != nil {
			notFound(c, "node not found")
			return
		}

		users, err := store.ListUsers()
		if err != nil {
			internalError(c, "failed to list users")
			return
		}

		items := make([]model.UserAccessItem, 0, len(users))
		for i := range users {
			u := &users[i]
			if u.Status != model.UserStatusActive {
				continue // a disabled account reaches nothing
			}
			level, ok, err := authz.NodeLevel(authz.SubjectOf(u), nodeID)
			if err != nil {
				internalError(c, "failed to resolve access")
				return
			}
			if !ok {
				continue
			}
			items = append(items, model.UserAccessItem{
				UserID:       u.ID,
				Username:     u.Username,
				Level:        level,
				ViaAdminRole: u.IsAdmin(),
			})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Username < items[j].Username })

		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}
