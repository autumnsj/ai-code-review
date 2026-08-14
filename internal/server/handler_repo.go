package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/store"
)

type createRepoReq struct {
	Provider      string `json:"provider"`
	CloneURL      string `json:"clone_url" binding:"required"`
	WebURL        string `json:"web_url"`
	Name          string `json:"name" binding:"required"`
	DefaultBranch string `json:"default_branch"`
	AccessToken   string `json:"access_token"`
	CredentialID  *int64 `json:"credential_id"`
	HookSecret    string `json:"hook_secret"`
}

type repoResp struct {
	ID             int64  `json:"id"`
	Provider       string `json:"provider"`
	CloneURL       string `json:"clone_url"`
	WebURL         string `json:"web_url"`
	Name           string `json:"name"`
	DefaultBranch  string `json:"default_branch"`
	CredentialID   int64  `json:"credential_id"`
	CredentialName string `json:"credential_name,omitempty"`
	HookURL        string `json:"hook_url"`
	HasSecret      bool   `json:"has_secret"`
	Status         string `json:"status"`
}

// toRepoResp 把领域对象转为响应。credNames 为 credentialID -> name 的映射（可为 nil），
// 由列表接口一次性查询填充，避免 N+1。
func (s *Server) toRepoResp(r *domain.Repo, credNames map[int64]string) repoResp {
	resp := repoResp{
		ID:            r.ID,
		Provider:      string(r.Provider),
		CloneURL:      maskCloneURL(r.CloneURL),
		WebURL:        r.WebURL,
		Name:          r.Name,
		DefaultBranch: r.DefaultBranch,
		CredentialID:  r.CredentialID,
		HookURL:       s.baseURL + "/hooks/" + r.HookToken,
		HasSecret:     r.HookSecret != "",
		Status:        r.Status,
	}
	if r.CredentialID > 0 && credNames != nil {
		resp.CredentialName = credNames[r.CredentialID]
	}
	return resp
}

func (s *Server) credentialNameMap(ctx context.Context) map[int64]string {
	creds, err := s.store.ListCredentials(ctx)
	if err != nil {
		return nil
	}
	m := make(map[int64]string, len(creds))
	for _, c := range creds {
		m[c.ID] = c.Name
	}
	return m
}

// toRepoRespWithToken 创建/重置时返回明文 hookToken（仅此一次）。
func (s *Server) toRepoRespWithToken(r *domain.Repo, credNames map[int64]string) repoResp {
	resp := s.toRepoResp(r, credNames)
	resp.HookURL = s.baseURL + "/hooks/" + r.HookToken
	return resp
}

func maskCloneURL(u string) string {
	// 简单隐藏 clone url 里的凭证
	if idx := IndexByte(u, '@'); idx > 0 {
		if scheme := IndexByte(u, ':'); scheme > 0 && scheme < idx {
			return u[:scheme+3] + "****" + u[idx:]
		}
	}
	return u
}

// IndexByte 避免引入 strings 只为一处使用，保持可读性。
func IndexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// GET /api/admin/repos
func (s *Server) listRepos(c *gin.Context) {
	repos, err := s.store.ListRepos(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	credNames := s.credentialNameMap(c.Request.Context())
	out := make([]repoResp, 0, len(repos))
	for _, r := range repos {
		out = append(out, s.toRepoResp(r, credNames))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// POST /api/admin/repos
func (s *Server) createRepo(c *gin.Context) {
	var req createRepoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prov := domain.Provider(req.Provider)
	if prov == "" {
		prov = domain.ProviderGitHub
	}
	var credID int64
	if req.CredentialID != nil && *req.CredentialID > 0 {
		if _, err := s.store.GetCredential(c.Request.Context(), *req.CredentialID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "所选凭据不存在"})
			return
		}
		credID = *req.CredentialID
	}
	r, err := s.store.CreateRepo(c.Request.Context(), store.CreateRepoInput{
		Provider:      prov,
		CloneURL:      req.CloneURL,
		WebURL:        req.WebURL,
		Name:          req.Name,
		DefaultBranch: req.DefaultBranch,
		AccessToken:   req.AccessToken,
		CredentialID:  credID,
		HookSecret:    req.HookSecret,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, s.toRepoRespWithToken(r, s.credentialNameMap(c.Request.Context())))
}

// GET /api/admin/repos/:id
func (s *Server) getRepo(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	r, err := s.store.GetRepoByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s.toRepoResp(r, s.credentialNameMap(c.Request.Context())))
}

type updateRepoReq struct {
	Name          *string `json:"name"`
	DefaultBranch *string `json:"default_branch"`
	AccessToken   *string `json:"access_token"`
	CredentialID  *int64  `json:"credential_id"`
	HookSecret    *string `json:"hook_secret"`
	Status        *string `json:"status"`
}

// PATCH /api/admin/repos/:id
func (s *Server) updateRepo(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req updateRepoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.CredentialID != nil && *req.CredentialID > 0 {
		if _, err := s.store.GetCredential(c.Request.Context(), *req.CredentialID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "所选凭据不存在"})
			return
		}
	}
	if err := s.store.UpdateRepo(c.Request.Context(), id, req.Name, req.DefaultBranch, req.AccessToken, req.HookSecret, req.CredentialID, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	r, _ := s.store.GetRepoByID(c.Request.Context(), id)
	c.JSON(http.StatusOK, s.toRepoResp(r, s.credentialNameMap(c.Request.Context())))
}

// POST /api/admin/repos/:id/reset-token
func (s *Server) resetRepoToken(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	tok, err := s.store.ResetHookToken(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"hook_token": tok, "hook_url": s.baseURL + "/hooks/" + tok})
}

// DELETE /api/admin/repos/:id
func (s *Server) deleteRepo(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := s.store.DeleteRepo(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
