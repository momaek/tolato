package task

import (
	"context"

	"github.com/momaek/tolato/internal/shared/types"
)

type Task = types.Task
type TaskExecution = types.TaskExecution

type Repository interface {
	Create(ctx context.Context, task Task) error
	Get(ctx context.Context, id string) (*Task, error)
	List(ctx context.Context) ([]Task, error)
	Update(ctx context.Context, task Task) error
	ListExecutions(ctx context.Context, taskID string) ([]TaskExecution, error)
	UpsertExecution(ctx context.Context, execution TaskExecution) error
}
