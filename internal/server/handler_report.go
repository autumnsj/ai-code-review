package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/store"
)

// GET /api/admin/settings/report-schedules
func (s *Server) getReportSchedules(c *gin.Context) {
	cfg, err := s.reports.GetReportSchedules(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取失败"})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// PUT /api/admin/settings/report-schedules
func (s *Server) updateReportSchedules(c *gin.Context) {
	var cfg domain.ReportScheduleConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.reports.SaveReportSchedules(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	saved, _ := s.reports.GetReportSchedules(c.Request.Context())
	c.JSON(http.StatusOK, saved)
}

type reportKindRequest struct {
	// Kind: daily | weekly
	Kind string `json:"kind"`
}

func (r reportKindRequest) valid() bool {
	return r.Kind == "daily" || r.Kind == "weekly"
}

// POST /api/admin/report-schedules/preview  生成报告内容但不发送。
func (s *Server) previewReport(c *gin.Context) {
	var req reportKindRequest
	if err := c.ShouldBindJSON(&req); err != nil || !req.valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kind 必须为 daily 或 weekly"})
		return
	}
	title, markdown, err := s.reports.PreviewReport(c.Request.Context(), req.Kind)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成预览失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"title": title, "markdown": markdown})
}

// POST /api/admin/report-schedules/run  立即入队一条手动报告任务。
func (s *Server) runReport(c *gin.Context) {
	var req reportKindRequest
	if err := c.ShouldBindJSON(&req); err != nil || !req.valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kind 必须为 daily 或 weekly"})
		return
	}
	if err := s.reports.RunReportNow(c.Request.Context(), req.Kind); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "入队失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/admin/reports?page=&page_size=  报告记录列表（不含正文）。
func (s *Server) listReportRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	items, total, err := s.store.ListReports(c.Request.Context(), pageSize, (page-1)*pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

// GET /api/admin/reports/:id  报告详情（含 markdown 正文）。
func (s *Server) getReportRecord(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rpt, err := s.store.GetReport(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "报告不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rpt)
}
