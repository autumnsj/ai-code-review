// Package report 生成并推送定时日报/周报。
//
// 报告聚焦「这个周期审了什么、发现了什么问题」：审查记录清单（仓库/PR 或分支/
// 提交/作者/评分）与 critical/high 重点问题清单，不做代码行数/排行榜统计。
// 触发方式有两种：定时调度（cmd/server/reportcron.go，幂等键按周期去重）
// 与管理端「立即发送」（manual，幂等键按时间戳，允许重复触发）。
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
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

	maxReviews  = 15 // 审查记录清单条数上限（企微 markdown 约 4KB，需控总长）
	maxFindings = 20 // 重点问题清单条数上限
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
	rvs, err := s.st.ListReviewsInRange(ctx, p.PeriodStart, p.PeriodEnd, maxReviews)
	if err != nil {
		return "", "", fmt.Errorf("load reviews in range: %w", err)
	}
	fs, err := s.st.ListTopFindingsInRange(ctx, p.PeriodStart, p.PeriodEnd, maxFindings)
	if err != nil {
		return "", "", fmt.Errorf("load findings in range: %w", err)
	}

	names := authorNamer{st: s.st}
	// 失败审查可能停留在入队时的占位作者 "admin"（真实作者要审查完成才回填），展示为 —。
	authorOf := func(key string) string {
		if strings.EqualFold(strings.TrimSpace(key), "admin") {
			return "—"
		}
		return names.name(ctx, key)
	}
	reviews := make([]notifier.ReportReview, 0, len(rvs))
	for _, r := range rvs {
		reviews = append(reviews, notifier.ReportReview{
			Repo:   r.RepoName,
			Title:  firstNonEmpty(r.PRTitle, commitSubject(r.Stats)),
			Desc:   firstSentence(r.Summary),
			Ref:    r.TargetRef,
			Commit: r.CommitSHA,
			Author: authorOf(r.Author),
			Score:  r.ScoreTotal,
			Status: r.Status,
			Error:  r.Error,
		})
	}
	findings := make([]notifier.ReportFinding, 0, len(fs))
	for _, f := range fs {
		findings = append(findings, notifier.ReportFinding{
			Repo:     f.RepoName,
			Severity: f.Severity,
			Location: shortLocation(f.FilePath, f.LineStart),
			Title:    f.Title,
			Author:   names.name(ctx, f.Author),
		})
	}

	loc := Location()
	md := notifier.BuildReportMarkdown(p.Kind, p.PeriodStart.In(loc), p.PeriodEnd.In(loc),
		notifier.ReportTotals{
			ReviewCount: totals.ReviewCount,
			Succeeded:   totals.Succeeded,
			Failed:      totals.Failed,
			Critical:    totals.Critical,
			High:        totals.High,
		},
		reviews, findings)

	title := "团队工作日报"
	if p.Kind == KindWeekly {
		title = "团队工作周报"
	}
	if p.Manual {
		title += "（手动发送）"
	}
	return title, md, nil
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

// authorNamer 把作者归属键（小写 email/login）解析为展示名，单次报告内缓存：
// 有成员备注用备注真名；否则用 email 前缀（@ 之前部分），避免报告里一长串邮箱。
type authorNamer struct {
	st    *store.Store
	cache map[string]string
}

func (n *authorNamer) name(ctx context.Context, key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	if n.cache != nil {
		if v, ok := n.cache[key]; ok {
			return v
		}
	}
	out := key
	if a, err := n.st.GetAuthorByLogin(ctx, key); err == nil {
		if dn := strings.TrimSpace(a.DisplayName); dn != "" {
			out = dn
		}
	} else if i := strings.Index(key, "@"); i > 0 {
		out = key[:i]
	}
	if n.cache == nil {
		n.cache = make(map[string]string)
	}
	n.cache[key] = out
	return out
}

// firstNonEmpty 返回第一个去空白后非空的字符串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// commitSubject 从 review 的 stats JSON 中取 head 提交标题（无 PR 时作为功能描述）。
func commitSubject(statsJSON string) string {
	if strings.TrimSpace(statsJSON) == "" {
		return ""
	}
	var st struct {
		CommitSubject string `json:"commit_subject"`
	}
	if err := json.Unmarshal([]byte(statsJSON), &st); err != nil {
		return ""
	}
	return strings.TrimSpace(st.CommitSubject)
}

// firstSentence 取摘要的第一句（按中英文句读切分），作为工作日报的功能概述。
func firstSentence(summary string) string {
	s := strings.TrimSpace(summary)
	if s == "" {
		return ""
	}
	for i, r := range s {
		if r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?' || r == '\n' {
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}

// shortLocation 把文件路径截短为最后两段 + 行号（a/b/c.go:12 → b/c.go:12），
// 控制卡片长度；无路径分隔符时原样返回。
func shortLocation(file string, line int) string {
	file = strings.TrimSpace(file)
	if file == "" {
		return "-"
	}
	dir, base := path.Split(file)
	if dir != "" {
		if parent := path.Base(strings.TrimRight(dir, "/")); parent != "" && parent != "." {
			base = parent + "/" + base
		}
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d", base, line)
	}
	return base
}
