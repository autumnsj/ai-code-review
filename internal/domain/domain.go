// Package domain 定义纯领域模型，不依赖任何外部包。
package domain

import "time"

// Provider 表示 Git 平台类型。
type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderGitLab Provider = "gitlab"
	ProviderGitee  Provider = "gitee"
	ProviderCoding Provider = "coding"
)

// Repo 是一个被接入审查的代码仓库。
type Repo struct {
	ID            int64     `json:"id"`
	Provider      Provider  `json:"provider"`
	CloneURL      string    `json:"clone_url"`
	WebURL        string    `json:"web_url"`
	Name          string    `json:"name"`
	DefaultBranch string    `json:"default_branch"`
	AccessToken   string    `json:"access_token,omitempty"` // 私有仓库 clone 凭证，不对外返回
	HookToken     string    `json:"hook_token,omitempty"`   // hookUrl 路径段，创建/重置时返回
	HookSecret    string    `json:"hook_secret,omitempty"`  // 平台签名校验 secret，可选
	Status        string    `json:"status"`                 // active | disabled
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Review 一次代码审查记录。
type Review struct {
	ID            int64     `json:"id"`
	RepoID        int64     `json:"repo_id"`
	RepoName      string    `json:"repo_name,omitempty"`
	PublicToken   string    `json:"public_token"`
	CommitSHA     string    `json:"commit_sha"`
	BaseSHA       string    `json:"base_sha,omitempty"`
	TargetRef     string    `json:"target_ref,omitempty"`
	SourceRef     string    `json:"source_ref,omitempty"`
	PRNumber      int       `json:"pr_number,omitempty"`
	PRTitle       string    `json:"pr_title,omitempty"`
	PRURL         string    `json:"pr_url,omitempty"`
	Author        string    `json:"author,omitempty"`
	EventType     string    `json:"event_type"` // push | pull_request | merge_request
	Status        string    `json:"status"`     // pending | running | succeeded | failed
	Summary       string    `json:"summary,omitempty"`
	ScoreTotal    int       `json:"score_total"`
	ScoreArch     int       `json:"score_arch"`
	ScoreQuality  int       `json:"score_quality"`
	ScoreSecurity int       `json:"score_security"`
	ScoreMaint    int       `json:"score_maint"`
	Stats         string    `json:"stats,omitempty"` // JSON: files_changed, additions, deletions...
	DiffTruncated bool      `json:"diff_truncated"`
	Error         string    `json:"error,omitempty"`
	TokensUsed    int       `json:"tokens_used"`
	Additions     int       `json:"additions"`
	Deletions     int       `json:"deletions"`
	FilesChanged  int       `json:"files_changed"`
	TriggeredAt   time.Time `json:"triggered_at"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// Finding 一条具体问题。
type Finding struct {
	ID         int64     `json:"id"`
	ReviewID   int64     `json:"review_id"`
	Source     string    `json:"source"` // static | llm
	RuleID     string    `json:"rule_id"`
	Severity   string    `json:"severity"` // critical | high | medium | low | info
	Category   string    `json:"category"` // security | quality | architecture | maintainability | style
	FilePath   string    `json:"file_path"`
	LineStart  int       `json:"line_start"`
	LineEnd    int       `json:"line_end"`
	Title      string    `json:"title"`
	Message    string    `json:"message"`
	Snippet    string    `json:"snippet,omitempty"`
	Suggestion string    `json:"suggestion,omitempty"`
	Confidence string    `json:"confidence,omitempty"` // high | medium | low
	CreatedAt  time.Time `json:"created_at"`
}

// Job 异步任务。
type Job struct {
	ID             int64     `json:"id"`
	Kind           string    `json:"kind"` // review
	Payload        string    `json:"payload"`
	Status         string    `json:"status"` // pending | running | succeeded | failed | dead
	LeaseOwner     string    `json:"lease_owner,omitempty"`
	LeaseUntil     *time.Time `json:"lease_until,omitempty"`
	Attempts       int       `json:"attempts"`
	MaxAttempts    int       `json:"max_attempts"`
	LastError      string    `json:"last_error,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	AvailableAt    time.Time `json:"available_at"`
}

// ReviewConfig 每仓库审查配置（各字段以 JSON 存储）。
type ReviewConfig struct {
	RepoID           int64    `json:"repo_id"`
	EnabledCheckers  []string `json:"enabled_checkers"`
	EnabledDimensions []string `json:"enabled_dimensions"`
	IgnorePaths      []string `json:"ignore_paths"`
	CustomPrompt     string   `json:"custom_prompt,omitempty"`
	ScoreWeights     map[string]float64 `json:"score_weights"`
	NotifyOn         []string `json:"notify_on"` // success | failure
	MinSeverity      string   `json:"min_severity"` // 通知里最低展示级别
	LLMModelOverride string   `json:"llm_model_override,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Notification 一条通知发送记录。
type Notification struct {
	ID           int64     `json:"id"`
	RepoID       int64     `json:"repo_id"`
	ReviewID     int64     `json:"review_id"`
	Channel      string    `json:"channel"` // wecom | feishu | dingtalk
	WebhookURL   string    `json:"webhook_url"`
	Secret       string    `json:"secret,omitempty"`
	Status       string    `json:"status"` // pending | sent | failed
	ResponseCode int       `json:"response_code,omitempty"`
	Error        string    `json:"error,omitempty"`
	SentAt       *time.Time `json:"sent_at,omitempty"`
}

// LLMConfig 传给 Pi Agent 的单个 LLM 连接配置（settings.llm 中某个 profile 选中后的形态）。
type LLMConfig struct {
	BaseURL       string  `json:"base_url"`
	APIKey        string  `json:"api_key"`
	Model         string  `json:"model"`
	Temperature   float64 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
	TimeoutSec    int     `json:"timeout_sec"`
	ContextWindow int     `json:"context_window"`
}

// LLMProfile 一个可配置的模型条目（UI 可配多个）。
type LLMProfile struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	BaseURL       string  `json:"base_url"`
	APIKey        string  `json:"api_key,omitempty"`
	Model         string  `json:"model"`
	Temperature   float64 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
	TimeoutSec    int     `json:"timeout_sec"`
	ContextWindow int     `json:"context_window"`
	Enabled       bool    `json:"enabled"`
}

// LLMSettings 多模型配置（settings.llm），DefaultID 指向当前使用的 profile。
type LLMSettings struct {
	Profiles  []LLMProfile `json:"profiles"`
	DefaultID string       `json:"default_id"`
}

// Default 返回当前选中模型对应的 LLMConfig；没有可用默认时返回 nil,false。
func (s LLMSettings) Default() (*LLMConfig, bool) {
	for _, p := range s.Profiles {
		if p.ID == s.DefaultID {
			return &LLMConfig{
				BaseURL:       p.BaseURL,
				APIKey:        p.APIKey,
				Model:         p.Model,
				Temperature:   p.Temperature,
				MaxTokens:     p.MaxTokens,
				TimeoutSec:    p.TimeoutSec,
				ContextWindow: p.ContextWindow,
			}, true
		}
	}
	return nil, false
}

// NotifierChannel 通知渠道配置。
type NotifierChannel struct {
	Type       string `json:"type"`        // wecom | feishu | dingtalk
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret,omitempty"`
	Enabled    bool   `json:"enabled"`
}

// ServerConfig 服务级设置（settings.server）。
type ServerConfig struct {
	AdminPasswordHash string `json:"admin_password_hash"`
	JWTSecret         string `json:"jwt_secret"`
	BaseURL           string `json:"base_url"`
}

// WebhookEvent 各平台解析后的统一事件。
type WebhookEvent struct {
	Provider      Provider
	EventType     string // push | pull_request | merge_request
	RepoURL       string
	RepoWebURL    string
	RepoName      string
	DefaultBranch string
	CommitSHA     string
	BaseSHA       string
	TargetRef     string
	SourceRef     string
	PRNumber      int
	PRTitle       string
	PRURL         string
	Author        string
}

// ReviewPayload 是 review 类型 job 的 payload。
type ReviewPayload struct {
	ReviewID  int64  `json:"review_id"`
	RepoID    int64  `json:"repo_id"`
	CommitSHA string `json:"commit_sha"`
	BaseSHA   string `json:"base_sha"`
	TargetRef string `json:"target_ref"`
	SourceRef string `json:"source_ref"`
	PRNumber  int    `json:"pr_number"`
	PRTitle   string `json:"pr_title"`
	PRURL     string `json:"pr_url"`
	Author    string `json:"author"`
	EventType string `json:"event_type"`
}
