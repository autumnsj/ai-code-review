package webhook

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ai-code-review/aicr/internal/domain"
)

type giteeParser struct{}

func (p *giteeParser) Match(r *http.Request) bool {
	return r.Header.Get("X-Gitee-Event") != ""
}

// Verify 校验 X-Gitee-Token（Webhook 配置的密码，明文回传）。
func (p *giteeParser) Verify(secret string, body []byte, r *http.Request) bool {
	if secret == "" {
		return true
	}
	return constantTimeEqual(r.Header.Get("X-Gitee-Token"), secret)
}

func (p *giteeParser) Parse(raw []byte, r *http.Request) (*domain.WebhookEvent, error) {
	switch r.Header.Get("X-Gitee-Event") {
	case "Push Hook", "Tag Push Hook":
		return p.parsePush(raw)
	case "Merge Request Hook":
		return p.parseMR(raw)
	default:
		return nil, ErrIgnoredEvent
	}
}

func (p *giteeParser) parsePush(raw []byte) (*domain.WebhookEvent, error) {
	var payload struct {
		Ref       string `json:"ref"`
		After     string `json:"after"`
		Before    string `json:"before"`
		UserName  string `json:"user_name"`
		Project   struct {
			GitHTTPURL    string `json:"git_http_url"`
			HTMLURL       string `json:"html_url"`
			PathNamespace string `json:"path_with_namespace"`
			DefaultBranch string `json:"default_branch"`
		} `json:"project"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if strings.Trim(payload.After, "0") == "" {
		return nil, ErrIgnoredEvent
	}
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	branch = strings.TrimPrefix(branch, "refs/tags/")
	return &domain.WebhookEvent{
		Provider:      domain.ProviderGitee,
		EventType:     "push",
		RepoURL:       payload.Project.GitHTTPURL,
		RepoWebURL:    payload.Project.HTMLURL,
		RepoName:      payload.Project.PathNamespace,
		DefaultBranch: payload.Project.DefaultBranch,
		CommitSHA:     payload.After,
		BaseSHA:       payload.Before,
		SourceRef:     branch,
		TargetRef:     branch,
		Author:        payload.UserName,
	}, nil
}

func (p *giteeParser) parseMR(raw []byte) (*domain.WebhookEvent, error) {
	var payload struct {
		Action       string `json:"action"`
		Number       int    `json:"number"`
		MergeRequest struct {
			IID          int    `json:"iid"`
			Title        string `json:"title"`
			URL          string `json:"url"`
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
			ID           int    `json:"id"`
			LastCommit   struct {
				ID string `json:"id"`
			} `json:"last_commit"`
		} `json:"merge_request"`
		PullRequest struct {
			IID          int    `json:"iid"`
			Title        string `json:"title"`
			URL          string `json:"url"`
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
			HTMLURL      string `json:"html_url"`
			Number       int    `json:"number"`
		} `json:"pull_request"`
		Project struct {
			GitHTTPURL    string `json:"git_http_url"`
			HTMLURL       string `json:"html_url"`
			PathNamespace string `json:"path_with_namespace"`
			DefaultBranch string `json:"default_branch"`
		} `json:"project"`
		User struct {
			Login string `json:"login"`
			Name  string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	switch payload.Action {
	case "open", "update", "reopen":
	default:
		return nil, ErrIgnoredEvent
	}
	mr := payload.MergeRequest
	if mr.IID == 0 {
		// 旧版字段兜底
		mr.IID = payload.PullRequest.Number
		mr.Title = payload.PullRequest.Title
		mr.URL = payload.PullRequest.HTMLURL
		mr.SourceBranch = payload.PullRequest.SourceBranch
		mr.TargetBranch = payload.PullRequest.TargetBranch
	}
	author := payload.User.Login
	if author == "" {
		author = payload.User.Name
	}
	return &domain.WebhookEvent{
		Provider:      domain.ProviderGitee,
		EventType:     "pull_request",
		RepoURL:       payload.Project.GitHTTPURL,
		RepoWebURL:    payload.Project.HTMLURL,
		RepoName:      payload.Project.PathNamespace,
		DefaultBranch: payload.Project.DefaultBranch,
		CommitSHA:     mr.LastCommit.ID,
		TargetRef:     mr.TargetBranch,
		SourceRef:     mr.SourceBranch,
		PRNumber:      mr.IID,
		PRTitle:       mr.Title,
		PRURL:         mr.URL,
		Author:        author,
	}, nil
}
