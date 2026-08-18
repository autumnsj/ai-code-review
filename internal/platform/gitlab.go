package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type gitLabClient struct {
	base   string
	token  string
	header http.Header
}

func newGitLabClient(baseURL, token string) *gitLabClient {
	base := trimSlash(baseURL)
	if base == "" {
		base = "https://gitlab.com"
	}
	h := http.Header{}
	h.Set("PRIVATE-TOKEN", token)
	return &gitLabClient{base: base, token: token, header: h}
}

func (c *gitLabClient) Me(ctx context.Context) (string, error) {
	body, err := httpGet(ctx, c.base+"/api/v4/user", c.header)
	if err != nil {
		return "", err
	}
	var u struct {
		Username string `json:"username"`
	}
	if err := decodeJSON(body, &u); err != nil {
		return "", err
	}
	return u.Username, nil
}

func (c *gitLabClient) ListRepos(ctx context.Context) ([]RepoInfo, error) {
	var out []RepoInfo
	for page := 1; page <= 50; page++ {
		u := setQuery(c.base+"/api/v4/projects", map[string]string{
			"membership": "true", "per_page": "100", "page": itoa(page),
		})
		body, err := httpGet(ctx, u, c.header)
		if err != nil {
			return nil, err
		}
		var projects []struct {
			PathWithNamespace string `json:"path_with_namespace"`
			HTTPURL           string `json:"http_url_to_repo"`
			WebURL            string `json:"web_url"`
			DefaultBranch     string `json:"default_branch"`
			Visibility        string `json:"visibility"`
		}
		if err := decodeJSON(body, &projects); err != nil {
			return nil, err
		}
		for _, p := range projects {
			out = append(out, RepoInfo{
				Name:          p.PathWithNamespace,
				CloneURL:      p.HTTPURL,
				WebURL:        p.WebURL,
				DefaultBranch: p.DefaultBranch,
				Private:       p.Visibility != "public",
			})
		}
		if len(projects) < 100 {
			break
		}
	}
	return out, nil
}

func (c *gitLabClient) GetRepo(ctx context.Context, repoFullName string) (RepoInfo, error) {
	pid := url.PathEscape(repoFullName)
	body, err := httpGet(ctx, c.base+"/api/v4/projects/"+pid, c.header)
	if err != nil {
		return RepoInfo{}, err
	}
	var p struct {
		PathWithNamespace string `json:"path_with_namespace"`
		HTTPURL           string `json:"http_url_to_repo"`
		WebURL            string `json:"web_url"`
		DefaultBranch     string `json:"default_branch"`
		Visibility        string `json:"visibility"`
	}
	if err := decodeJSON(body, &p); err != nil {
		return RepoInfo{}, err
	}
	return RepoInfo{
		Name: p.PathWithNamespace, CloneURL: p.HTTPURL, WebURL: p.WebURL,
		DefaultBranch: p.DefaultBranch, Private: p.Visibility != "public",
	}, nil
}

func (c *gitLabClient) ResolveCommit(ctx context.Context, repoFullName, ref string) (string, error) {
	if isSHA(ref) {
		return ref, nil
	}
	// GitLab 用 URL-encoded path 作为项目 id；branch 接口返回 commit.id。
	pid := url.PathEscape(repoFullName)
	u := c.base + "/api/v4/projects/" + pid + "/repository/branches/" + url.PathEscape(ref)
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

func (c *gitLabClient) EnsureWebhook(ctx context.Context, repoFullName, targetURL, secret, branchFilter string) (bool, string, error) {
	pid := url.PathEscape(repoFullName)
	body, err := httpGet(ctx, c.base+"/api/v4/projects/"+pid+"/hooks?per_page=100", c.header)
	if err != nil {
		return false, "", hookErr(err)
	}
	var hooks []struct {
		ID   int64  `json:"id"`
		URL  string `json:"url"`
		Push bool   `json:"push_events"`
	}
	if err := decodeJSON(body, &hooks); err != nil {
		return false, "", err
	}
	// 删除所有指向本服务的旧 hook（含历史重复项），再用最新配置重建。
	replaced := false
	for _, h := range hooks {
		if h.Push && sameHookURL(h.URL, targetURL) {
			if err := httpDelete(ctx, c.base+"/api/v4/projects/"+pid+"/hooks/"+fmt.Sprintf("%d", h.ID), c.header); err != nil {
				return false, "", hookErr(err)
			}
			replaced = true
		}
	}
	payload := map[string]any{
		"url":         targetURL,
		"push_events": true,
		"token":       secret,
	}
	// GitLab 顶层 push_events_branch_filter 用通配符过滤；单分支名即精确匹配。
	if branchFilter != "" {
		payload["push_events_branch_filter"] = branchFilter
	}
	rb, err := httpPost(ctx, c.base+"/api/v4/projects/"+pid+"/hooks", c.header, mustJSON(payload))
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

func (c *gitLabClient) injectToken(cloneURL string) string {
	if c.token == "" {
		return cloneURL
	}
	u, err := url.Parse(cloneURL)
	if err != nil || u.Host == "" {
		return cloneURL
	}
	// GitLab 兼容 oauth2:token 形式
	u.User = url.UserPassword("oauth2", c.token)
	return u.String()
}
