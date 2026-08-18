package main

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/ai-code-review/aicr/internal/store"
)

// 回收阈值：
//   - pendingGrace：审查入队后等待 worker 认领的最长时间。超过说明入队失败或 worker 未运行。
//   - runningGrace：审查进入 running 后的最长存活时间，需大于单次执行硬超时（25 分钟）+
//     退避重试窗口（10+20+40s）+ 余量，给足真正在跑的任务；超过则视为进程在写终态前被杀。
const (
	pendingGrace = 10 * time.Minute
	runningGrace = 35 * time.Minute
	reaperEvery  = time.Minute
)

// startReaper 启动定时回收 goroutine，把永远卡在 pending/running 的僵尸审查标记为 failed。
// review 与 job 是两套状态：正常完成/失败时 pipeline 会自己写 review 终态，但入队失败
// （webhook 注释里的"后台可补偿"）或进程在写终态前被杀会导致 review 永久卡住，重启也不会恢复。
// 回收时排除仍有活跃（pending/running）job 对应的审查，绝不误杀正在排队/重试/执行的任务。
func (a *application) startReaper(ctx context.Context, st *store.Store) {
	go func() {
		// 启动后稍等片刻再首次回收，给刚启动的 worker 认领任务的时间。
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
		ticker := time.NewTicker(reaperEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.reapOnce(ctx, st)
			}
		}
	}()
}

func (a *application) reapOnce(ctx context.Context, st *store.Store) {
	defer func() {
		if r := recover(); r != nil {
			a.log.Error("reaper panic", zap.Any("recover", r))
		}
	}()

	active, err := st.ListActiveJobReviewIDs(ctx)
	if err != nil {
		a.log.Warn("reaper: list active jobs", zap.Error(err))
		return
	}
	now := time.Now()
	ids, err := st.ListStaleReviewIDs(ctx, now.Add(-pendingGrace), now.Add(-runningGrace), active)
	if err != nil {
		a.log.Warn("reaper: list stale reviews", zap.Error(err))
		return
	}
	for _, id := range ids {
		rv, err := st.GetReview(ctx, id)
		if err != nil {
			continue
		}
		var reason string
		if rv.Status == "pending" {
			reason = "审查等待执行超时（超过 10 分钟未被 worker 认领，可能入队失败或服务曾重启），已自动标记为失败"
		} else {
			reason = "审查执行超时（超过 35 分钟未结束，进程可能已被中断），已自动标记为失败"
		}
		if err := st.MarkReviewFailed(ctx, id, reason); err != nil {
			a.log.Warn("reaper: mark review failed", zap.Int64("id", id), zap.Error(err))
			continue
		}
		// 写一行进度日志，让详情页能看到回收原因。
		_ = st.AppendReviewLog(ctx, id, "error", "[reaper] "+reason)
		a.log.Warn("reaper: reaped stale review",
			zap.Int64("id", id), zap.String("status", rv.Status),
			zap.String("commit", rv.CommitSHA))
	}
}
