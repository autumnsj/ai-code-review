package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ai-code-review/aicr/internal/store"
)

// RegisterPublicRoutes 注册免鉴权公开接口。
func (s *Server) RegisterPublicRoutes(r *gin.RouterGroup) {
	r.GET("/reviews/:token", s.getPublicReview)
}

// GET /public/reviews/:token
func (s *Server) getPublicReview(c *gin.Context) {
	token := c.Param("token")
	rv, err := s.store.GetReviewByPublicToken(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "报告不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	findings, err := s.store.ListFindings(c.Request.Context(), rv.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 公开报告不暴露 repo_id 等内部字段，用专门的响应结构
	c.JSON(http.StatusOK, gin.H{
		"review":  rv,
		"findings": findings,
	})
}
