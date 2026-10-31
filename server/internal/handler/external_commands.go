package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

type commandCursor struct {
	Version int    `json:"v"`
	NodeID  string `json:"node_id"`
	Poll    bool   `json:"poll,omitempty"`
	store.CommandEventWindow
}

func queryPositiveInt(c *gin.Context, key string, def, max int) (int, bool) {
	s, exists := c.GetQuery(key)
	if !exists {
		return def, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > max {
		badRequest(c, "invalid "+key)
		return 0, false
	}
	return n, true
}

// ExternalListCommands exposes all audit sources using a deliberately small DTO.
func ExternalListCommands(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if !requireNodeLevel(c, id, model.LevelViewer) {
			return
		}
		page, ok := queryPositiveInt(c, "page", 1, 1000000)
		if !ok {
			return
		}
		size, ok := queryPositiveInt(c, "page_size", 20, 100)
		if !ok {
			return
		}
		cursor := commandCursor{Version: 1, NodeID: id}
		if raw, exists := c.GetQuery("cursor"); exists {
			data, err := base64.RawURLEncoding.DecodeString(raw)
			if len(raw) > 2048 || err != nil || json.Unmarshal(data, &cursor) != nil ||
				cursor.Version != 1 || cursor.NodeID != id || cursor.LowerID > cursor.UpperID ||
				(!cursor.Poll && (cursor.AfterID <= cursor.LowerID || cursor.AfterID > cursor.UpperID || cursor.AfterTime.IsZero())) ||
				(cursor.Poll && (cursor.LowerID != cursor.UpperID || cursor.AfterID != 0 || !cursor.AfterTime.IsZero())) || page != 1 || c.Query("since") != "" {
				badRequest(c, "invalid cursor; use it without since or page")
				return
			}
			if cursor.Poll {
				cursor.UpperID, err = store.LatestNodeAuditID(id)
				if err != nil {
					internalError(c, "failed to list commands")
					return
				}
				cursor.Poll = false
			}
		} else {
			if since, exists := c.GetQuery("since"); exists {
				t, err := time.Parse(time.RFC3339Nano, since)
				if err != nil {
					badRequest(c, "since must be RFC 3339")
					return
				}
				cursor.Since = t
			}
			var err error
			cursor.UpperID, err = store.LatestNodeAuditID(id)
			if err != nil {
				internalError(c, "failed to list commands")
				return
			}
		}
		items, total, err := store.ListNodeCommandEvents(id, cursor.CommandEventWindow, page, size)
		if err != nil {
			internalError(c, "failed to list commands")
			return
		}
		next, checkpoint := "", ""
		if len(items) > size {
			items = items[:size]
			last := items[len(items)-1]
			cursor.AfterID, cursor.AfterTime = last.ID, last.CreatedAt
			data, _ := json.Marshal(cursor)
			next = base64.RawURLEncoding.EncodeToString(data)
		} else {
			// A polling checkpoint advances by insertion id, so records arriving
			// later with an equal/older timestamp are still seen next time.
			cursor.LowerID = cursor.UpperID
			cursor.AfterID, cursor.AfterTime, cursor.Poll = 0, time.Time{}, true
			data, _ := json.Marshal(cursor)
			checkpoint = base64.RawURLEncoding.EncodeToString(data)
		}
		c.JSON(http.StatusOK, gin.H{
			"items": items, "total": total, "page": page, "page_size": size,
			"total_pages": (total + int64(size) - 1) / int64(size), "next_cursor": next, "checkpoint": checkpoint,
		})
	}
}
