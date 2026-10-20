package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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

		metadata, err := metadataFilter(c)
		if err != nil {
			badRequest(c, err.Error())
			return
		}

		filter := store.AuditLogFilter{
			ID: q.ID, NodeID: q.NodeID, Keyword: q.Keyword, From: fromTime, To: toTime, Metadata: metadata,
		}
		logs, total, err := store.ListAuditLogsScoped(q.Page, q.PageSize, filter, visibleIDs, unrestricted)
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to list audit logs",
			})
			return
		}

		items := make([]model.AuditLogItem, 0, len(logs))
		for _, l := range logs {
			items = append(items, auditLogItem(l))
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

// auditLogItem is the API view of one audit row.
func auditLogItem(l model.AuditLog) model.AuditLogItem {
	return model.AuditLogItem{
		ID:             l.ID,
		NodeID:         l.NodeID,
		NodeName:       l.NodeName,
		Actor:          l.Actor,
		Command:        l.Command,
		ExitCode:       l.ExitCode,
		Stdout:         l.Stdout,
		Stderr:         l.Stderr,
		OutputRedacted: l.StdoutDigest != nil || l.StderrDigest != nil,
		StdoutDigest:   l.StdoutDigest,
		StderrDigest:   l.StderrDigest,
		DurationMS:     l.DurationMS,
		Confirmed:      l.Confirmed,
		Source:         l.Source,
		Metadata:       l.Metadata,
		CreatedAt:      l.CreatedAt,
	}
}

// Bounds on the metadata a caller may attach to an audited call. It is
// bookkeeping, not storage: enough for a few ids and a name, not a payload.
const (
	maxMetadataKeys     = 32
	maxMetadataKeyLen   = 64
	maxMetadataValueLen = 256
)

// metadataQueryPrefix marks an audit-log filter on a metadata key, as in
// ?metadata.ticket=T-42.
const metadataQueryPrefix = "metadata."

// validateMetadataKey keeps keys to a charset that survives being written as a
// query parameter name.
func validateMetadataKey(k string) error {
	if k == "" || len(k) > maxMetadataKeyLen {
		return fmt.Errorf("metadata key %q must be 1 to %d bytes", k, maxMetadataKeyLen)
	}
	for _, r := range k {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.'
		if !ok {
			return fmt.Errorf("metadata key %q may only use letters, digits, '_', '-' and '.'", k)
		}
	}
	return nil
}

// validateMetadata checks the tags a caller attached to a call before they are
// written to the audit log.
func validateMetadata(m map[string]string) error {
	if len(m) > maxMetadataKeys {
		return fmt.Errorf("metadata has %d keys, at most %d are allowed", len(m), maxMetadataKeys)
	}
	for k, v := range m {
		if err := validateMetadataKey(k); err != nil {
			return err
		}
		if len(v) > maxMetadataValueLen {
			return fmt.Errorf("metadata value for %q is %d bytes, at most %d are allowed", k, len(v), maxMetadataValueLen)
		}
		// jsonb rejects NUL, and invalid UTF-8 would be silently mangled.
		if !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
			return fmt.Errorf("metadata value for %q must be valid UTF-8 text", k)
		}
	}
	return nil
}

// metadataFilter collects the metadata.<key>=<value> parameters of an
// audit-log query. A repeated key keeps its first value.
func metadataFilter(c *gin.Context) (map[string]string, error) {
	var out map[string]string
	for name, values := range c.Request.URL.Query() {
		key, ok := strings.CutPrefix(name, metadataQueryPrefix)
		if !ok || len(values) == 0 {
			continue
		}
		if err := validateMetadataKey(key); err != nil {
			return nil, err
		}
		if out == nil {
			out = map[string]string{}
		}
		out[key] = values[0]
	}
	return out, nil
}
