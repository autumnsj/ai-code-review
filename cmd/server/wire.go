package main

import (
	"context"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/queue"
)

// reviewEnqueuer 把 queue.Scheduler 适配为 webhook 所需的 Enqueuer 接口。
type reviewEnqueuer struct {
	q *queue.Scheduler
}

func (e *reviewEnqueuer) EnqueueReview(ctx context.Context, payload domain.ReviewPayload, idempotencyKey string) error {
	_, err := e.q.Enqueue(ctx, "review", payload, idempotencyKey)
	return err
}
