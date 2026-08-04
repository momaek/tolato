package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/authz"
	"github.com/momaek/tolato/server/internal/middleware"
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
