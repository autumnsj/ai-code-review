package webhook

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ai-code-review/aicr/internal/domain"
)

type gitlabParser struct{}

func (p *gitlabParser) Match(r *http.Request) bool {
	return r.Header.Get("X-Gitlab-Event") != ""
}

// Verify 校验 X-Gitlab-Token（若配置了 secret）。
func (p *gitlabParser) Verify(secret string, body []byte, r *http.Request) bool {
	if secret == "" {
		return true
	}
	return constantTimeEqual(r.Header.Get("X-Gitlab-Token"), secret)
}

func (p *gitlabParser) Parse(raw []byte, r *http.Request) (*domain.WebhookEvent, error) {
	switch r.Header.Get("X-Gitlab-Event") {
	case "Push Hook", "Tag Push Hook":
		return p.parsePush(raw)
	case "Merge Request Hook":
		return p.parseMR(raw)
	default:
		return nil, ErrIgnoredEvent
	}
}

func (p *gitlabParser) parsePush(raw []byte) (*domain.WebhookEvent, error) {
	var payload struct {
		Ref        string `json:"ref"`
		After      string `json:"after"`
		Before     string `json:"before"`
		UserName   string `json:"user_name"`
		Project    struct {
			GitHTTPURL    string `json:"git_http_url"`
			WebURL        string `json:"web_url"`
			PathNamespace string `json:"path_with_namespace"`
			DefaultBranch string `json:"default_branch"`
		} `json:"project"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if strings.Trim(payload.After, "0") == "" {
		return nil, ErrIgnoredEvent // 删除分支/标签
	}
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	branch = strings.TrimPrefix(branch, "refs/tags/")
	author := payload.UserName
	return &domain.WebhookEvent{
		Provider:      domain.ProviderGitLab,
		EventType:     "push",
		RepoURL:       payload.Project.GitHTTPURL,
		RepoWebURL:    payload.Project.WebURL,
		RepoName:      payload.Project.PathNamespace,
		DefaultBranch: payload.Project.DefaultBranch,
		CommitSHA:     payload.After,
		BaseSHA:       payload.Before,
		SourceRef:     branch,
		TargetRef:     branch,
		Author:        author,
	}, nil
}

func (p *gitlabParser) parseMR(raw []byte) (*domain.WebhookEvent, error) {
	var payload struct {
		ObjectAttributes struct {
			Action       string `json:"action"`
			IID          int    `json:"iid"`
			Title        string `json:"title"`
			URL          string `json:"url"`
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
			LastCommit   struct {
				ID string `json:"id"`
			} `json:"last_commit"`
		} `json:"object_attributes"`
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Project struct {
			GitHTTPURL    string `json:"git_http_url"`
			WebURL        string `json:"web_url"`
			PathNamespace    string `json:"path_with_namespace"`
			DefaultBranch string `json:"default_branch"`
		} `json:"project"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	switch payload.ObjectAttributes.Action {
	case "open", "update", "reopen":
	default:
		return nil, ErrIgnoredEvent
	}
	return &domain.WebhookEvent{
		Provider:      domain.ProviderGitLab,
		EventType:     "merge_request",
		RepoURL:       payload.Project.GitHTTPURL,
		RepoWebURL:    payload.Project.WebURL,
		RepoName:      payload.Project.PathNamespace,
		DefaultBranch: payload.Project.DefaultBranch,
		CommitSHA:     payload.ObjectAttributes.LastCommit.ID,
		TargetRef:     payload.ObjectAttributes.TargetBranch,
		SourceRef:     payload.ObjectAttributes.SourceBranch,
		PRNumber:      payload.ObjectAttributes.IID,
		PRTitle:       payload.ObjectAttributes.Title,
		PRURL:         payload.ObjectAttributes.URL,
		Author:        payload.User.Username,
	}, nil
}
