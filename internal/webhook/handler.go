package webhook

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/store"
)

// SignatureVerifier 是可选的能力：平台 parser 实现它以校验请求签名。
type SignatureVerifier interface {
	Verify(secret string, body []byte, r *http.Request) bool
}

// Enqueuer 负责把审查任务入队。
type Enqueuer interface {
	EnqueueReview(ctx context.Context, payload domain.ReviewPayload, idempotencyKey string) error
}

type Handler struct {
	store   *store.Store
	log     *zap.Logger
	enqueue Enqueuer
	parsers []Parser
}

func NewHandler(st *store.Store, log *zap.Logger, enq Enqueuer) *Handler {
	return &Handler{store: st, log: log, enqueue: enq, parsers: DefaultParsers()}
}

// Register 在 gin 引擎上注册 POST /hooks/:token。
func (h *Handler) Register(r gin.IRouter) {
	r.POST("/hooks/:token", h.handle)
}

// RegisterOn 在给定路由组上注册 POST /:token（组前缀通常为 /hooks）。
func (h *Handler) RegisterOn(r gin.IRouter) {
	r.POST("/:token", h.handle)
}

func (h *Handler) handle(c *gin.Context) {
	token := c.Param("token")
	repo, err := h.store.GetRepoByHookToken(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "invalid hook token"})
			return
		}
		h.log.Error("lookup repo by hook token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	if repo.Status != "active" {
		c.JSON(http.StatusOK, gin.H{"ok": true, "ignored": "repo disabled"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body"})
		return
	}

	// 匹配平台解析器
	var parser Parser
	for _, p := range h.parsers {
		if p.Match(c.Request) {
			parser = p
			break
		}
	}
	if parser == nil {
		c.JSON(http.StatusAccepted, gin.H{"ok": true, "ignored": "unknown platform"})
		return
	}

	// 签名校验（若配置了 secret）
	if v, ok := parser.(SignatureVerifier); ok && repo.HookSecret != "" {
		if !v.Verify(repo.HookSecret, body, c.Request) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
	}

	event, err := parser.Parse(body, c.Request)
	if err != nil {
		if errors.Is(err, ErrIgnoredEvent) {
			c.JSON(http.StatusAccepted, gin.H{"ok": true, "ignored": "event type"})
			return
		}
		h.log.Warn("parse webhook", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "parse payload"})
		return
	}

	// 校验来源仓库与配置一致（防止 token 泄露后被挪用于其它仓库）
	if !SameRepo(repo.CloneURL, event.RepoURL) {
		h.log.Warn("webhook repo mismatch",
			zap.String("configured", repo.CloneURL), zap.String("event", event.RepoURL))
		c.JSON(http.StatusForbidden, gin.H{"error": "repository mismatch"})
		return
	}

	// 只审查推送到「默认分支」的提交；其它分支（feature/release 等）的 push 直接忽略，
	// 避免每个分支推送都触发审查。PR/MR 事件不受此限制（合并前审查目标分支改动）。
	if event.EventType == "push" {
		defaultBranch := repo.DefaultBranch
		if defaultBranch == "" {
			defaultBranch = event.DefaultBranch
		}
		if defaultBranch == "" {
			defaultBranch = "main"
		}
		if event.TargetRef != defaultBranch {
			c.JSON(http.StatusOK, gin.H{
				"ok":      true,
				"ignored": "non-default branch",
				"branch":  event.TargetRef,
				"default": defaultBranch,
			})
			return
		}
	}

	// 创建审查记录
	publicToken, err := newPublicToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token"})
		return
	}
	rv, err := h.store.CreateReview(c.Request.Context(), store.CreateReviewInput{
		RepoID:      repo.ID,
		PublicToken: publicToken,
		CommitSHA:   event.CommitSHA,
		BaseSHA:     event.BaseSHA,
		TargetRef:   event.TargetRef,
		SourceRef:   event.SourceRef,
		PRNumber:    event.PRNumber,
		PRTitle:     event.PRTitle,
		PRURL:       event.PRURL,
		Author:      event.Author,
		EventType:   event.EventType,
	})
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			c.JSON(http.StatusOK, gin.H{"ok": true, "ignored": "duplicate"})
			return
		}
		h.log.Error("create review", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create review"})
		return
	}

	payload := domain.ReviewPayload{
		ReviewID:  rv.ID,
		RepoID:    repo.ID,
		CommitSHA: event.CommitSHA,
		BaseSHA:   event.BaseSHA,
		TargetRef: event.TargetRef,
		SourceRef: event.SourceRef,
		PRNumber:  event.PRNumber,
		PRTitle:   event.PRTitle,
		PRURL:     event.PRURL,
		Author:    event.Author,
		EventType: event.EventType,
	}
	key := reviewIdempotencyKey(repo.ID, event.CommitSHA, event.TargetRef)
	if err := h.enqueue.EnqueueReview(c.Request.Context(), payload, key); err != nil {
		h.log.Error("enqueue review", zap.Error(err))
		// review 记录已建但入队失败，不阻断 webhook 响应，后台可补偿
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "review_id": rv.ID, "public_token": publicToken})
}

func reviewIdempotencyKey(repoID int64, sha, targetRef string) string {
	return "review:" + strconv.FormatInt(repoID, 10) + ":" + sha + ":" + targetRef
}

// SameRepo 比较两个 clone URL 是否指向同一仓库（忽略协议、凭证、末尾 .git）。
func SameRepo(configured, event string) bool {
	a, b := NormalizeCloneURL(configured), NormalizeCloneURL(event)
	return a != "" && a == b
}

// NormalizeCloneURL 把 clone URL 归一化为 host/owner/repo（小写、去协议/凭证/.git）。
func NormalizeCloneURL(s string) string {
	s = strings.TrimSpace(s)
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		s = u.Host + u.Path
	}
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "git@")
	s = strings.ReplaceAll(s, ":", "/")
	return strings.ToLower(strings.TrimSuffix(s, "/"))
}

func newPublicToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
