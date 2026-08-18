package webhook

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ai-code-review/aicr/internal/domain"
)

// giteaParser 处理 Gitea webhook。payload 结构与 GitHub 高度相似，
// 签名头为 X-Gitea-Signature（HMAC-SHA256 hex），事件头为 X-Gitea-Event。
type giteaParser struct{}

func (p *giteaParser) Match(r *http.Request) bool {
	return r.Header.Get("X-Gitea-Event") != ""
}

func (p *giteaParser) Verify(secret string, body []byte, r *http.Request) bool {
	if secret == "" {
		return true
	}
	sig := r.Header.Get("X-Gitea-Signature")
	return verifyHMACHex("sha256", sig, secret, body)
}

func (p *giteaParser) Parse(raw []byte, r *http.Request) (*domain.WebhookEvent, error) {
	switch r.Header.Get("X-Gitea-Event") {
	case "push":
		return p.parsePush(raw)
	case "pull_request":
		return p.parsePR(raw)
	default:
		return nil, ErrIgnoredEvent
	}
}

func (p *giteaParser) parsePush(raw []byte) (*domain.WebhookEvent, error) {
	var payload struct {
		Ref        string `json:"ref"`
		After      string `json:"after"`
		Before     string `json:"before"`
		Repository struct {
			CloneURL string `json:"clone_url"`
			HTMLURL  string `json:"html_url"`
			FullName string `json:"full_name"`
			Default  string `json:"default_branch"`
		} `json:"repository"`
		Pusher struct {
			Login string `json:"login"`
		} `json:"pusher"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if strings.Trim(payload.After, "0") == "" {
		return nil, ErrIgnoredEvent
	}
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	return &domain.WebhookEvent{
		Provider:      domain.ProviderGitea,
		EventType:     "push",
		RepoURL:       payload.Repository.CloneURL,
		RepoWebURL:    payload.Repository.HTMLURL,
		RepoName:      payload.Repository.FullName,
		DefaultBranch: payload.Repository.Default,
		CommitSHA:     payload.After,
		BaseSHA:       payload.Before,
		SourceRef:     branch,
		TargetRef:     branch,
		Author:        payload.Pusher.Login,
	}, nil
}

func (p *giteaParser) parsePR(raw []byte) (*domain.WebhookEvent, error) {
	var payload struct {
		Action string `json:"action"`
		PR     struct {
			Number  int    `json:"number"`
			Title   string `json:"title"`
			HTMLURL string `json:"html_url"`
			Head    struct {
				SHA string `json:"sha"`
				Ref string `json:"ref"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"pull_request"`
		Repository struct {
			CloneURL string `json:"clone_url"`
			HTMLURL  string `json:"html_url"`
			FullName string `json:"full_name"`
			Default  string `json:"default_branch"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	switch payload.Action {
	case "opened", "synchronized", "reopened", "edited":
	default:
		return nil, ErrIgnoredEvent
	}
	return &domain.WebhookEvent{
		Provider:      domain.ProviderGitea,
		EventType:     "pull_request",
		RepoURL:       payload.Repository.CloneURL,
		RepoWebURL:    payload.Repository.HTMLURL,
		RepoName:      payload.Repository.FullName,
		DefaultBranch: payload.Repository.Default,
		CommitSHA:     payload.PR.Head.SHA,
		TargetRef:     payload.PR.Base.Ref,
		SourceRef:     payload.PR.Head.Ref,
		PRNumber:      payload.PR.Number,
		PRTitle:       payload.PR.Title,
		PRURL:         payload.PR.HTMLURL,
		Author:        payload.PR.User.Login,
	}, nil
}
