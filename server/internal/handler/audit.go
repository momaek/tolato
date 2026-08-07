package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/authz"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// ListAuditLogs handles GET /api/audit-logs.
func ListAuditLogs(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.AuditLogQuery
		if err := c.ShouldBindQuery(&q); err != nil {
			q = model.AuditLogQuery{}
		}
		if q.Page <= 0 {
			q.Page = 1
		}
		if q.PageSize <= 0 || q.PageSize > 100 {
			q.PageSize = 20
		}

		var fromTime, toTime *time.Time
		if q.From != nil && *q.From != "" {
			t, err := time.Parse(time.RFC3339, *q.From)
			if err == nil {
				fromTime = &t
			}
		}
		if q.To != nil && *q.To != "" {
			t, err := time.Parse(time.RFC3339, *q.To)
			if err == nil {
				toTime = &t
			}
		}

		// Audit rows quote command output verbatim, so a row about a node the
		// caller can't see would leak that machine's contents. Scope the query
		// to the visible set rather than filtering afterwards, so paging stays
		// meaningful.
		visibleIDs, unrestricted, err := authz.VisibleNodeIDs(subjectOf(c))
		if err != nil {
			internalError(c, "failed to check permissions")
			return
		}
		if !unrestricted && len(visibleIDs) == 0 {
			c.JSON(http.StatusOK, model.PaginatedResponse{
				Items: []model.AuditLogItem{}, Total: 0, Page: q.Page, PageSize: q.PageSize, TotalPages: 0,
			})
			return
		}
		// A node_id filter must itself be inside the visible set, or it becomes
		// a way to probe for nodes by watching result counts.
		if q.NodeID != nil && *q.NodeID != "" && !unrestricted {
			allowed := false
			for _, id := range visibleIDs {
				if id == *q.NodeID {
					allowed = true
					break
				}
			}
			if !allowed {
				notFound(c, "node not found")
				return
			}
		}

		logs, total, err := store.ListAuditLogsScoped(q.Page, q.PageSize, q.NodeID, q.Keyword, fromTime, toTime, visibleIDs, unrestricted)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to list audit logs",
			})
			return
		}

		items := make([]model.AuditLogItem, 0, len(logs))
		for _, l := range logs {
			items = append(items, model.AuditLogItem{
				ID:         l.ID,
				NodeID:     l.NodeID,
				NodeName:   l.NodeName,
				Actor:      l.Actor,
				Command:    l.Command,
				ExitCode:   l.ExitCode,
				Stdout:     l.Stdout,
				Stderr:     l.Stderr,
				DurationMS: l.DurationMS,
				Confirmed:  l.Confirmed,
				Source:     l.Source,
				CreatedAt:  l.CreatedAt,
			})
		}

		totalPages := int(total) / q.PageSize
		if int(total)%q.PageSize > 0 {
			totalPages++
		}

		c.JSON(http.StatusOK, model.PaginatedResponse{
			Items:      items,
			Total:      int(total),
			Page:       q.Page,
			PageSize:   q.PageSize,
			TotalPages: totalPages,
		})
	}
}
