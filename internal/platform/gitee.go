package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

type giteeClient struct {
	base  string
	token string
}

func newGiteeClient(baseURL, token string) *giteeClient {
	base := trimSlash(baseURL)
	if base == "" {
		base = "https://gitee.com"
	}
	return &giteeClient{base: base, token: token}
}

func (c *giteeClient) Me(ctx context.Context) (string, error) {
	u := setQuery(c.base+"/api/v5/user", map[string]string{"access_token": c.token})
	body, err := httpGet(ctx, u, nil)
	if err != nil {
		return "", err
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := decodeJSON(body, &user); err != nil {
		return "", err
	}
	return user.Login, nil
}

func (c *giteeClient) ListRepos(ctx context.Context) ([]RepoInfo, error) {
	var out []RepoInfo
	for page := 1; page <= 50; page++ {
		u := setQuery(c.base+"/api/v5/user/repos", map[string]string{
			"access_token": c.token, "per_page": "100", "page": itoa(page),
		})
		body, err := httpGet(ctx, u, nil)
		if err != nil {
			return nil, err
		}
		var repos []struct {
			FullName string `json:"full_name"`
			HTMLURL  string `json:"html_url"`
			Default  string `json:"default_branch"`
			Private  bool   `json:"private"`
			// gitee 不直接返回 https clone url 字段名不一，用 html_url 拼
		}
		if err := decodeJSON(body, &repos); err != nil {
			return nil, err
		}
		for _, r := range repos {
			cloneURL := r.HTMLURL + ".git"
			out = append(out, RepoInfo{
				Name: r.FullName, CloneURL: cloneURL, WebURL: r.HTMLURL,
				DefaultBranch: r.Default, Private: r.Private,
			})
		}
		if len(repos) < 100 {
			break
		}
	}
	return out, nil
}

func (c *giteeClient) GetRepo(ctx context.Context, repoFullName string) (RepoInfo, error) {
	u := setQuery(c.base+"/api/v5/repos/"+repoFullName, map[string]string{"access_token": c.token})
	body, err := httpGet(ctx, u, nil)
	if err != nil {
		return RepoInfo{}, err
	}
	var r struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		Default  string `json:"default_branch"`
		Private  bool   `json:"private"`
	}
	if err := decodeJSON(body, &r); err != nil {
		return RepoInfo{}, err
	}
	return RepoInfo{
		Name: r.FullName, CloneURL: r.HTMLURL + ".git", WebURL: r.HTMLURL,
		DefaultBranch: r.Default, Private: r.Private,
	}, nil
}

func (c *giteeClient) ResolveCommit(ctx context.Context, repoFullName, ref string) (string, error) {
	if isSHA(ref) {
		return ref, nil
	}
	u := setQuery(c.base+"/api/v5/repos/"+repoFullName+"/branches/"+url.PathEscape(ref),
		map[string]string{"access_token": c.token})
	body, err := httpGet(ctx, u, nil)
	if err != nil {
		return "", err
	}
	var br struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := decodeJSON(body, &br); err != nil {
		return "", err
	}
	return br.Commit.SHA, nil
}

func (c *giteeClient) EnsureWebhook(ctx context.Context, repoFullName, targetURL, secret, _ string) (bool, string, error) {
	listURL := setQuery(c.base+"/api/v5/repos/"+repoFullName+"/hooks",
		map[string]string{"access_token": c.token, "per_page": "100"})
	body, err := httpGet(ctx, listURL, nil)
	if err != nil {
		return false, "", hookErr(err)
	}
	var hooks []struct {
		ID         int64  `json:"id"`
		URL        string `json:"url"`
		PushEvents bool   `json:"push_events"`
	}
	if err := decodeJSON(body, &hooks); err != nil {
		return false, "", err
	}
	// Gitee webhook 不支持按分支过滤；删除所有指向本服务的旧 hook 后重建。
	replaced := false
	for _, h := range hooks {
		if h.PushEvents && sameHookURL(h.URL, targetURL) {
			delURL := setQuery(c.base+"/api/v5/repos/"+repoFullName+"/hooks/"+fmt.Sprintf("%d", h.ID),
				map[string]string{"access_token": c.token})
			if err := httpDelete(ctx, delURL, nil); err != nil {
				return false, "", hookErr(err)
			}
			replaced = true
		}
	}
	payload := map[string]any{
		"url":         targetURL,
		"push_events": true,
		"password":    secret,
	}
	createURL := setQuery(c.base+"/api/v5/repos/"+repoFullName+"/hooks",
		map[string]string{"access_token": c.token})
	rb, err := httpPost(ctx, createURL, nil, mustJSON(payload))
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

func (c *giteeClient) injectToken(cloneURL string) string {
	if c.token == "" {
		return cloneURL
	}
	u, err := url.Parse(cloneURL)
	if err != nil || u.Host == "" {
		return cloneURL
	}
	u.User = url.UserPassword("oauth2", c.token)
	return u.String()
}
