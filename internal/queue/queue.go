// Package queue 基于 SQLite jobs 表实现一个简单的任务调度器与 worker pool。
package queue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/store"
)

// Handler 处理某类任务，返回 error 会按重试策略处理。
type Handler func(ctx context.Context, job *domain.Job) error

type Scheduler struct {
	store    *store.Store
	log      *zap.Logger
	handlers map[string]Handler
	owner    string
	lease    time.Duration
	workers  int
}

func New(st *store.Store, log *zap.Logger, workers int) *Scheduler {
	if workers < 1 {
		workers = 1
	}
	return &Scheduler{
		store:    st,
		log:      log,
		handlers: map[string]Handler{},
		owner:    uuid.NewString(),
		lease:    2 * time.Minute,
		workers:  workers,
	}
}

func (s *Scheduler) Register(kind string, h Handler) {
	s.handlers[kind] = h
}

// Enqueue 便捷入队，序列化 payload。
func (s *Scheduler) Enqueue(ctx context.Context, kind string, payload any, idempotencyKey string) (int64, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	id, err := s.store.EnqueueJob(ctx, kind, string(raw), idempotencyKey, 3)
	if err == store.ErrDuplicate {
		return 0, nil // 幂等：已有相同任务，不算错误
	}
	return id, err
}

// Start 启动调度循环，直到 ctx 取消。
func (s *Scheduler) Start(ctx context.Context) {
	tasks := make(chan *domain.Job, s.workers)
	for i := 0; i < s.workers; i++ {
		go s.worker(ctx, tasks)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler stopping")
			return
		case <-ticker.C:
			s.dispatch(ctx, tasks)
		}
	}
}

func (s *Scheduler) dispatch(ctx context.Context, tasks chan<- *domain.Job) {
	for len(tasks) < cap(tasks) {
		job, err := s.store.ClaimJob(ctx, s.owner, s.lease)
		if err != nil {
			s.log.Error("claim job", zap.Error(err))
			return
		}
		if job == nil {
			return
		}
		select {
		case tasks <- job:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) worker(ctx context.Context, tasks <-chan *domain.Job) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-tasks:
			s.runJob(ctx, job)
		}
	}
}

func (s *Scheduler) runJob(ctx context.Context, job *domain.Job) {
	h, ok := s.handlers[job.Kind]
	if !ok {
		s.log.Error("no handler for job kind", zap.String("kind", job.Kind))
		_ = s.store.FailJob(ctx, job.ID, "no handler for kind: "+job.Kind)
		return
	}

	// 心跳续租
	hbCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go s.heartbeat(hbCtx, job.ID)

	if err := h(ctx, job); err != nil {
		s.log.Warn("job failed", zap.Int64("id", job.ID), zap.Error(err))
		if ferr := s.store.FailJob(ctx, job.ID, err.Error()); ferr != nil {
			s.log.Error("mark job failed", zap.Error(ferr))
		}
		return
	}
	if err := s.store.CompleteJob(ctx, job.ID); err != nil {
		s.log.Error("mark job complete", zap.Error(err))
	}
}

func (s *Scheduler) heartbeat(ctx context.Context, jobID int64) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.store.Heartbeat(ctx, jobID, s.owner, s.lease); err != nil {
				s.log.Debug("heartbeat", zap.Error(err))
			}
		}
	}
}
