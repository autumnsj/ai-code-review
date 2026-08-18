// Package analyzer 编排代码审查流程：拉取代码、调用 Pi Agent、解析报告、落库。
package analyzer

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/store"
)

// newPublicToken 生成 32 字节随机、URL 安全的公开报告 token（crypto/rand）。
func newPublicToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// PiAgent 抽象对 Pi Agent CLI 的调用，便于测试与替换。
type PiAgent interface {
	// Run 在工作目录执行审查，返回结构化报告。
	Run(ctx context.Context, in PiAgentInput) (*PiAgentReport, error)
}

// PiAgentInput 是传给 Pi Agent 的任务输入（序列化为 spec JSON）。
type PiAgentInput struct {
	RepoID      int64             `json:"repo_id"`
	CloneURL    string            `json:"clone_url"`
	AccessToken string            `json:"access_token,omitempty"`
	// SSHPrivateKey 是 SSH 凭据的私钥 PEM。不写入 input.json（json:"-"），
	// 由 CLI 写入仅当前用户可读的临时 key 文件后通过 GIT_SSH_COMMAND 注入给 git。
	SSHPrivateKey string          `json:"-"`
	CommitSHA     string          `json:"commit_sha"`
	BaseSHA       string          `json:"base_sha,omitempty"`
	TargetRef     string          `json:"target_ref,omitempty"`
	SourceRef     string          `json:"source_ref,omitempty"`
	PR            *PRInfo         `json:"pr,omitempty"`
	Config        *ReviewConfig   `json:"config,omitempty"`
	LLM           *domain.LLMConfig `json:"llm,omitempty"`
}

type PRInfo struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Author string `json:"author"`
}

// ReviewConfig 传给 Pi Agent 的审查配置（随 input.json）。
type ReviewConfig struct {
	Dimensions   []domain.DimensionSpec `json:"dimensions,omitempty"`
	IgnorePaths  []string               `json:"ignore_paths,omitempty"`
	CustomPrompt string                 `json:"custom_prompt,omitempty"`
}

// PiAgentReport 是 Pi Agent 输出的标准报告结构（JSON）。
type PiAgentReport struct {
	Summary     string            `json:"summary"`
	Dimensions  map[string]Dimension `json:"dimensions"`
	Findings    []ReportFinding   `json:"findings"`
	Strengths   []string          `json:"strengths,omitempty"`
	Risks       []string          `json:"risks,omitempty"`
	Stats       ReportStats       `json:"stats"`
	TokensUsed  int               `json:"tokens_used,omitempty"`
	Truncated   bool              `json:"truncated,omitempty"`
	// CommitAuthor 是被审 commit 的真正作者（取自 git 元数据），
	// 覆盖创建 review 时写入的 pusher/触发者，确保评分归属到写代码的人。
	CommitAuthor CommitAuthor `json:"commit_author"`
	// Participants 是 base..head 区间内的所有提交作者（去重）。
	Participants []CommitAuthor `json:"participants,omitempty"`
	// AuthorStats 按 email 汇总每位作者在区间内的增删行与改动文件数。
	AuthorStats map[string]AuthorDiffStat `json:"author_stats,omitempty"`
}

// CommitAuthor git commit 元数据中的作者。
type CommitAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// AuthorDiffStat 某位作者在审查区间内的改动量。
type AuthorDiffStat struct {
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
	FilesChanged int `json:"files_changed"`
}

type Dimension struct {
	Score     int    `json:"score"`
	Rationale string `json:"rationale"`
}

type ReportFinding struct {
	RuleID     string `json:"rule_id"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	FilePath   string `json:"file_path"`
	Line       int    `json:"line"`
	LineEnd    int    `json:"line_end,omitempty"`
	Title      string `json:"title"`
	Message    string `json:"message"`
	Snippet    string `json:"snippet,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Source     string `json:"source,omitempty"` // static | llm
	// Author 是 pi-agent 用 git blame 定位到的行作者 email（小写）；空表示无法归属。
	Author     string `json:"author,omitempty"`
	AuthorName string `json:"author_name,omitempty"`
}

type ReportStats struct {
	FilesChanged int `json:"files_changed"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
}

// Notifier 审查完成后推送通知。成功时按作者逐条发送（NotifyAuthorReview），
// 失败时发一条整体失败通知（NotifyReview）。
type Notifier interface {
	NotifyReview(ctx context.Context, r *domain.Review, findings []*domain.Finding)
	NotifyAuthorReview(ctx context.Context, r *domain.Review)
}

// Pipeline 持有审查执行所需依赖。
type Pipeline struct {
	store    *store.Store
	agent    PiAgent
	log      *zap.Logger
	dataDir  string
	notifier Notifier
}

func NewPipeline(st *store.Store, agent PiAgent, log *zap.Logger, dataDir string) *Pipeline {
	return &Pipeline{store: st, agent: agent, log: log, dataDir: dataDir}
}

// SetNotifier 注入通知器（避免循环依赖）。
func (p *Pipeline) SetNotifier(n Notifier) {
	p.notifier = n
}

// HandleJob 实现 queue.Handler，处理 review 类型任务。
func (p *Pipeline) HandleJob(ctx context.Context, job *domain.Job) error {
	var payload domain.ReviewPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}
	reviewID := payload.ReviewID
	if reviewID == 0 {
		return fmt.Errorf("payload missing review_id")
	}

	if err := p.store.SetReviewStatus(ctx, reviewID, "running"); err != nil {
		return fmt.Errorf("set running: %w", err)
	}

	// 把子进程逐行输出落库到 review_logs，供前端实时轮询查看进度。
	logLine := func(line string) {
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return
		}
		_ = p.store.AppendReviewLog(ctx, reviewID, levelFor(line), line)
	}
	ctx = withLogSink(ctx, logLine)
	shortSHA := payload.CommitSHA
	if len(shortSHA) > 10 {
		shortSHA = shortSHA[:10]
	}
	attempt := ""
	if job.MaxAttempts > 1 {
		attempt = fmt.Sprintf("（第 %d/%d 次尝试）", job.Attempts+1, job.MaxAttempts)
	}
	logLine(fmt.Sprintf("[review] 开始审查 commit %s%s", shortSHA, attempt))

	report, specs, runErr := p.run(ctx, payload)
	if runErr != nil {
		logLine("[review] 审查失败：" + runErr.Error())
		_ = p.store.FinishReview(ctx, reviewID, store.FinishReviewParams{
			Status: "failed",
			Error:  runErr.Error(),
		})
		// 队列会按 max_attempts 自动重试并退避；仅在最后一次尝试失败时通知，
		// 否则同一次逻辑失败会推送多条失败通知。
		if job.MaxAttempts == 0 || job.Attempts >= job.MaxAttempts {
			p.notifyFailure(ctx, reviewID)
		}
		return runErr
	}

	if err := p.persist(ctx, reviewID, specs, report); err != nil {
		logLine("[review] 结果落库失败：" + err.Error())
		return fmt.Errorf("persist report: %w", err)
	}
	logLine(fmt.Sprintf("[review] 审查完成，综合评分 %d，发现 %d 个问题",
		Score(report.Dimensions, specs, report.Findings), len(report.Findings)))
	p.notifySuccess(ctx, reviewID)
	return nil
}

func (p *Pipeline) run(ctx context.Context, payload domain.ReviewPayload) (*PiAgentReport, []domain.DimensionSpec, error) {
	repo, err := p.store.GetRepoByID(ctx, payload.RepoID)
	if err != nil {
		return nil, nil, err
	}
	var settings domain.LLMSettings
	if err := p.store.GetSetting(ctx, "llm", &settings); err != nil && !store.IsSettingNotFound(err) {
		return nil, nil, fmt.Errorf("load llm settings: %w", err)
	}
	llm, ok := settings.Default()
	if !ok {
		return nil, nil, fmt.Errorf("未配置默认模型：请在「设置 → AI 模型」中添加并选择一个默认模型")
	}

	// 读取全局打分维度（缺省用内置默认），随 input.json 传给 Pi Agent。
	var specs []domain.DimensionSpec
	if err := p.store.GetSetting(ctx, "score_dimensions", &specs); err != nil && !store.IsSettingNotFound(err) {
		return nil, nil, fmt.Errorf("load dimensions: %w", err)
	}
	if len(specs) == 0 {
		specs = domain.DefaultDimensions()
	}

	in := PiAgentInput{
		RepoID:      payload.RepoID,
		CloneURL:    repo.CloneURL,
		AccessToken: repo.AccessToken,
		CommitSHA:   payload.CommitSHA,
		BaseSHA:     payload.BaseSHA,
		TargetRef:   payload.TargetRef,
		SourceRef:   payload.SourceRef,
		LLM:         llm,
		Config: &ReviewConfig{
			Dimensions: specs,
		},
	}
	// 绑定了可复用凭据时，按类型覆盖内联 token 或注入 SSH 私钥。
	if repo.CredentialID > 0 {
		cred, err := p.store.GetCredential(ctx, repo.CredentialID)
		if err != nil {
			return nil, nil, fmt.Errorf("加载仓库凭据失败: %w", err)
		}
		switch cred.Type {
		case domain.CredentialHTTPSToken:
			in.AccessToken = cred.Secret
		case domain.CredentialSSH:
			in.SSHPrivateKey = cred.Secret
		}
	}
	if payload.PRNumber > 0 {
		in.PR = &PRInfo{
			Number: payload.PRNumber, Title: payload.PRTitle, URL: payload.PRURL, Author: payload.Author,
		}
	}
	report, err := p.agent.Run(ctx, in)
	return report, specs, err
}

// notifySuccess 审查成功后按作者逐条发送通知。
func (p *Pipeline) notifySuccess(ctx context.Context, reviewID int64) {
	if p.notifier == nil {
		return
	}
	rv, err := p.store.GetReview(ctx, reviewID)
	if err != nil {
		p.log.Warn("notify: load review", zap.Error(err))
		return
	}
	p.notifier.NotifyAuthorReview(ctx, rv)
}

// notifyFailure 审查失败后发送一条整体失败通知（不按作者拆分）。
func (p *Pipeline) notifyFailure(ctx context.Context, reviewID int64) {
	if p.notifier == nil {
		return
	}
	rv, err := p.store.GetReview(ctx, reviewID)
	if err != nil {
		p.log.Warn("notify: load review", zap.Error(err))
		return
	}
	p.notifier.NotifyReview(ctx, rv, nil)
}

func (p *Pipeline) persist(ctx context.Context, reviewID int64, specs []domain.DimensionSpec, r *PiAgentReport) error {
	total := Score(r.Dimensions, specs, r.Findings)
	statsJSON, _ := json.Marshal(r.Stats)

	// 组装带展示名的维度评分（落库 score_dimensions，使报告自包含）。
	labelByKey := make(map[string]string, len(specs))
	for _, s := range specs {
		labelByKey[s.Key] = s.Label
	}
	dimScores := make(map[string]domain.DimensionScore, len(r.Dimensions))
	for key, d := range r.Dimensions {
		label := labelByKey[key]
		if label == "" {
			label = key
		}
		dimScores[key] = domain.DimensionScore{Score: d.Score, Label: label, Rationale: d.Rationale}
	}
	dimJSON, _ := json.Marshal(dimScores)

	// 作者归属：用被审 commit 的 git 作者覆盖创建时写入的 pusher/触发者。
	// 以 email 作为稳定唯一键（便于按人聚合与成员备注映射），email 缺失回退 name。
	author := strings.TrimSpace(r.CommitAuthor.Email)
	if author == "" {
		author = strings.TrimSpace(r.CommitAuthor.Name)
	}

	// 旧四维列在命中默认维度时回填，保持既有统计/UI 兼容。
	var arch, qual, sec, maint int
	if d, ok := r.Dimensions["architecture"]; ok {
		arch = d.Score
	}
	if d, ok := r.Dimensions["quality"]; ok {
		qual = d.Score
	}
	if d, ok := r.Dimensions["security"]; ok {
		sec = d.Score
	}
	if d, ok := r.Dimensions["maintainability"]; ok {
		maint = d.Score
	}

	if err := p.store.FinishReview(ctx, reviewID, store.FinishReviewParams{
		Status:          "succeeded",
		Author:          author,
		Summary:         r.Summary,
		ScoreTotal:      total,
		ScoreArch:       arch,
		ScoreQuality:    qual,
		ScoreSecurity:   sec,
		ScoreMaint:      maint,
		ScoreDimensions: string(dimJSON),
		Stats:           string(statsJSON),
		DiffTruncated:   r.Truncated,
		TokensUsed:      r.TokensUsed,
		Additions:       r.Stats.Additions,
		Deletions:       r.Stats.Deletions,
		FilesChanged:    r.Stats.FilesChanged,
	}); err != nil {
		return err
	}

	// findings 带上 pi-agent 用 git blame 归属的作者 email（已小写）。
	findings := make([]*domain.Finding, 0, len(r.Findings))
	for _, f := range r.Findings {
		source := f.Source
		if source == "" {
			source = "llm"
		}
		lineEnd := f.LineEnd
		if lineEnd == 0 {
			lineEnd = f.Line
		}
		findings = append(findings, &domain.Finding{
			Source:     source,
			RuleID:     f.RuleID,
			Severity:   normalizeSeverity(f.Severity),
			Category:   f.Category,
			FilePath:   f.FilePath,
			LineStart:  f.Line,
			LineEnd:    lineEnd,
			Title:      f.Title,
			Message:    f.Message,
			Snippet:    f.Snippet,
			Suggestion: f.Suggestion,
			Confidence: f.Confidence,
			Author:     strings.ToLower(strings.TrimSpace(f.Author)),
		})
	}
	if err := p.store.InsertFindings(ctx, reviewID, findings); err != nil {
		return err
	}

	// 多作者拆分：为 base..head 区间内的每位参与者生成独立报告与评分。
	// blame 到区间外旧作者的 finding 只保留在全量报告里，不归入任何参与者。
	return p.persistAuthorReports(ctx, reviewID, specs, labelByKey, r, findings)
}

// persistAuthorReports 按参与者把 findings 分组，每位作者计算独立评分并写入
// review_author_reports；重新审查时先清空旧报告。通知以作者为单位逐条发送。
func (p *Pipeline) persistAuthorReports(
	ctx context.Context, reviewID int64, specs []domain.DimensionSpec,
	labelByKey map[string]string, r *PiAgentReport, findings []*domain.Finding,
) error {
	if err := p.store.DeleteAuthorReports(ctx, reviewID); err != nil {
		return err
	}

	// 参与者集合（email 小写）。blame 作者不在其中的 finding 不拆给任何人。
	participants := make(map[string]string, len(r.Participants)) // email -> name
	for _, pa := range r.Participants {
		email := strings.ToLower(strings.TrimSpace(pa.Email))
		if email == "" {
			continue
		}
		if _, ok := participants[email]; !ok {
			participants[email] = pa.Name
		}
	}
	if len(participants) == 0 {
		return nil // 区间读不到参与者（浅克隆等），跳过拆分，全量报告仍可用。
	}

	// 按作者分桶 findings。
	byAuthor := make(map[string][]*domain.Finding, len(participants))
	for _, f := range findings {
		if f.Author == "" {
			continue
		}
		if _, ok := participants[f.Author]; !ok {
			continue // 区间外旧代码作者，不归属本次参与者
		}
		byAuthor[f.Author] = append(byAuthor[f.Author], f)
	}

	// 稳定顺序：按 email 排序，便于测试与通知顺序一致。
	emails := make([]string, 0, len(participants))
	for e := range participants {
		emails = append(emails, e)
	}
	sort.Strings(emails)

	for _, email := range emails {
		authorFindings := byAuthor[email]
		// 把 domain.Finding 转回 scorer 需要的 ReportFinding 视图，复用评分逻辑。
		rf := make([]ReportFinding, 0, len(authorFindings))
		var counts [5]int // critical,high,medium,low,info
		for _, f := range authorFindings {
			rf = append(rf, ReportFinding{
				Severity: f.Severity, Category: f.Category,
			})
			switch f.Severity {
			case "critical":
				counts[0]++
			case "high":
				counts[1]++
			case "medium":
				counts[2]++
			case "low":
				counts[3]++
			default:
				counts[4]++
			}
		}

		scoreTotal := Score(r.Dimensions, specs, rf)
		// 维度扣分后的得分（与 scorer 内部一致地展示四维），这里用全量 dims 作基线，
		// 再扣减该作者问题，得到该作者各维度分。
		dimScores := authorDimensionScores(r.Dimensions, specs, rf, labelByKey)
		dimJSON, _ := json.Marshal(dimScores)

		var arch, qual, sec, maint int
		if d, ok := dimScores["architecture"]; ok {
			arch = d.Score
		}
		if d, ok := dimScores["quality"]; ok {
			qual = d.Score
		}
		if d, ok := dimScores["security"]; ok {
			sec = d.Score
		}
		if d, ok := dimScores["maintainability"]; ok {
			maint = d.Score
		}

		st := r.AuthorStats[email]
		if err := p.store.UpsertAuthorReport(ctx, store.UpsertAuthorReportParams{
			ReviewID:        reviewID,
			Author:          email,
			AuthorName:      participants[email],
			PublicToken:     newPublicToken(),
			Summary:         r.Summary,
			ScoreTotal:      scoreTotal,
			ScoreArch:       arch,
			ScoreQuality:    qual,
			ScoreSecurity:   sec,
			ScoreMaint:      maint,
			ScoreDimensions: string(dimJSON),
			FindingsCount:   len(authorFindings),
			CriticalCount:   counts[0],
			HighCount:       counts[1],
			MediumCount:     counts[2],
			LowCount:        counts[3],
			InfoCount:       counts[4],
			Additions:       st.Additions,
			Deletions:       st.Deletions,
			FilesChanged:    st.FilesChanged,
		}); err != nil {
			return err
		}
	}
	return nil
}

// authorDimensionScores 在 agent 给出的整体维度原始分基础上，只对某作者的 findings
// 扣分，得到该作者的维度评分（与 scorer.Score 的扣分项保持一致）。
func authorDimensionScores(dims map[string]Dimension, specs []domain.DimensionSpec, findings []ReportFinding, labelByKey map[string]string) map[string]domain.DimensionScore {
	scores := make(map[string]int, len(specs))
	for _, s := range specs {
		if d, ok := dims[s.Key]; ok {
			scores[s.Key] = d.Score
		}
	}
	for _, f := range findings {
		if _, ok := scores[f.Category]; !ok {
			continue
		}
		switch f.Severity {
		case "critical":
			scores[f.Category] -= 20
			if f.Category != "security" {
				if _, ok := scores["security"]; ok {
					scores["security"] -= 10
				}
			}
		case "high":
			scores[f.Category] -= 10
		case "medium":
			scores[f.Category] -= 5
		case "low":
			scores[f.Category] -= 2
		}
	}
	out := make(map[string]domain.DimensionScore, len(scores))
	for key, v := range scores {
		label := labelByKey[key]
		if label == "" {
			label = key
		}
		out[key] = domain.DimensionScore{Score: clampScore(v), Label: label}
	}
	return out
}

func normalizeSeverity(s string) string {
	switch s {
	case "critical", "high", "medium", "low", "info":
		return s
	default:
		return "medium"
	}
}
