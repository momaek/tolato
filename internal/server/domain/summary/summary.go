package summary

import (
	"context"

	"github.com/momaek/tolato/internal/shared/types"
)

type Service interface {
	SummarizeTask(ctx context.Context, task types.Task, executions []types.TaskExecution, aggregate types.TaskAggregate) (Result, error)
}

type Result struct {
	Summary        string
	ResultSummary  string
	FailureNodeIDs []string
	Source         string
}
