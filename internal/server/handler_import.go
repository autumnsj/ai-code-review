package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ai-code-review/aicr/internal/domain"
	"github.com/ai-code-review/aicr/internal/platform"
	"github.com/ai-code-review/aicr/internal/store"
	"github.com/ai-code-review/aicr/internal/webhook"
)

type importPreviewReq struct {
	Provider     string `json:"provider" binding:"required"`
	APIBaseURL   string `json:"api_base_url"`
	CredentialID int64  `json:"credential_id" binding:"required"`
}

type importRepoItem struct {
	Name            string `json:"name"`
	CloneURL        string `json:"clone_url"`
	WebURL          string `json:"web_url"`
	DefaultBranch   string `json:"default_branch"`
	Private         bool   `json:"private"`
	AlreadyImported bool   `json:"already_imported"`
}

// POST /api/admin/import/preview
func (s *Server) importPreview(c *gin.Context) {
	var req importPreviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := s.buildPlatformClient(c, req.Provider, req.APIBaseURL, req.CredentialID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	login, err := client.Me(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Token 校验失败: " + err.Error()})
		return
	}
	repos, err := client.ListRepos(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "拉取仓库列表失败: " + err.Error()})
		return
	}
	existing, err := s.store.ListRepos(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	have := make(map[string]bool, len(existing))
	for _, r := range existing {
		if n := webhook.NormalizeCloneURL(r.CloneURL); n != "" {
			have[n] = true
		}
	}
	items := make([]importRepoItem, 0, len(repos))
	for _, r := range repos {
		items = append(items, importRepoItem{
			Name:            r.Name,
			CloneURL:        r.CloneURL,
			WebURL:          r.WebURL,
			DefaultBranch:   r.DefaultBranch,
			Private:         r.Private,
			AlreadyImported: have[webhook.NormalizeCloneURL(r.CloneURL)],
		})
	}
	c.JSON(http.StatusOK, gin.H{"login": login, "repos": items})
}

type importCommitItem struct {
	Name          string `json:"name" binding:"required"`
	CloneURL      string `json:"clone_url" binding:"required"`
	WebURL        string `json:"web_url"`
	DefaultBranch string `json:"default_branch"`
}

type importCommitReq struct {
	Provider     string             `json:"provider" binding:"required"`
	APIBaseURL   string             `json:"api_base_url"`
	CredentialID int64              `json:"credential_id" binding:"required"`
	HookSecret   string             `json:"hook_secret"`
	Items        []importCommitItem `json:"items" binding:"required"`
}

type importResultRepo struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	HookURL              string `json:"hook_url"`
	Action               string `json:"action"`                           // created | updated
	DefaultBranch        string `json:"default_branch,omitempty"`         // 同步到的平台默认分支
	DefaultBranchChanged bool   `json:"default_branch_changed,omitempty"` // 是否更新了本地默认分支
	HookRegistered       bool   `json:"hook_registered"`                  // 是否已在平台注册/更新 webhook
	HookError            string `json:"hook_error,omitempty"`             // webhook 注册失败原因
}

// POST /api/admin/import/commit
// 同步语义：本地不存在的仓库新建；已存在的仓库不跳过，而是用平台最新值更新
// （默认分支、网页地址、凭据绑定），随后为每个仓库注册/更新 push webhook
// （删旧重建并同步默认分支），使「从平台导入」成为可重复执行的同步操作。
func (s *Server) importCommit(c *gin.Context) {
	var req importCommitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prov := domain.Provider(strings.ToLower(strings.TrimSpace(req.Provider)))
	if prov == "" {
		prov = domain.ProviderGitHub
	}
	// 凭据存在性校验；若为 HTTPS Token 类型，后续 worker clone 会使用它。
	if req.CredentialID > 0 {
		if _, err := s.store.GetCredential(c.Request.Context(), req.CredentialID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "所选凭据不存在"})
			return
		}
	}
	ctx := c.Request.Context()
	existing, err := s.store.ListRepos(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 以规范化 clone URL 为键，定位已存在的仓库记录。
	have := make(map[string]*domain.Repo, len(existing))
	for _, r := range existing {
		if n := webhook.NormalizeCloneURL(r.CloneURL); n != "" {
			have[n] = r
		}
	}

	results := make([]importResultRepo, 0, len(req.Items))
	for _, it := range req.Items {
		key := webhook.NormalizeCloneURL(it.CloneURL)
		var repo *domain.Repo
		action := "created"
		if exist := have[key]; exist != nil {
			// 已存在：更新为平台最新值，不跳过。
			action = "updated"
			branch := it.DefaultBranch
			if branch == "" {
				branch = "main"
			}
			credID := req.CredentialID
			var hookSecret *string
			if strings.TrimSpace(req.HookSecret) != "" {
				hs := req.HookSecret
				hookSecret = &hs
			}
			if err := s.store.UpdateRepoWithWebURL(ctx, exist.ID, &it.Name, &branch, nil, hookSecret, &credID, nil, &it.WebURL); err != nil {
				results = append(results, importResultRepo{
					ID: exist.ID, Name: it.Name, Action: action, HookError: "更新本地记录失败: " + err.Error(),
				})
				continue
			}
			// 重新读取，拿到更新后的字段（hookToken/provider 等）。
			repo, err = s.store.GetRepoByID(ctx, exist.ID)
			if err != nil {
				results = append(results, importResultRepo{
					ID: exist.ID, Name: it.Name, Action: action, HookError: "读取本地记录失败: " + err.Error(),
				})
				continue
			}
		} else {
			// 不存在：新建。
			repo, err = s.store.CreateRepo(ctx, store.CreateRepoInput{
				Provider:      prov,
				CloneURL:      it.CloneURL,
				WebURL:        it.WebURL,
				Name:          it.Name,
				DefaultBranch: it.DefaultBranch,
				CredentialID:  req.CredentialID,
				HookSecret:    req.HookSecret,
			})
			if err != nil {
				if isDupErr(err) {
					// 并发或规范化差异导致唯一冲突：回读已有记录走更新路径。
					if existing, lerr := s.store.ListRepos(ctx); lerr == nil {
						for _, r := range existing {
							if webhook.NormalizeCloneURL(r.CloneURL) == key {
								repo = r
								action = "updated"
								break
							}
						}
					}
					if repo == nil {
						results = append(results, importResultRepo{Name: it.Name, Action: action, HookError: "记录冲突且无法回读: " + err.Error()})
						continue
					}
				} else {
					results = append(results, importResultRepo{Name: it.Name, Action: action, HookError: "创建失败: " + err.Error()})
					continue
				}
			}
			have[key] = repo
		}

		// 为该仓库同步默认分支并删旧重建 webhook（幂等，失败不影响其它仓库）。
		wr := s.ensureRepoWebhook(ctx, repo)
		item := importResultRepo{
			ID:                   repo.ID,
			Name:                 repo.Name,
			HookURL:              s.baseURL + "/hooks/" + repo.HookToken,
			Action:               action,
			DefaultBranch:        wr.DefaultBranch,
			DefaultBranchChanged: wr.DefaultBranchChanged,
		}
		switch {
		case wr.Skipped != "":
			item.HookError = wr.Skipped
		case wr.Error != "":
			item.HookError = wr.Error
		default:
			item.HookRegistered = true
		}
		results = append(results, item)
	}

	createdN, updatedN, hookFailed := 0, 0, 0
	for _, r := range results {
		if r.Action == "created" {
			createdN++
		} else {
			updatedN++
		}
		if !r.HookRegistered {
			hookFailed++
		}
	}
	c.JSON(http.StatusCreated, gin.H{
		"results": results,
		"created": createdN,
		"updated": updatedN,
		"failed":  hookFailed,
	})
}

// buildPlatformClient 校验凭据并构造平台客户端。
func (s *Server) buildPlatformClient(c *gin.Context, provider, apiBaseURL string, credentialID int64) (platform.Client, error) {
	return s.buildPlatformClientFromCreds(c.Request.Context(), provider, apiBaseURL, credentialID)
}

// buildPlatformClientFromCreds 与上面相同，但接受 context.Context，便于在非单次 HTTP 请求
// 的批量操作（如全部仓库一键注册 webhook）中复用。
func (s *Server) buildPlatformClientFromCreds(ctx context.Context, provider, apiBaseURL string, credentialID int64) (platform.Client, error) {
	cred, err := s.store.GetCredential(ctx, credentialID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errors.New("所选凭据不存在")
		}
		return nil, err
	}
	if cred.Type != domain.CredentialHTTPSToken {
		return nil, errors.New("请选择 HTTPS Token 类型的凭据")
	}
	// 凭据上若已记录 provider/api_base_url，调用方可省略；以请求参数为准。
	p := strings.TrimSpace(provider)
	if p == "" {
		p = cred.Provider
	}
	base := strings.TrimSpace(apiBaseURL)
	if base == "" {
		base = cred.APIBaseURL
	}
	return platform.New(p, base, cred.Secret)
}

func isDupErr(err error) bool {
	if errors.Is(err, store.ErrDuplicate) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}
