package server

import (
	"context"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/ai-code-review/aicr/internal/auth"
	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/notifier"
	"github.com/ai-code-review/aicr/internal/server/middleware"
	"github.com/ai-code-review/aicr/internal/store"
	"github.com/ai-code-review/aicr/internal/webhook"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	loginMaxFailures = 5
	loginLockout     = 10 * time.Minute
)

type Server struct {
	store     *store.Store
	log       *zap.Logger
	webFS     fs.FS
	baseURL   string
	jwt       *auth.Manager
	webhook   *webhook.Handler
	notifiers NotifierSettings
	starter   ReviewStarter
	jobs      JobAdmin
	dash      DashboardProvider
	stats     StatsProvider
	loginGuard *middleware.LoginGuard
}

// NotifierSettings 通知配置读写接口。
type NotifierSettings interface {
	GetChannels(ctx context.Context) ([]notifier.Channel, error)
	SaveChannels(ctx context.Context, chs []notifier.Channel) error
}

// StartReviewInput 手动触发审查的入参。
type StartReviewInput struct {
	// Mode: commit(默认,需 CommitSHA) | branch(用 Ref 分支名解析 HEAD) | repo(评审默认分支当前状态)。
	Mode      string
	CommitSHA string
	BaseSHA   string
	TargetRef string
	SourceRef string
	// Ref 为 branch 模式下的分支名。
	Ref string
	// Force 为 true 时，若该 commit/ref 已有审查记录，则重置并重新审查（复用同一行）。
	Force bool
}

// ReviewStarter 手动触发审查（创建 review 记录并入队）。
type ReviewStarter interface {
	StartReview(ctx context.Context, repoID int64, in StartReviewInput) (reviewID int64, publicToken string, duplicated bool, err error)
}

// JobAdmin 任务查看与重试。
type JobAdmin interface {
	ListJobs(ctx context.Context, status string, limit, offset int) ([]*domain.Job, int, error)
	RetryJob(ctx context.Context, jobID int64) error
}

// DashboardSummary 概览统计。
type DashboardSummary struct {
	RepoCount     int64   `json:"repo_count"`
	ReviewCount   int64   `json:"review_count"`
	Succeeded     int64   `json:"succeeded"`
	Failed        int64   `json:"failed"`
	Running       int64   `json:"pending"`
	AvgScore      float64 `json:"avg_score"`
	RecentReviews any     `json:"recent_reviews"`
}

type DashboardProvider interface {
	Dashboard(ctx context.Context) (*DashboardSummary, error)
}

// AuthorSummary 作者统计行（对外 JSON）。
type AuthorSummary struct {
	Author        string  `json:"author"`
	DisplayName   string  `json:"display_name"`
	Team          string  `json:"team"`
	ReviewCount   int64   `json:"review_count"`
	AvgTotal      float64 `json:"avg_total"`
	AvgArch       float64 `json:"avg_arch"`
	AvgQuality    float64 `json:"avg_quality"`
	AvgSecurity   float64 `json:"avg_security"`
	AvgMaint      float64 `json:"avg_maint"`
	Additions     int64   `json:"additions"`
	Deletions     int64   `json:"deletions"`
	FilesChanged  int64   `json:"files_changed"`
	Churn         int64   `json:"churn"`
	TokensUsed    int64   `json:"tokens_used"`
	FindingsTotal int64   `json:"findings_total"`
	Critical      int64   `json:"critical"`
	High          int64   `json:"high"`
	Medium        int64   `json:"medium"`
	Low           int64   `json:"low"`
	Info          int64   `json:"info"`
	LastReviewed  string  `json:"last_reviewed"`
}

// AuthorDetail 单个作者明细：汇总 + 最近审查。
type AuthorDetail struct {
	Summary AuthorSummary `json:"summary"`
	Recent  []any         `json:"recent"`
}

// StatsProvider 提供作者维度统计。
type StatsProvider interface {
	ListAuthors(ctx context.Context, days int, repoID int64, sort string, limit, offset int) ([]AuthorSummary, error)
	// Leaderboard 一次性返回多个指标各取 Top N 的作者排行，用于排行榜页面。
	// 返回 key 为指标名（churn/additions/deletions/review_count/findings/
	// avg_total/avg_arch/avg_quality/avg_security/avg_maint），value 为该指标降序的作者列表。
	Leaderboard(ctx context.Context, days int, repoID int64, limit int) (map[string][]AuthorSummary, error)
	GetAuthor(ctx context.Context, author string, days int, repoID int64) (*AuthorDetail, error)
}

func New(st *store.Store, log *zap.Logger, webFS fs.FS, baseURL string, jwt *auth.Manager, wh *webhook.Handler, notif NotifierSettings, starter ReviewStarter, jobs JobAdmin, dash DashboardProvider, stats StatsProvider) *Server {
	return &Server{
		store:      st,
		log:        log,
		webFS:      webFS,
		baseURL:    baseURL,
		jwt:        jwt,
		webhook:    wh,
		notifiers:  notif,
		starter:    starter,
		jobs:       jobs,
		dash:       dash,
		stats:      stats,
		loginGuard: middleware.NewLoginGuard(loginMaxFailures, loginLockout),
	}
}

func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(s.requestLogger())
	r.Use(middleware.SecurityHeaders())

	// 免鉴权公共端点
	r.GET("/public/healthz", s.healthz)

	// Webhook（按 IP 限流：每 IP 每秒 5 个、突发 10）
	hooks := r.Group("/hooks", middleware.RateLimit(5, 10))
	s.webhook.RegisterOn(hooks)

	// 公开报告（免鉴权）
	public := r.Group("/public")
	s.RegisterPublicRoutes(public)

	// 管理后台 API
	admin := r.Group("/api/admin")
	admin.POST("/login", s.loginGuard.Middleware(), s.login)

	authed := admin.Group("")
	authed.Use(middleware.JWTAuth(s.jwt))
	{
		authed.POST("/change-password", s.changePassword)
		authed.GET("/settings/llm", s.getLLMSettings)
		authed.PUT("/settings/llm", s.updateLLMSettings)
		authed.POST("/settings/llm/fetch-models", s.fetchModels)
		authed.GET("/settings/server", s.getServerSettings)
		authed.PUT("/settings/server", s.updateServerSettings)
		authed.GET("/settings/notifications", s.getNotifiers)
		authed.PUT("/settings/notifications", s.updateNotifiers)
		authed.GET("/settings/dimensions", s.getDimensions)
		authed.PUT("/settings/dimensions", s.updateDimensions)
		authed.GET("/settings/review-limits", s.getReviewLimits)
		authed.PUT("/settings/review-limits", s.updateReviewLimits)

		authed.GET("/repos", s.listRepos)
		authed.POST("/repos", s.createRepo)
		authed.GET("/repos/:id", s.getRepo)
		authed.PATCH("/repos/:id", s.updateRepo)
		authed.DELETE("/repos/:id", s.deleteRepo)
		authed.POST("/repos/:id/reset-token", s.resetRepoToken)
		authed.POST("/repos/:id/trigger", s.triggerRepoReview)
		authed.POST("/repos/:id/webhook", s.registerRepoWebhook)
		authed.POST("/webhooks/register-all", s.registerAllRepoWebhooks)
		authed.POST("/import/preview", s.importPreview)
		authed.POST("/import/commit", s.importCommit)

		authed.GET("/credentials", s.listCredentials)
		authed.POST("/credentials", s.createCredential)
		authed.PATCH("/credentials/:id", s.updateCredential)
		authed.DELETE("/credentials/:id", s.deleteCredential)

		authed.GET("/members", s.listMembers)
		authed.POST("/members", s.createMember)
		authed.PATCH("/members/:id", s.updateMember)
		authed.DELETE("/members/:id", s.deleteMember)
		authed.GET("/unknown-members", s.unknownMembers)

		authed.GET("/reviews", s.listReviews)
		authed.GET("/reviews/:id", s.getReview)
		authed.GET("/reviews/:id/findings", s.listFindings)
		authed.GET("/reviews/:id/author-reports", s.listAuthorReports)
		authed.GET("/reviews/:id/log", s.listReviewLogs)

		authed.GET("/jobs", s.listJobs)
		authed.POST("/jobs/:id/retry", s.retryJob)

		authed.GET("/dashboard", s.getDashboard)

		authed.GET("/stats/authors", s.listAuthorStats)
		authed.GET("/stats/leaderboard", s.leaderboard)
		authed.GET("/stats/authors/:author", s.getAuthorStats)
	}

	// 静态资源 + SPA fallback
	r.NoRoute(s.serveSPA)
	return r
}

func (s *Server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// serveSPA 服务嵌入的前端静态资源，非文件路径回退到 index.html。
func (s *Server) serveSPA(c *gin.Context) {
	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/hooks/") || strings.HasPrefix(path, "/public/") {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	if path != "/" {
		name := strings.TrimPrefix(path, "/")
		if f, err := s.webFS.Open(name); err == nil {
			f.Close()
			c.FileFromFS(name, http.FS(s.webFS))
			return
		}
	}
	data, err := fs.ReadFile(s.webFS, "index.html")
	if err != nil {
		c.String(http.StatusServiceUnavailable, "frontend not built")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		s.log.Debug("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
		)
	}
}
