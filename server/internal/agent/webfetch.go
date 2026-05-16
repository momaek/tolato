package agent

import (
	"context"

	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/webfetch"
)

// executeWebFetch adapts the shared webfetch package into a ToolResultItem for
// the LLM tool loop. The fetch core lives in internal/webfetch so the MCP
// handler can reuse it without pulling in this package.
func (te *ToolExecutor) executeWebFetch(ctx context.Context, target string) *model.ToolResultItem {
	res, err := webfetch.Fetch(ctx, te.settings, target)
	if err != nil {
		return &model.ToolResultItem{Data: map[string]any{"error": err.Error()}}
	}
	return &model.ToolResultItem{
		Data: map[string]any{
			"url":          res.URL,
			"source":       res.Source,
			"status":       res.Status,
			"content_type": res.ContentType,
			"content":      res.Content,
			"bytes":        res.Bytes,
			"truncated":    res.Truncated,
		},
	}
}
