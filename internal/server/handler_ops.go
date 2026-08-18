package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/store"
)

type triggerReq struct {
	Mode      string `json:"mode"`
	CommitSHA string `json:"commit_sha"`
	BaseSHA   string `json:"base_sha"`
	TargetRef string `json:"target_ref"`
	SourceRef string `json:"source_ref"`
	Ref       string `json:"ref"`
	Force     bool   `json:"force"`
}

// POST /api/admin/repos/:id/trigger  手动触发一次审查
func (s *Server) triggerRepoReview(c *gin.Context) {
	if s.starter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "trigger unavailable"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req triggerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = "commit"
	}
	if mode == "commit" && req.CommitSHA == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "commit 模式需要 commit_sha"})
		return
	}
	if mode == "branch" && req.Ref == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "branch 模式需要 ref（分支名）"})
		return
	}
	reviewID, token, duplicated, err := s.starter.StartReview(c.Request.Context(), id, StartReviewInput{
		Mode: mode, CommitSHA: req.CommitSHA, BaseSHA: req.BaseSHA,
		TargetRef: req.TargetRef, SourceRef: req.SourceRef, Ref: req.Ref, Force: req.Force,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "repo not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if duplicated {
		c.JSON(http.StatusConflict, gin.H{"error": "该 commit/ref 已有审查记录，传 force=true 可重新审查"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"review_id": reviewID, "public_token": token})
}

type jobResp struct {
	ID            int64   `json:"id"`
	Kind          string  `json:"kind"`
	Status        string  `json:"status"`
	Attempts      int     `json:"attempts"`
	MaxAttempts   int     `json:"max_attempts"`
	LastError     string  `json:"last_error"`
	IdempotencyKey string `json:"idempotency_key"`
	CreatedAt     string  `json:"created_at"`
	StartedAt     *string `json:"started_at"`
	FinishedAt    *string `json:"finished_at"`
}

func toJobResp(j *domain.Job) jobResp {
	resp := jobResp{
		ID: j.ID, Kind: j.Kind, Status: j.Status, Attempts: j.Attempts,
		MaxAttempts: j.MaxAttempts, LastError: j.LastError, IdempotencyKey: j.IdempotencyKey,
		CreatedAt: j.CreatedAt.Format(time.RFC3339),
	}
	if j.StartedAt != nil {
		s := j.StartedAt.Format(time.RFC3339)
		resp.StartedAt = &s
	}
	if j.FinishedAt != nil {
		s := j.FinishedAt.Format(time.RFC3339)
		resp.FinishedAt = &s
	}
	return resp
}

// GET /api/admin/jobs?status=&page=&page_size=
func (s *Server) listJobs(c *gin.Context) {
	if s.jobs == nil {
		c.JSON(http.StatusOK, gin.H{"items": []any{}, "total": 0})
		return
	}
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	jobs, total, err := s.jobs.ListJobs(c.Request.Context(), status, pageSize, (page-1)*pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items := make([]jobResp, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, toJobResp(j))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

// POST /api/admin/jobs/:id/retry
func (s *Server) retryJob(c *gin.Context) {
	if s.jobs == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jobs unavailable"})
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := s.jobs.RetryJob(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found or already running"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/admin/dashboard
func (s *Server) getDashboard(c *gin.Context) {
	if s.dash == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	summary, err := s.dash.Dashboard(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}
