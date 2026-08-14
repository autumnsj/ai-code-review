// Package analyzer 编排代码审查流程：拉取代码、调用 Pi Agent、解析报告、落库。
package analyzer

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/store"
)

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

// ReviewConfig 仓库级审查配置（简化，阶段9补 review_configs 读取）。
type ReviewConfig struct {
	IgnorePaths  []string           `json:"ignore_paths,omitempty"`
	CustomPrompt string             `json:"custom_prompt,omitempty"`
}

// PiAgentReport 是 Pi Agent 输出的标准报告结构（JSON）。
type PiAgentReport struct {
	Summary     string            `json:"summary"`
	Dimensions  Dimensions        `json:"dimensions"`
	Findings    []ReportFinding   `json:"findings"`
	Strengths   []string          `json:"strengths,omitempty"`
	Risks       []string          `json:"risks,omitempty"`
	Stats       ReportStats       `json:"stats"`
	TokensUsed  int               `json:"tokens_used,omitempty"`
	Truncated   bool              `json:"truncated,omitempty"`
}

type Dimensions struct {
	Architecture  Dimension `json:"architecture"`
	Quality       Dimension `json:"quality"`
	Security      Dimension `json:"security"`
	Maintainability Dimension `json:"maintainability"`
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
}

type ReportStats struct {
	FilesChanged int `json:"files_changed"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
}

// Notifier 审查完成后推送通知。
type Notifier interface {
	NotifyReview(ctx context.Context, r *domain.Review, findings []*domain.Finding)
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

	report, runErr := p.run(ctx, payload)
	if runErr != nil {
		_ = p.store.FinishReview(ctx, reviewID, store.FinishReviewParams{
			Status: "failed",
			Error:  runErr.Error(),
		})
		p.notify(ctx, reviewID)
		return runErr
	}

	if err := p.persist(ctx, reviewID, report); err != nil {
		return fmt.Errorf("persist report: %w", err)
	}
	p.notify(ctx, reviewID)
	return nil
}

func (p *Pipeline) run(ctx context.Context, payload domain.ReviewPayload) (*PiAgentReport, error) {
	repo, err := p.store.GetRepoByID(ctx, payload.RepoID)
	if err != nil {
		return nil, err
	}
	var settings domain.LLMSettings
	if err := p.store.GetSetting(ctx, "llm", &settings); err != nil && !store.IsSettingNotFound(err) {
		return nil, fmt.Errorf("load llm settings: %w", err)
	}
	llm, ok := settings.Default()
	if !ok {
		return nil, fmt.Errorf("未配置默认模型：请在「设置 → AI 模型」中添加并选择一个默认模型")
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
	}
	// 绑定了可复用凭据时，按类型覆盖内联 token 或注入 SSH 私钥。
	if repo.CredentialID > 0 {
		cred, err := p.store.GetCredential(ctx, repo.CredentialID)
		if err != nil {
			return nil, fmt.Errorf("加载仓库凭据失败: %w", err)
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
	return p.agent.Run(ctx, in)
}

func (p *Pipeline) notify(ctx context.Context, reviewID int64) {
	if p.notifier == nil {
		return
	}
	rv, err := p.store.GetReview(ctx, reviewID)
	if err != nil {
		p.log.Warn("notify: load review", zap.Error(err))
		return
	}
	var findings []*domain.Finding
	if rv.Status == "succeeded" {
		findings, err = p.store.ListFindings(ctx, reviewID)
		if err != nil {
			p.log.Warn("notify: load findings", zap.Error(err))
		}
	}
	p.notifier.NotifyReview(ctx, rv, findings)
}

func (p *Pipeline) persist(ctx context.Context, reviewID int64, r *PiAgentReport) error {
	total := Score(r.Dimensions, r.Findings)
	statsJSON, _ := json.Marshal(r.Stats)
	if err := p.store.FinishReview(ctx, reviewID, store.FinishReviewParams{
		Status:        "succeeded",
		Summary:       r.Summary,
		ScoreTotal:    total,
		ScoreArch:     r.Dimensions.Architecture.Score,
		ScoreQuality:  r.Dimensions.Quality.Score,
		ScoreSecurity: r.Dimensions.Security.Score,
		ScoreMaint:    r.Dimensions.Maintainability.Score,
		Stats:         string(statsJSON),
		DiffTruncated: r.Truncated,
		TokensUsed:    r.TokensUsed,
		Additions:     r.Stats.Additions,
		Deletions:     r.Stats.Deletions,
		FilesChanged:  r.Stats.FilesChanged,
	}); err != nil {
		return err
	}
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
		})
	}
	return p.store.InsertFindings(ctx, reviewID, findings)
}

func normalizeSeverity(s string) string {
	switch s {
	case "critical", "high", "medium", "low", "info":
		return s
	default:
		return "medium"
	}
}
