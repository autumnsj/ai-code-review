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
	"sort"
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

// personReport 一位成员在本周期内的考核数据（一人一份报告）。
type personReport struct {
	name     string
	features []notifier.ReportFeature // AI 看代码判断的功能/工作项（老审查回退一条审查标题）
	findings []notifier.ReportFinding
}

// reportData 一个周期聚合出的全部报告素材：按人分组的个人报告 + 团队级异常。
type reportData struct {
	persons []personReport // 按展示名排序
	failed  []notifier.ReportReview
	orphans []notifier.ReportFinding // 无法 blame 到具体成员的重点问题
	hasAny  bool                     // 窗口内是否有任何审查（含失败）
}

// HandleJob 满足 queue.Handler：解析 payload → 按人聚合 → 逐人落库并推送。
// 报告以人为单位（考核个人）：每位有产出的成员一条独立消息、一条独立记录；
// 单人落库/推送失败只记日志不影响其他人，job 整体成功，避免重试导致已发送的人重复收到
// （(job_id,author) 唯一约束兜底）。
func (s *Service) HandleJob(ctx context.Context, job *domain.Job) error {
	var p JobPayload
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return fmt.Errorf("parse report payload: %w", err)
	}
	data, err := s.aggregate(ctx, p)
	if err != nil {
		return err
	}
	loc := Location()
	start, end := p.PeriodStart.In(loc), p.PeriodEnd.In(loc)
	word := "日报"
	if p.Kind == KindWeekly {
		word = "周报"
	}
	suffix := ""
	if p.Manual {
		suffix = "（手动发送）"
	}
	trigger := "scheduled"
	if p.Manual {
		trigger = "manual"
	}

	persistAndSend := func(author, title, md string) {
		if err := s.st.CreateReport(ctx, store.CreateReportInput{
			Kind: p.Kind, TriggerType: trigger, Author: author,
			PeriodStart: p.PeriodStart, PeriodEnd: p.PeriodEnd,
			Title: title, Content: md, JobID: job.ID,
		}); err != nil {
			s.log.Warn("report: persist", zap.String("author", author), zap.Error(err))
			return
		}
		s.dispatcher.SendMarkdown(ctx, title, md)
	}

	if !data.hasAny {
		md := notifier.BuildTeamNoticeMarkdown(p.Kind, start, end, nil, nil, false)
		persistAndSend("", "团队工作"+word+suffix, md)
		s.log.Info("report sent (empty)", zap.String("kind", p.Kind))
		return nil
	}

	for _, person := range data.persons {
		md := notifier.BuildPersonalReportMarkdown(p.Kind, start, end, person.name, person.features, person.findings)
		persistAndSend(person.name, person.name+" · 工作"+word+suffix, md)
	}
	if notice := notifier.BuildTeamNoticeMarkdown(p.Kind, start, end, data.failed, data.orphans, true); notice != "" {
		persistAndSend("", "工作"+word+" · 异常提醒"+suffix, notice)
	}
	s.log.Info("report sent", zap.String("kind", p.Kind), zap.Int("persons", len(data.persons)))
	return nil
}

// Build 管理端预览：把全员个人报告拼接成一份 markdown（实际推送为每人一条独立消息），不落库。
func (s *Service) Build(ctx context.Context, kind string, now time.Time) (title, markdown string, start, end time.Time, err error) {
	start, end = Period(kind, now)
	p := JobPayload{Kind: kind, PeriodStart: start, PeriodEnd: end, Manual: true}
	data, err := s.aggregate(ctx, p)
	if err != nil {
		return
	}
	loc := Location()
	st, en := start.In(loc), end.In(loc)
	word := "日报"
	if kind == KindWeekly {
		word = "周报"
	}
	var parts []string
	if !data.hasAny {
		parts = append(parts, notifier.BuildTeamNoticeMarkdown(kind, st, en, nil, nil, false))
	} else {
		for _, person := range data.persons {
			parts = append(parts, notifier.BuildPersonalReportMarkdown(kind, st, en, person.name, person.features, person.findings))
		}
		if notice := notifier.BuildTeamNoticeMarkdown(kind, st, en, data.failed, data.orphans, true); notice != "" {
			parts = append(parts, notice)
		}
	}
	title = "团队工作" + word + "（预览）"
	markdown = "> 预览为全员拼接；实际推送时每位成员各收到一条独立的个人报告。\n\n" + strings.Join(parts, "\n---\n")
	return
}

// aggregate 加载窗口内的审查与重点问题，按成员（小写 email）分组归属。
func (s *Service) aggregate(ctx context.Context, p JobPayload) (*reportData, error) {
	rvs, err := s.st.ListReviewsInRange(ctx, p.PeriodStart, p.PeriodEnd, maxReviews)
	if err != nil {
		return nil, fmt.Errorf("load reviews in range: %w", err)
	}
	fs, err := s.st.ListTopFindingsInRange(ctx, p.PeriodStart, p.PeriodEnd, maxFindings)
	if err != nil {
		return nil, fmt.Errorf("load findings in range: %w", err)
	}
	names := authorNamer{st: s.st}

	data := &reportData{hasAny: len(rvs) > 0}
	personIdx := map[string]int{} // 作者 key（小写 email）→ persons 下标
	// 可归属的成功审查（key → review），稍后按 AI 功能清单分发到人。
	type successReview struct {
		key string
		rv  *domain.Review
	}
	var succeeded []successReview
	for _, r := range rvs {
		if r.Status != "succeeded" {
			data.failed = append(data.failed, notifier.ReportReview{
				Repo:   r.RepoName,
				Title:  r.FeatureTitle(),
				Ref:    r.TargetRef,
				Commit: r.CommitSHA,
				Score:  r.ScoreTotal,
				Status: r.Status,
				Error:  r.Error,
			})
			continue
		}
		key := strings.ToLower(strings.TrimSpace(r.Author))
		// 占位 admin/空作者无法归属到具体成员，不进个人考核报告（平台审查记录可查）。
		if key == "" || strings.EqualFold(key, "admin") {
			continue
		}
		if _, ok := personIdx[key]; !ok {
			personIdx[key] = len(data.persons)
			data.persons = append(data.persons, personReport{name: names.name(ctx, key)})
		}
		succeeded = append(succeeded, successReview{key: key, rv: r})
	}
	// 功能数以代码为准：用 AI 看代码判断的 features 清单，按 AI 标注的 author 归属到人；
	// AI 未标注或标注的人不在本周期人员中时，归给该审查的提交作者。
	// 老审查（stats 里无 features）回退为一条「审查 = 一个功能」，标题取 PR 标题/commit 标题。
	for _, sr := range succeeded {
		r := sr.rv
		feats := r.Features()
		if len(feats) == 0 {
			feats = []domain.ReviewFeature{{Title: r.FeatureTitle(), Detail: firstSentence(r.Summary)}}
		}
		for _, f := range feats {
			key := strings.ToLower(strings.TrimSpace(f.Author))
			if _, ok := personIdx[key]; !ok {
				key = sr.key
			}
			idx := personIdx[key]
			data.persons[idx].features = append(data.persons[idx].features, notifier.ReportFeature{
				Repo:   r.RepoName,
				Title:  f.Title,
				Detail: f.Detail,
				Score:  r.ScoreTotal,
			})
		}
	}
	for _, f := range fs {
		ff := notifier.ReportFinding{
			Repo:     f.RepoName,
			Severity: f.Severity,
			Location: shortLocation(f.FilePath, f.LineStart),
			Title:    f.Title,
		}
		key := strings.ToLower(strings.TrimSpace(f.Author))
		if key != "" {
			if idx, ok := personIdx[key]; ok {
				data.persons[idx].findings = append(data.persons[idx].findings, ff)
				continue
			}
		}
		data.orphans = append(data.orphans, ff)
	}
	sort.Slice(data.persons, func(i, j int) bool { return data.persons[i].name < data.persons[j].name })
	return data, nil
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
