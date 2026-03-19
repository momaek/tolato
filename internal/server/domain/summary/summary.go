package summary

import "context"

type Service interface {
	SummarizeTask(ctx context.Context, taskID string) (string, error)
}
