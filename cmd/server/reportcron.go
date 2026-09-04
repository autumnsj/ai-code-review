package main

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/ai-code-review/aicr/internal/queue"
	"github.com/ai-code-review/aicr/internal/report"
)

const reportCronEvery = time.Minute

// startReportCron 启动定时报告 goroutine：每分钟检查一次日报/周报调度配置，
// 命中北京时间设定时刻（周报还需匹配星期）就入队一条 report 任务。
// 幂等键按统计周期去重，同一分钟内的重复 tick 或服务重启都不会重复发送；
// 错过整点不补发（服务当时没在跑就跳过，手动可在设置页立即发送）。
func (a *application) startReportCron(ctx context.Context, sched *queue.Scheduler, svc *report.Service) {
	go func() {
		// 启动后稍等，避开引导/建库高峰。
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
		}
		// 立即检查一次：若启动恰好跨过配置的分钟（ticker 首次触发在一分钟后），
		// 也能在当前分钟内补入队；幂等键保证不会与后续 tick 重复。
		a.reportCronOnce(ctx, sched, svc)
		ticker := time.NewTicker(reportCronEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.reportCronOnce(ctx, sched, svc)
			}
		}
	}()
}

func (a *application) reportCronOnce(ctx context.Context, sched *queue.Scheduler, svc *report.Service) {
	defer func() {
		if r := recover(); r != nil {
			a.log.Error("report cron panic", zap.Any("recover", r))
		}
	}()

	cfg := svc.Config(ctx)
	if !cfg.Daily.Enabled && !cfg.Weekly.Enabled {
		return
	}
	now := time.Now()
	n := now.In(report.Location())
	hhmm := n.Format("15:04")
	// time.Weekday：Sunday=0…Saturday=6；配置用 1=周一…7=周日。
	weekday := int(n.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	if cfg.Daily.Enabled && cfg.Daily.SendAt == hhmm {
		a.enqueueReport(ctx, sched, report.KindDaily, now)
	}
	if cfg.Weekly.Enabled && cfg.Weekly.SendAt == hhmm && cfg.Weekly.Weekday == weekday {
		a.enqueueReport(ctx, sched, report.KindWeekly, now)
	}
}

func (a *application) enqueueReport(ctx context.Context, sched *queue.Scheduler, kind string, now time.Time) {
	payload, idemKey := report.PayloadFor(kind, now, false)
	id, err := sched.Enqueue(ctx, "report", payload, idemKey)
	if err != nil {
		a.log.Warn("report cron: enqueue", zap.String("kind", kind), zap.Error(err))
		return
	}
	if id != 0 {
		a.log.Info("report cron: scheduled", zap.String("kind", kind), zap.Int64("job_id", id))
	}
}
