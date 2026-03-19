package approval

import "context"

type Service interface {
	Approve(ctx context.Context, taskID string) error
	Reject(ctx context.Context, taskID string) error
	Cancel(ctx context.Context, taskID string) error
}
