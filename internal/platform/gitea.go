package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type giteaClient struct {
	base   string
	token  string
	header http.Header
}

func newGiteaClient(baseURL, token string) *giteaClient {
	h := http.Header{}
	h.Set("Authorization", "token "+token)
	return &giteaClient{base: trimSlash(baseURL), token: token, header: h}
}

func (c *giteaClient) Me(ctx context.Context) (string, error) {
	if c.base == "" {
		return "", fmt.Errorf("Gitea 需要填写 API 地址（自建实例）")
	}
	body, err := httpGet(ctx, c.base+"/api/v1/user", c.header)
	if err != nil {
		return "", err
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := decodeJSON(body, &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

// giteaRepo 是 Gitea /repos、/user/repos、/orgs/{org}/repos 共用的仓库字段子集。
type giteaRepo struct {
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
	HTMLURL  string `json:"html_url"`
	Default  string `json:"default_branch"`
	Private  bool   `json:"private"`
}

// fetchRepoPage 拉取一页仓库列表（Gitea 单页 limit 上限 50）。
func (c *giteaClient) fetchRepoPage(ctx context.Context, endpoint string, page int) ([]giteaRepo, error) {
	u := setQuery(c.base+endpoint, map[string]string{
		"limit": "50", "page": itoa(page),
	})
	body, err := httpGet(ctx, u, c.header)
	if err != nil {
		return nil, err
	}
	var repos []giteaRepo
	if err := decodeJSON(body, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// appendRepos 把仓库追加到 out，按 full_name 去重。
func appendGiteaRepos(out *[]RepoInfo, seen map[string]bool, repos []giteaRepo) {
	for _, r := range repos {
		if r.FullName == "" || seen[r.FullName] {
			continue
		}
		seen[r.FullName] = true
		*out = append(*out, RepoInfo{
			Name: r.FullName, CloneURL: r.CloneURL, WebURL: r.HTMLURL,
			DefaultBranch: r.Default, Private: r.Private,
		})
	}
}

func (c *giteaClient) ListRepos(ctx context.Context) ([]RepoInfo, error) {
	if c.base == "" {
		return nil, fmt.Errorf("Gitea 需要填写 API 地址（自建实例）")
	}
	var out []RepoInfo
	seen := make(map[string]bool)

	// 1) 个人仓库 + 协作者仓库 + 组织成员仓库（显式带 affiliation=organization_member，
	//    Gitea /user/repos 默认不返回通过团队授权的组织仓库）。
	for page := 1; page <= 50; page++ {
		u := setQuery(c.base+"/api/v1/user/repos", map[string]string{
			"limit": "50", "page": itoa(page),
			"affiliation": "owner,collaborator,organization_member",
		})
		body, err := httpGet(ctx, u, c.header)
		if err != nil {
			return nil, err
		}
		var repos []giteaRepo
		if err := decodeJSON(body, &repos); err != nil {
			return nil, err
		}
		appendGiteaRepos(&out, seen, repos)
		if len(repos) < 50 {
			break
		}
	}

	// 2) 组织仓库：Gitea 的 /user/repos 不保证返回所有通过团队(team)授权的组织仓库，
	// 逐个拉取用户所属组织的仓库列表补齐（按 full_name 去重）。
	body, err := httpGet(ctx, c.base+"/api/v1/user/orgs", c.header)
	if err != nil {
		// 组织列表是「补齐」用途，不可得时不阻断个人仓库结果。
		return out, nil
	}
	var orgs []struct {
		UserName string `json:"username"`
	}
	if err := decodeJSON(body, &orgs); err != nil {
		return out, nil
	}
	for _, org := range orgs {
		if org.UserName == "" {
			continue
		}
		endpoint := "/api/v1/orgs/" + url.PathEscape(org.UserName) + "/repos"
		for page := 1; page <= 50; page++ {
			repos, err := c.fetchRepoPage(ctx, endpoint, page)
			if err != nil {
				// 单个组织无权限/不可达时跳过，继续其它组织。
				break
			}
			appendGiteaRepos(&out, seen, repos)
			if len(repos) < 50 {
				break
			}
		}
	}
	return out, nil
}

func (c *giteaClient) GetRepo(ctx context.Context, repoFullName string) (RepoInfo, error) {
	if c.base == "" {
		return RepoInfo{}, fmt.Errorf("Gitea 需要填写 API 地址（自建实例）")
	}
	body, err := httpGet(ctx, c.base+"/api/v1/repos/"+repoFullName, c.header)
	if err != nil {
		return RepoInfo{}, err
	}
	var r struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
		Default  string `json:"default_branch"`
		Private  bool   `json:"private"`
	}
	if err := decodeJSON(body, &r); err != nil {
		return RepoInfo{}, err
	}
	return RepoInfo{
		Name: r.FullName, CloneURL: r.CloneURL, WebURL: r.HTMLURL,
		DefaultBranch: r.Default, Private: r.Private,
	}, nil
}

func (c *giteaClient) ResolveCommit(ctx context.Context, repoFullName, ref string) (string, error) {
	if isSHA(ref) {
		return ref, nil
	}
	u := c.base + "/api/v1/repos/" + repoFullName + "/branches/" + url.PathEscape(ref)
	body, err := httpGet(ctx, u, c.header)
	if err != nil {
		return "", err
	}
	var br struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := decodeJSON(body, &br); err != nil {
		return "", err
	}
	return br.Commit.ID, nil
}

func (c *giteaClient) EnsureWebhook(ctx context.Context, repoFullName, targetURL, secret, branchFilter string) (bool, string, error) {
	if c.base == "" {
		return false, "", fmt.Errorf("Gitea 需要填写 API 地址（自建实例）")
	}
	listURL := c.base + "/api/v1/repos/" + repoFullName + "/hooks?limit=50"
	body, err := httpGet(ctx, listURL, c.header)
	if err != nil {
		return false, "", hookErr(err)
	}
	var hooks []struct {
		ID     int64 `json:"id"`
		Active bool  `json:"active"`
		Config struct {
			URL string `json:"url"`
		} `json:"config"`
	}
	if err := decodeJSON(body, &hooks); err != nil {
		return false, "", err
	}
	// 删除所有指向本服务的旧 hook（含历史重复项），再用最新配置重建。
	replaced := false
	for _, h := range hooks {
		if sameHookURL(h.Config.URL, targetURL) {
			if err := httpDelete(ctx, c.base+"/api/v1/repos/"+repoFullName+"/hooks/"+fmt.Sprintf("%d", h.ID), c.header); err != nil {
				return false, "", hookErr(err)
			}
			replaced = true
		}
	}
	cfg := map[string]string{
		"url":          targetURL,
		"content_type": "json",
		"secret":       secret,
	}
	// Gitea 顶层 branch_filter（glob）在较新版本支持平台侧只推该分支；
	// 旧版（如 1.18.x）会静默忽略该字段，因此非默认分支的权威过滤始终由服务端兜底。
	payload := map[string]any{
		"type":   "gitea",
		"active": true,
		"events": []string{"push"},
		"config": cfg,
	}
	if branchFilter != "" {
		payload["branch_filter"] = branchFilter
	}
	rb, err := httpPost(ctx, c.base+"/api/v1/repos/"+repoFullName+"/hooks", c.header, mustJSON(payload))
	if err != nil {
		return false, "", hookErr(err)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rb, &created); err != nil {
		return !replaced, "", nil
	}
	return !replaced, fmt.Sprintf("%d", created.ID), nil
}

func (c *giteaClient) injectToken(cloneURL string) string {
	if c.token == "" {
		return cloneURL
	}
	u, err := url.Parse(cloneURL)
	if err != nil || u.Host == "" {
		return cloneURL
	}
	u.User = url.UserPassword(c.token, "")
	return u.String()
}
