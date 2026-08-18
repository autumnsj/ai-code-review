package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/store"
)

type memberResp struct {
	ID          int64  `json:"id"`
	GitLogin    string `json:"git_login"`
	DisplayName string `json:"display_name"`
	Team        string `json:"team"`
	Note        string `json:"note"`
	Active      bool   `json:"active"`
	CreatedAt   string `json:"created_at"`
}

func toMemberResp(a *domain.Author) memberResp {
	return memberResp{
		ID: a.ID, GitLogin: a.GitLogin, DisplayName: a.DisplayName, Team: a.Team,
		Note: a.Note, Active: a.Active, CreatedAt: a.CreatedAt.Format("2006-01-02 15:04"),
	}
}

type createMemberReq struct {
	GitLogin    string `json:"git_login" binding:"required"`
	DisplayName string `json:"display_name"`
	Team        string `json:"team"`
	Note        string `json:"note"`
	Active      *bool  `json:"active"`
}

// POST /api/admin/members
func (s *Server) createMember(c *gin.Context) {
	var req createMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	login := strings.TrimSpace(req.GitLogin)
	if login == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "git_login 不能为空"})
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	a, err := s.store.CreateAuthor(c.Request.Context(), store.CreateAuthorInput{
		GitLogin: login, DisplayName: strings.TrimSpace(req.DisplayName),
		Team: strings.TrimSpace(req.Team), Note: strings.TrimSpace(req.Note), Active: active,
	})
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": "该账号已存在备注"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toMemberResp(a))
}

// GET /api/admin/members
func (s *Server) listMembers(c *gin.Context) {
	authors, err := s.store.ListAuthors(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]memberResp, 0, len(authors))
	for _, a := range authors {
		out = append(out, toMemberResp(a))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type updateMemberReq struct {
	DisplayName *string `json:"display_name"`
	Team        *string `json:"team"`
	Note        *string `json:"note"`
	Active      *bool   `json:"active"`
}

// PATCH /api/admin/members/:id
func (s *Server) updateMember(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req updateMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := store.UpdateAuthorInput{
		DisplayName: trimPtr(req.DisplayName),
		Team:        trimPtr(req.Team),
		Note:        trimPtr(req.Note),
		Active:      req.Active,
	}
	if err := s.store.UpdateAuthor(c.Request.Context(), id, in); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	a, err := s.store.GetAuthorByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toMemberResp(a))
}

// DELETE /api/admin/members/:id
func (s *Server) deleteMember(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := s.store.DeleteAuthor(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/admin/members/unknown  返回审查记录里出现但尚未备注的 git_login。
func (s *Server) unknownMembers(c *gin.Context) {
	logins, err := s.store.ListUnknownLogins(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logins})
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}
