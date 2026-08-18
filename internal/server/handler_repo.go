package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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

// webhookRegisterResult 单个仓库的注册结果。
type webhookRegisterResult struct {
	RepoID               int64  `json:"repo_id"`
	RepoName             string `json:"repo_name"`
	Created              bool   `json:"created"`
	AlreadyExists        bool   `json:"already_exists"`
	HookID               string `json:"hook_id,omitempty"`
	HookURL              string `json:"hook_url,omitempty"`
	DefaultBranch        string `json:"default_branch,omitempty"`         // 从平台同步到的默认分支
	DefaultBranchChanged bool   `json:"default_branch_changed,omitempty"` // 是否回写更新了本地记录
	Skipped              string `json:"skipped,omitempty"`                // 非空表示跳过原因（如未绑定凭据）
	Error                string `json:"error,omitempty"`
}

// ensureRepoWebhook 为单个仓库幂等注册 push webhook；缺 secret 时自动生成并落库。
// 同时从平台拉取仓库元数据，把真实的默认分支同步回本地（平台改过默认分支也能跟上）。
func (s *Server) ensureRepoWebhook(ctx context.Context, repo *domain.Repo) webhookRegisterResult {
	res := webhookRegisterResult{RepoID: repo.ID, RepoName: repo.Name, HookURL: s.baseURL + "/hooks/" + repo.HookToken}
	if repo.CredentialID == 0 {
		res.Skipped = "未绑定 HTTPS Token 凭据"
		return res
	}
	client, err := s.buildPlatformClientFromCreds(ctx, string(repo.Provider), "", repo.CredentialID)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	// 1) 从平台同步默认分支（失败不阻断 webhook 注册）。
	if info, err := client.GetRepo(ctx, repo.Name); err != nil {
		s.log.Warn("同步仓库默认分支失败",
			zap.String("repo", repo.Name), zap.Error(err))
	} else if info.DefaultBranch != "" {
		res.DefaultBranch = info.DefaultBranch
		if info.DefaultBranch != repo.DefaultBranch {
			branch := info.DefaultBranch
			if err := s.store.UpdateRepo(ctx, repo.ID, nil, &branch, nil, nil, nil, nil); err != nil {
				s.log.Warn("回写默认分支失败",
					zap.String("repo", repo.Name), zap.Error(err))
			} else {
				res.DefaultBranchChanged = true
				repo.DefaultBranch = info.DefaultBranch
			}
		}
	}
	// 2) 缺签名 secret 时自动生成并落库。
	secret := repo.HookSecret
	if secret == "" {
		secret, err = newHookSecret()
		if err != nil {
			res.Error = err.Error()
			return res
		}
		if err := s.store.UpdateRepo(ctx, repo.ID, nil, nil, nil, &secret, nil, nil); err != nil {
			res.Error = err.Error()
			return res
		}
	}
	// 3) 注册 push webhook；把默认分支作为平台侧分支过滤器（Gitea/GitLab 支持），
	//    使非默认分支的 push 在平台侧就不发送，GitHub/Gitee 由服务端兜底过滤。
	//    分支名不能为空（否则平台会退化为「所有分支」），取不到时保守回退 main。
	branchFilter := repo.DefaultBranch
	if branchFilter == "" {
		branchFilter = "main"
	}
	created, hookID, err := client.EnsureWebhook(ctx, repo.Name, res.HookURL, secret, branchFilter)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Created = created
	res.AlreadyExists = !created
	res.HookID = hookID
	return res
}

// POST /api/admin/repos/:id/webhook  一键注册 push webhook（已存在同 URL 的 hook 则删旧重建，并同步默认分支）。
func (s *Server) registerRepoWebhook(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	ctx := c.Request.Context()
	repo, err := s.store.GetRepoByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "仓库不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	res := s.ensureRepoWebhook(ctx, repo)
	if res.Skipped != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先为该仓库绑定 HTTPS Token 凭据（需 webhook 管理权限）"})
		return
	}
	if res.Error != "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": res.Error})
		return
	}
	c.JSON(http.StatusOK, res)
}

// POST /api/admin/repos/webhook/register-all  为所有 active 且绑定了凭据的仓库批量注册 webhook。
// 逐个幂等注册，部分失败不影响其它仓库；返回每个仓库的结果汇总。
func (s *Server) registerAllRepoWebhooks(c *gin.Context) {
	ctx := c.Request.Context()
	repos, err := s.store.ListRepos(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	results := make([]webhookRegisterResult, 0, len(repos))
	var created, existed, skipped, failed int
	for _, repo := range repos {
		if repo.Status == "disabled" {
			continue
		}
		res := s.ensureRepoWebhook(ctx, repo)
		switch {
		case res.Skipped != "":
			skipped++
		case res.Error != "":
			failed++
		case res.Created:
			created++
		default:
			existed++
		}
		results = append(results, res)
	}
	c.JSON(http.StatusOK, gin.H{
		"total":   len(results),
		"created": created,
		"existed": existed,
		"skipped": skipped,
		"failed":  failed,
		"items":   results,
	})
}

// newHookSecret 生成 24 字节随机 hex，用作 webhook 签名密钥。
func newHookSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
