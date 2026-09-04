// Package report 生成并推送定时日报/周报。
//
// 报告本身是模板聚合（不调 LLM）：从 reviews / review_author_reports 汇总
// 统计窗口内的审查次数、平均分、改动量、问题数与作者榜单，经通知渠道广播。
// 触发方式有两种：定时调度（cmd/server/reportcron.go，幂等键按周期去重）
// 与管理端「立即发送」（manual，幂等键按时间戳，允许重复触发）。
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/notifier"
	"github.com/ai-code-review/aicr/internal/store"
)

const (
	KindDaily  = "daily"
	KindWeekly = "weekly"

	// SettingsKey 定时调度配置在 settings 表的键。
	SettingsKey = "report_schedules"

	topN = 5 // 每个榜单展示的作者数
)

// JobPayload 是 report 类型 job 的 payload。
// 统计窗口在入队时就固定下来，worker 延迟执行也不会漂移周期。
type JobPayload struct {
	Kind        string    `json:"kind"` // daily | weekly
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	Manual      bool      `json:"manual,omitempty"`
}

// Service 组装报告内容并推送。
type Service struct {
	st         *store.Store
	dispatcher *notifier.Dispatcher
	log        *zap.Logger
}

func NewService(st *store.Store, d *notifier.Dispatcher, log *zap.Logger) *Service {
	return &Service{st: st, dispatcher: d, log: log}
}

// HandleJob 满足 queue.Handler：解析 payload → 聚合 → 推送。
// 渠道级失败只记日志（Dispatcher 内部处理），job 始终成功——
// 推送失败不应触发队列重试导致群里刷屏。
func (s *Service) HandleJob(ctx context.Context, job *domain.Job) error {
	var p JobPayload
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return fmt.Errorf("parse report payload: %w", err)
	}
	title, md, err := s.build(ctx, p)
	if err != nil {
		return err
	}
	s.dispatcher.SendMarkdown(ctx, title, md)
	s.log.Info("report sent", zap.String("kind", p.Kind),
		zap.Time("period_start", p.PeriodStart), zap.Time("period_end", p.PeriodEnd),
		zap.Bool("manual", p.Manual))
	return nil
}

// Build 按类型与当前时间计算窗口并生成报告（管理端预览用）。
func (s *Service) Build(ctx context.Context, kind string, now time.Time) (title, markdown string, start, end time.Time, err error) {
	start, end = Period(kind, now)
	p := JobPayload{Kind: kind, PeriodStart: start, PeriodEnd: end, Manual: true}
	title, markdown, err = s.build(ctx, p)
	return
}

func (s *Service) build(ctx context.Context, p JobPayload) (string, string, error) {
	totals, err := s.st.ReportRangeStats(ctx, p.PeriodStart, p.PeriodEnd)
	if err != nil {
		return "", "", fmt.Errorf("load report totals: %w", err)
	}
	topScore, err := s.topAuthors(ctx, p, "avg_total")
	if err != nil {
		return "", "", err
	}
	topChurn, err := s.topAuthors(ctx, p, "churn")
	if err != nil {
		return "", "", err
	}
	topFindings, err := s.topAuthors(ctx, p, "findings_total")
	if err != nil {
		return "", "", err
	}

	loc := Location()
	md := notifier.BuildReportMarkdown(p.Kind, p.PeriodStart.In(loc), p.PeriodEnd.In(loc),
		notifier.ReportTotals{
			ReviewCount:  totals.ReviewCount,
			Succeeded:    totals.Succeeded,
			Failed:       totals.Failed,
			AvgScore:     totals.AvgScore,
			Additions:    totals.Additions,
			Deletions:    totals.Deletions,
			FilesChanged: totals.FilesChanged,
			Critical:     totals.Critical,
			High:         totals.High,
			Medium:       totals.Medium,
			Low:          totals.Low,
		},
		topScore, topChurn, topFindings)

	title := "代码审查日报"
	if p.Kind == KindWeekly {
		title = "代码审查周报"
	}
	if p.Manual {
		title += "（手动发送）"
	}
	return title, md, nil
}

// topAuthors 取窗口内某一维度的作者榜单。
func (s *Service) topAuthors(ctx context.Context, p JobPayload, sort string) ([]notifier.ReportAuthor, error) {
	since, until := p.PeriodStart, p.PeriodEnd
	rows, err := s.st.ListAuthorStats(ctx, store.AuthorFilter{
		Since: &since,
		Until: &until,
		Sort:  sort,
		Limit: topN,
	})
	if err != nil {
		return nil, fmt.Errorf("load author ranking %s: %w", sort, err)
	}
	out := make([]notifier.ReportAuthor, 0, len(rows))
	for _, a := range rows {
		out = append(out, notifier.ReportAuthor{
			Name:      authorLabel(a.Author, a.DisplayName),
			Reviews:   a.ReviewCount,
			AvgScore:  a.AvgTotal,
			Additions: a.Additions,
			Deletions: a.Deletions,
			Findings:  a.FindingsTotal,
			Critical:  a.Critical,
			High:      a.High,
		})
	}
	return out, nil
}

// Config 读取调度配置；缺省/损坏时返回归一化默认（默认全关）。
func (s *Service) Config(ctx context.Context) domain.ReportScheduleConfig {
	var cfg domain.ReportScheduleConfig
	if err := s.st.GetSetting(ctx, SettingsKey, &cfg); err != nil {
		if !store.IsSettingNotFound(err) {
			s.log.Warn("report: load schedule config", zap.Error(err))
		}
	}
	return cfg.Normalize()
}

// SaveConfig 保存调度配置（写入前归一化）。
func (s *Service) SaveConfig(ctx context.Context, cfg domain.ReportScheduleConfig) error {
	return s.st.SetSetting(ctx, SettingsKey, cfg.Normalize())
}

// Location 返回报告时区：Asia/Shanghai，加载失败回退进程本地时区。
func Location() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Local
	}
	return loc
}

// Period 返回某类报告在 now 时刻对应的统计窗口 [start, end)：
// daily=[昨日 00:00, 今日 00:00)，weekly=[7 天前 00:00, 今日 00:00)，
// 均按北京时间对齐到自然日。
func Period(kind string, now time.Time) (start, end time.Time) {
	loc := Location()
	n := now.In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	if kind == KindWeekly {
		return today.AddDate(0, 0, -7), today
	}
	return today.AddDate(0, 0, -1), today
}

// PayloadFor 构造入队载荷与幂等键。
// 定时触发：键按周期起始日（report:daily:2026-09-04），重复 tick/重启不重复入队；
// 手动触发：键带纳秒时间戳，允许重复发送。
func PayloadFor(kind string, now time.Time, manual bool) (payload any, idempotencyKey string) {
	start, end := Period(kind, now)
	p := JobPayload{Kind: kind, PeriodStart: start, PeriodEnd: end, Manual: manual}
	if manual {
		return p, fmt.Sprintf("report:manual:%s:%d", kind, now.UnixNano())
	}
	return p, "report:" + kind + ":" + start.Format("2006-01-02")
}

// authorLabel 榜单作者展示名：有备注显示「真名（账号）」，否则裸账号。
func authorLabel(account, displayName string) string {
	displayName = strings.TrimSpace(displayName)
	if displayName != "" {
		return fmt.Sprintf("%s（%s）", displayName, account)
	}
	return account
}
