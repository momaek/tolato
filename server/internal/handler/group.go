package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
	"gorm.io/gorm"
)

// Group management is admin-only throughout — these routes are mounted on the
// admin router, so no handler here re-checks the role.

// --- User groups ---

func ListUserGroups(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		groups, err := store.ListUserGroups()
		if err != nil {
			internalError(c, "failed to list user groups")
			return
		}
		items := make([]model.UserGroupItem, 0, len(groups))
		for _, g := range groups {
			members, err := store.ListUserGroupMemberIDs(g.ID)
			if err != nil {
				internalError(c, "failed to load group members")
				return
			}
			items = append(items, model.UserGroupItem{
				ID: g.ID, Name: g.Name, Description: g.Description,
				MemberIDs: members, CreatedAt: g.CreatedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func CreateUserGroup(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := bindGroupRequest(c)
		if !ok {
			return
		}
		name := derefTrim(req.Name)
		if name == "" {
			badRequest(c, "name is required")
			return
		}

		g := &model.UserGroup{ID: uuid.New().String(), Name: name, Description: derefTrim(req.Description)}
		if err := store.CreateUserGroup(g); err != nil {
			conflictOrInternal(c, err, "a group with that name already exists", "failed to create group")
			return
		}
		if req.MemberIDs != nil {
			if err := store.SetUserGroupMembers(g.ID, *req.MemberIDs); err != nil {
				internalError(c, "failed to set members")
				return
			}
		}
		c.JSON(http.StatusCreated, model.UserGroupItem{
			ID: g.ID, Name: g.Name, Description: g.Description,
			MemberIDs: valueOrEmpty(req.MemberIDs), CreatedAt: g.CreatedAt,
		})
	}
}

func UpdateUserGroup(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		req, ok := bindGroupRequest(c)
		if !ok {
			return
		}
		if _, err := store.GetUserGroupByID(id); err != nil {
			notFound(c, "group not found")
			return
		}

		if updates := groupUpdates(req); len(updates) > 0 {
			if err := store.UpdateUserGroup(id, updates); err != nil {
				conflictOrInternal(c, err, "a group with that name already exists", "failed to update group")
				return
			}
		}
		if req.MemberIDs != nil {
			if err := store.SetUserGroupMembers(id, *req.MemberIDs); err != nil {
				internalError(c, "failed to set members")
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "updated"})
	}
}

func DeleteUserGroup(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := store.DeleteUserGroup(c.Param("id")); err != nil {
			internalError(c, "failed to delete group")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}

// --- Node groups ---

func ListNodeGroups(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		groups, err := store.ListNodeGroups()
		if err != nil {
			internalError(c, "failed to list node groups")
			return
		}
		items := make([]model.NodeGroupItem, 0, len(groups))
		for _, g := range groups {
			members, err := store.ListNodeGroupMemberIDs(g.ID)
			if err != nil {
				internalError(c, "failed to load group members")
				return
			}
			items = append(items, model.NodeGroupItem{
				ID: g.ID, Name: g.Name, Description: g.Description,
				MemberIDs: members, CreatedAt: g.CreatedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func CreateNodeGroup(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := bindGroupRequest(c)
		if !ok {
			return
		}
		name := derefTrim(req.Name)
		if name == "" {
			badRequest(c, "name is required")
			return
		}

		g := &model.NodeGroup{ID: uuid.New().String(), Name: name, Description: derefTrim(req.Description)}
		if err := store.CreateNodeGroup(g); err != nil {
			conflictOrInternal(c, err, "a group with that name already exists", "failed to create group")
			return
		}
		if req.MemberIDs != nil {
			if err := store.SetNodeGroupMembers(g.ID, *req.MemberIDs); err != nil {
				internalError(c, "failed to set members")
				return
			}
		}
		c.JSON(http.StatusCreated, model.NodeGroupItem{
			ID: g.ID, Name: g.Name, Description: g.Description,
			MemberIDs: valueOrEmpty(req.MemberIDs), CreatedAt: g.CreatedAt,
		})
	}
}

func UpdateNodeGroup(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		req, ok := bindGroupRequest(c)
		if !ok {
			return
		}
		if _, err := store.GetNodeGroupByID(id); err != nil {
			notFound(c, "group not found")
			return
		}

		if updates := groupUpdates(req); len(updates) > 0 {
			if err := store.UpdateNodeGroup(id, updates); err != nil {
				conflictOrInternal(c, err, "a group with that name already exists", "failed to update group")
				return
			}
		}
		if req.MemberIDs != nil {
			if err := store.SetNodeGroupMembers(id, *req.MemberIDs); err != nil {
				internalError(c, "failed to set members")
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "updated"})
	}
}

func DeleteNodeGroup(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := store.DeleteNodeGroup(c.Param("id")); err != nil {
			internalError(c, "failed to delete group")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	}
}

// --- shared helpers ---

func bindGroupRequest(c *gin.Context) (model.GroupRequest, bool) {
	var req model.GroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid request body")
		return req, false
	}
	return req, true
}

func groupUpdates(req model.GroupRequest) map[string]any {
	updates := map[string]any{}
	if req.Name != nil {
		if name := strings.TrimSpace(*req.Name); name != "" {
			updates["name"] = name
		}
	}
	if req.Description != nil {
		updates["description"] = strings.TrimSpace(*req.Description)
	}
	return updates
}

func derefTrim(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func valueOrEmpty(s *[]string) []string {
	if s == nil {
		return []string{}
	}
	return *s
}

func badRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: msg})
}

func notFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: msg})
}

func internalError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: msg})
}

// conflictOrInternal maps a unique-constraint violation to 409 so a duplicate
// group name reads as a user mistake rather than a server fault.
func conflictOrInternal(c *gin.Context, err error, conflictMsg, internalMsg string) {
	if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate key") {
		c.JSON(http.StatusConflict, model.ErrorResponse{Error: "conflict", Message: conflictMsg})
		return
	}
	internalError(c, internalMsg)
}
