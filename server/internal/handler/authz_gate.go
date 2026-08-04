package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/authz"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// subjectOf builds the authz subject for the current request. It works for both
// authentication schemes: JWTAuth pins the signed-in user, and APIKeyAuth pins
// the key's owner, so a key never reaches further than the person who made it.
func subjectOf(c *gin.Context) authz.Subject {
	return authz.Subject{
		UserID:  middleware.CurrentUserID(c),
		IsAdmin: middleware.IsAdmin(c),
	}
}

// requireNodeLevel is the gate in front of every per-node route. It reports
// whether the caller may continue, having already written the response if not.
//
// Insufficient permission and a nonexistent node both answer 404. A 403 would
// confirm the node exists to somebody who has no business knowing that, which
// turns the node list into an enumeration oracle.
func requireNodeLevel(c *gin.Context, nodeID, want string) bool {
	ok, err := authz.Can(subjectOf(c), nodeID, want)
	if err != nil {
		internalError(c, "failed to check permissions")
		return false
	}
	if !ok {
		notFound(c, "node not found")
		return false
	}
	// Permission is settled; the node still has to exist.
	if _, err := store.GetNodeByID(nodeID); err != nil {
		notFound(c, "node not found")
		return false
	}
	return true
}

// visibleNodeFilter returns a predicate for "may this caller see this node",
// resolved once per request rather than per row.
func visibleNodeFilter(c *gin.Context) (func(nodeID string) bool, error) {
	ids, unrestricted, err := authz.VisibleNodeIDs(subjectOf(c))
	if err != nil {
		return nil, err
	}
	if unrestricted {
		return func(string) bool { return true }, nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return func(nodeID string) bool {
		_, ok := set[nodeID]
		return ok
	}, nil
}

// forbidden writes a 403. Used where the resource itself isn't secret and only
// the action is refused — group and settings administration, not nodes.
func forbidden(c *gin.Context, msg string) {
	c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "forbidden", Message: msg})
}
