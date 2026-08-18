package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type gitHubClient struct {
	base   string
	token  string
	header http.Header
}

func newGitHubClient(baseURL, token string) *gitHubClient {
	base := trimSlash(baseURL)
	if base == "" {
		base = "https://api.github.com"
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	h.Set("Accept", "application/vnd.github+json")
	h.Set("X-GitHub-Api-Version", "2022-11-28")
	return &gitHubClient{base: base, token: token, header: h}
}

func (c *gitHubClient) Me(ctx context.Context) (string, error) {
	body, err := httpGet(ctx, c.base+"/user", c.header)
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

func (c *gitHubClient) ListRepos(ctx context.Context) ([]RepoInfo, error) {
	var out []RepoInfo
	for page := 1; page <= 50; page++ {
		u := setQuery(c.base+"/user/repos", map[string]string{
			"per_page": "100", "page": itoa(page), "sort": "updated",
		})
		body, err := httpGet(ctx, u, c.header)
		if err != nil {
			return nil, err
		}
		var repos []struct {
			FullName string `json:"full_name"`
			CloneURL string `json:"clone_url"`
			HTMLURL  string `json:"html_url"`
			Default  string `json:"default_branch"`
			Private  bool   `json:"private"`
		}
		if err := decodeJSON(body, &repos); err != nil {
			return nil, err
		}
		for _, r := range repos {
			out = append(out, RepoInfo{
				Name: r.FullName, CloneURL: r.CloneURL, WebURL: r.HTMLURL,
				DefaultBranch: r.Default, Private: r.Private,
			})
		}
		if len(repos) < 100 {
			break
		}
	}
	return out, nil
}

func (c *gitHubClient) GetRepo(ctx context.Context, repoFullName string) (RepoInfo, error) {
	body, err := httpGet(ctx, c.base+"/repos/"+repoFullName, c.header)
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

func (c *gitHubClient) ResolveCommit(ctx context.Context, repoFullName, ref string) (string, error) {
	if isSHA(ref) {
		return ref, nil
	}
	u := c.base + "/repos/" + repoFullName + "/commits/" + url.PathEscape(ref)
	body, err := httpGet(ctx, u, c.header)
	if err != nil {
		return "", err
	}
	var cmt struct {
		SHA string `json:"sha"`
	}
	if err := decodeJSON(body, &cmt); err != nil {
		return "", err
	}
	return cmt.SHA, nil
}

func (c *gitHubClient) EnsureWebhook(ctx context.Context, repoFullName, targetURL, secret, _ string) (bool, string, error) {
	listURL := c.base + "/repos/" + repoFullName + "/hooks?per_page=100"
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
	// GitHub webhook 不支持按分支过滤；删除所有指向本服务的旧 hook 后重建。
	replaced := false
	for _, h := range hooks {
		if h.Active && sameHookURL(h.Config.URL, targetURL) {
			if err := httpDelete(ctx, c.base+"/repos/"+repoFullName+"/hooks/"+fmt.Sprintf("%d", h.ID), c.header); err != nil {
				return false, "", hookErr(err)
			}
			replaced = true
		}
	}
	payload := map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": map[string]string{
			"url":          targetURL,
			"content_type": "json",
			"secret":       secret,
		},
	}
	rb, err := httpPost(ctx, c.base+"/repos/"+repoFullName+"/hooks", c.header, mustJSON(payload))
	if err != nil {
		return false, "", hookErr(err)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rb, &created); err != nil {
		return !replaced, "", nil // 创建成功但解析 id 失败，不影响结果
	}
	return !replaced, fmt.Sprintf("%d", created.ID), nil
}

// injectToken 把 token 嵌入 https clone URL，供服务端 clone 时鉴权；前端展示前需脱敏。
func (c *gitHubClient) injectToken(cloneURL string) string {
	if c.token == "" {
		return cloneURL
	}
	u, err := url.Parse(cloneURL)
	if err != nil || u.Host == "" {
		return cloneURL
	}
	u.User = url.UserPassword("x-access-token", c.token)
	return u.String()
}

func isSHA(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
