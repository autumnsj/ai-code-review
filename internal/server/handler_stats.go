package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GET /api/admin/stats/authors?days=&repo_id=&sort=&page=&page_size=
func (s *Server) listAuthorStats(c *gin.Context) {
	if s.stats == nil {
		c.JSON(http.StatusOK, gin.H{"items": []any{}})
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	repoID, _ := strconv.ParseInt(c.Query("repo_id"), 10, 64)
	sort := c.DefaultQuery("sort", "avg_score")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	items, err := s.stats.ListAuthors(c.Request.Context(), days, repoID, sort, pageSize, (page-1)*pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": page, "page_size": pageSize})
}

// GET /api/admin/stats/leaderboard?days=&repo_id=&limit=
func (s *Server) leaderboard(c *gin.Context) {
	if s.stats == nil {
		c.JSON(http.StatusOK, gin.H{"boards": gin.H{}})
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	repoID, _ := strconv.ParseInt(c.Query("repo_id"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit < 1 || limit > 50 {
		limit = 10
	}
	boards, err := s.stats.Leaderboard(c.Request.Context(), days, repoID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"boards": boards})
}

// GET /api/admin/stats/authors/:author?days=&repo_id=
func (s *Server) getAuthorStats(c *gin.Context) {
	if s.stats == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "stats unavailable"})
		return
	}
	author := c.Param("author")
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	repoID, _ := strconv.ParseInt(c.Query("repo_id"), 10, 64)
	detail, err := s.stats.GetAuthor(c.Request.Context(), author, days, repoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}
