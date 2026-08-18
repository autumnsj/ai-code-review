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
	ProviderGitea  Provider = "gitea"
)

// Repo 是一个被接入审查的代码仓库。
type Repo struct {
	ID            int64     `json:"id"`
	Provider      Provider  `json:"provider"`
	CloneURL      string    `json:"clone_url"`
	WebURL        string    `json:"web_url"`
	Name          string    `json:"name"`
	DefaultBranch string    `json:"default_branch"`
	AccessToken   string    `json:"access_token,omitempty"`  // 私有仓库 clone 凭证，不对外返回
	CredentialID  int64     `json:"credential_id,omitempty"` // 绑定的可复用凭据（credentials.id），0 表示未绑定
	HookToken     string    `json:"hook_token,omitempty"`    // hookUrl 路径段，创建/重置时返回
	HookSecret    string    `json:"hook_secret,omitempty"`   // 平台签名校验 secret，可选
	Status        string    `json:"status"`                  // active | disabled
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Credential 类型。
const (
	CredentialSSH        = "ssh"
	CredentialHTTPSToken = "https_token"
)

// Credential 是可跨仓库复用的 clone 凭据（SSH 密钥对或 HTTPS Token）。
type Credential struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"` // ssh | https_token
	Secret      string    `json:"-"`    // SSH 私钥 PEM / HTTPS Token，永不通过 API 返回
	PublicKey   string    `json:"public_key,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Provider    string    `json:"provider,omitempty"`     // https_token 时：github|gitlab|gitee|gitea，用于平台 API
	APIBaseURL  string    `json:"api_base_url,omitempty"` // 自建实例 API 地址（如 Gitea/GH Enterprise/GitLab 自建）
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	// ScoreDimensions 各维度评分（含展示名与评分理由），JSON 列。四维默认维度时同时回填上面四列。
	ScoreDimensions map[string]DimensionScore `json:"score_dimensions,omitempty"`
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

// ReviewAuthorReport 是一次审查中按作者（git blame 归属）拆分出的个人报告。
// 一次 base..head 审查可能涉及多位作者，每位作者一条独立记录，拥有自己的
// public_token、评分与问题计数，通知与公开报告均以作者为单位。
type ReviewAuthorReport struct {
	ID              int64                  `json:"id"`
	ReviewID        int64                  `json:"review_id"`
	Author          string                 `json:"author"` // email 稳定键
	AuthorName      string                 `json:"author_name"`
	PublicToken     string                 `json:"public_token"`
	RepoName        string                 `json:"repo_name,omitempty"`
	CommitSHA       string                 `json:"commit_sha,omitempty"`
	BaseSHA         string                 `json:"base_sha,omitempty"`
	TargetRef       string                 `json:"target_ref,omitempty"`
	Summary         string                 `json:"summary,omitempty"`
	ScoreTotal      int                    `json:"score_total"`
	ScoreArch       int                    `json:"score_arch"`
	ScoreQuality    int                    `json:"score_quality"`
	ScoreSecurity   int                    `json:"score_security"`
	ScoreMaint      int                    `json:"score_maint"`
	ScoreDimensions map[string]DimensionScore `json:"score_dimensions,omitempty"`
	FindingsCount   int                    `json:"findings_count"`
	CriticalCount   int                    `json:"critical_count"`
	HighCount       int                    `json:"high_count"`
	MediumCount     int                    `json:"medium_count"`
	LowCount        int                    `json:"low_count"`
	InfoCount       int                    `json:"info_count"`
	Additions       int                    `json:"additions"`
	Deletions       int                    `json:"deletions"`
	FilesChanged    int                    `json:"files_changed"`
	TriggeredAt     time.Time              `json:"triggered_at"`
	FinishedAt      *time.Time             `json:"finished_at,omitempty"`
}

// DimensionScore 单个维度的评分结果（落入 reviews.score_dimensions）。
type DimensionScore struct {
	Score     int    `json:"score"`
	Label     string `json:"label"`
	Rationale string `json:"rationale,omitempty"`
}

// DimensionSpec 一个打分维度的定义（settings.score_dimensions，全局一套）。
type DimensionSpec struct {
	Key         string  `json:"key"`                   // slug，[a-z0-9_]+，报告里的维度键
	Label       string  `json:"label"`                 // 展示名
	Description string  `json:"description,omitempty"` // 评分标准描述（喂给 LLM）
	Weight      float64 `json:"weight"`                // 权重，保存时归一化
}

// DefaultDimensions 返回内置默认打分维度（架构/质量/安全/可维护）。
func DefaultDimensions() []DimensionSpec {
	return []DimensionSpec{
		{Key: "architecture", Label: "架构", Weight: 0.2, Description: "代码结构、模块划分、耦合度、抽象与设计合理性。"},
		{Key: "quality", Label: "质量", Weight: 0.3, Description: "代码正确性、可读性、命名、重复、测试覆盖与边界处理。"},
		{Key: "security", Label: "安全", Weight: 0.3, Description: "注入、硬编码凭据、越权、敏感信息泄露、不安全依赖等安全风险。"},
		{Key: "maintainability", Label: "可维护性", Weight: 0.2, Description: "可扩展性、复杂度、注释文档、未来修改成本与技术债。"},
	}
}

// Author 是 git login 到真人的备注映射（成员管理）。
type Author struct {
	ID          int64     `json:"id"`
	GitLogin    string    `json:"git_login"`
	DisplayName string    `json:"display_name"`
	Team        string    `json:"team"`
	Note        string    `json:"note"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	// Author 是该问题所在行经 git blame 归属的提交者 email（小写）；空表示无法归属。
	Author     string    `json:"author,omitempty"`
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
	// Dimensions 是本次审查实际使用的维度定义（全局维度快照），随 input.json 传给 Pi Agent。
	Dimensions       []DimensionSpec `json:"dimensions,omitempty"`
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

// ReviewLog 是一次审查执行过程中产生的一行进度日志，供前端实时轮询展示。
type ReviewLog struct {
	ID        int64     `json:"id"`
	ReviewID  int64     `json:"review_id"`
	Level     string    `json:"level"` // info|warn|error
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
