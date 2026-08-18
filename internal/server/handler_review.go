package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ai-code-review/aicr/internal/store"
)

// GET /api/admin/reviews?repo_id=&page=&page_size=
func (s *Server) listReviews(c *gin.Context) {
	repoID, _ := strconv.ParseInt(c.Query("repo_id"), 10, 64)
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	items, total, err := s.store.ListReviews(c.Request.Context(), repoID, pageSize, (page-1)*pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

// GET /api/admin/reviews/:id
func (s *Server) getReview(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rv, err := s.store.GetReview(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rv)
}

// GET /api/admin/reviews/:id/findings
func (s *Server) listFindings(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	items, err := s.store.ListFindings(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GET /api/admin/reviews/:id/author-reports
// 列出一次审查按作者拆分的个人报告（多作者时每位参与者一条，各自有公开 token）。
func (s *Server) listAuthorReports(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	items, err := s.store.ListAuthorReportsByReview(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GET /api/admin/reviews/:id/log?since=0&limit=500
// 返回审查执行过程中逐行落库的进度日志，供前端实时轮询。since 为已收到的最大日志 id（不含）。
func (s *Server) listReviewLogs(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if _, err := s.store.GetReview(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	since, _ := strconv.ParseInt(c.DefaultQuery("since", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
	items, err := s.store.ListReviewLogs(c.Request.Context(), id, since, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	nextID := since
	if len(items) > 0 {
		nextID = items[len(items)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "next_id": nextID})
}
