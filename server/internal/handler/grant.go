package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// ListGrants handles GET /api/grants (admin only), resolving subject and object
// names so the UI can render each rule as a sentence.
func ListGrants(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		grants, err := store.ListGrants()
		if err != nil {
			internalError(c, "failed to list grants")
			return
		}

		names, err := grantNameIndex()
		if err != nil {
			internalError(c, "failed to resolve grant names")
			return
		}

		items := make([]model.GrantItem, 0, len(grants))
		for _, g := range grants {
			items = append(items, model.GrantItem{
				ID:          g.ID,
				SubjectType: g.SubjectType,
				SubjectID:   g.SubjectID,
				SubjectName: names.subject(g.SubjectType, g.SubjectID),
				ObjectType:  g.ObjectType,
				ObjectID:    g.ObjectID,
				ObjectName:  names.object(g.ObjectType, g.ObjectID),
				Level:       g.Level,
				CreatedAt:   g.CreatedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

// CreateGrant handles POST /api/grants (admin only).
func CreateGrant(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.CreateGrantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			badRequest(c, "invalid request body")
			return
		}

		if !model.IsValidLevel(req.Level) {
			badRequest(c, "level must be viewer, operator or manager")
			return
		}

		// Both sides are validated against the real rows: a grant naming a
		// subject or object that doesn't exist would sit in the table looking
		// authoritative while matching nothing — or worse, start matching once
		// that id is created later.
		switch req.SubjectType {
		case model.SubjectUser:
			if _, err := store.GetUserByID(req.SubjectID); err != nil {
				badRequest(c, "no such user")
				return
			}
		case model.SubjectUserGroup:
			if _, err := store.GetUserGroupByID(req.SubjectID); err != nil {
				badRequest(c, "no such user group")
				return
			}
		default:
			badRequest(c, "subject_type must be user or user_group")
			return
		}

		objectIDs, err := grantObjectIDs(&req)
		if err != nil {
			badRequest(c, err.Error())
			return
		}

		author := middleware.CurrentUserID(c)
		grants := make([]*model.Grant, 0, len(objectIDs))
		for _, objectID := range objectIDs {
			grants = append(grants, &model.Grant{
				ID:          uuid.New().String(),
				SubjectType: req.SubjectType,
				SubjectID:   req.SubjectID,
				ObjectType:  req.ObjectType,
				ObjectID:    objectID,
				Level:       req.Level,
				CreatedBy:   author,
			})
		}
		if err := store.UpsertGrants(grants); err != nil {
			internalError(c, "failed to save grant")
			return
		}
		c.JSON(http.StatusCreated, gin.H{"items": grants})
	}
}

// grantObjectIDs merges the single- and multi-object forms of the request into
// one deduped list, with every id checked against a real row: a grant naming an
// object that doesn't exist would sit in the table looking authoritative while
// matching nothing — or worse, start matching once that id is created later.
func grantObjectIDs(req *model.CreateGrantRequest) ([]string, error) {
	switch req.ObjectType {
	case model.ObjectAll:
		// Normalized to a single row with no id, so "all" is one rule rather
		// than one per stray id the client sent along with it.
		return []string{""}, nil
	case model.ObjectNode, model.ObjectNodeGroup:
	default:
		return nil, errors.New("object_type must be node, node_group or all")
	}

	ids := make([]string, 0, len(req.ObjectIDs)+1)
	seen := make(map[string]bool, len(req.ObjectIDs)+1)
	for _, id := range append(append([]string{}, req.ObjectIDs...), req.ObjectID) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("name at least one %s", strings.ReplaceAll(req.ObjectType, "_", " "))
	}

	for _, id := range ids {
		var err error
		if req.ObjectType == model.ObjectNode {
			_, err = store.GetNodeByID(id)
		} else {
			_, err = store.GetNodeGroupByID(id)
		}
		if err != nil {
			return nil, fmt.Errorf("no such %s: %s", strings.ReplaceAll(req.ObjectType, "_", " "), id)
		}
	}
	return ids, nil
}

// DeleteGrant handles DELETE /api/grants/:id (admin only).
func DeleteGrant(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := store.DeleteGrant(c.Param("id")); err != nil {
			internalError(c, "failed to delete grant")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}

// userLabel names a user the way a human would recognise them. An SSO username
// can be as anonymous as "user", so the display name leads and the username
// stays alongside it to keep two people with the same display name apart.
func userLabel(username, displayName string) string {
	if displayName == "" || displayName == username {
		return username
	}
	return displayName + " (" + username + ")"
}

// nameIndex resolves grant subject/object ids to display names in bulk.
type nameIndex struct {
	users      map[string]string
	userGroups map[string]string
	nodes      map[string]string
	nodeGroups map[string]string
}

func grantNameIndex() (*nameIndex, error) {
	idx := &nameIndex{
		users:      map[string]string{},
		userGroups: map[string]string{},
		nodes:      map[string]string{},
		nodeGroups: map[string]string{},
	}

	users, err := store.ListUsers()
	if err != nil {
		return nil, err
	}
	for i := range users {
		idx.users[users[i].ID] = userLabel(users[i].Username, users[i].DisplayName)
	}

	ugs, err := store.ListUserGroups()
	if err != nil {
		return nil, err
	}
	for _, g := range ugs {
		idx.userGroups[g.ID] = g.Name
	}

	nodes, _, err := store.ListNodes(1, 10000, "")
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		idx.nodes[nodes[i].ID] = nodeDisplayName(&nodes[i])
	}

	ngs, err := store.ListNodeGroups()
	if err != nil {
		return nil, err
	}
	for _, g := range ngs {
		idx.nodeGroups[g.ID] = g.Name
	}
	return idx, nil
}

func (n *nameIndex) subject(kind, id string) string {
	if kind == model.SubjectUserGroup {
		return orUnknown(n.userGroups[id])
	}
	return orUnknown(n.users[id])
}

func (n *nameIndex) object(kind, id string) string {
	switch kind {
	case model.ObjectAll:
		return "*"
	case model.ObjectNodeGroup:
		return orUnknown(n.nodeGroups[id])
	default:
		return orUnknown(n.nodes[id])
	}
}

// orUnknown keeps a row renderable when the thing it names has vanished. The
// cascade deletes should prevent this, so seeing it means something is wrong.
func orUnknown(name string) string {
	if name == "" {
		return "(deleted)"
	}
	return name
}

// nodeDisplayName prefers the user-set alias over the reported hostname.
func nodeDisplayName(n *model.Node) string {
	if n.Alias != nil && *n.Alias != "" {
		return *n.Alias
	}
	return n.Name
}
