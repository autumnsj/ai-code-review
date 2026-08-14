package webhook

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ai-code-review/aicr/internal/domain"
)

type githubParser struct{}

func (p *githubParser) Match(r *http.Request) bool {
	return r.Header.Get("X-GitHub-Event") != ""
}

// VerifyGitHub 校验 X-Hub-Signature-256（若配置了 secret）。
func (p *githubParser) Verify(secret string, body []byte, r *http.Request) bool {
	if secret == "" {
		return true
	}
	sig := r.Header.Get("X-Hub-Signature-256")
	if strings.HasPrefix(sig, "sha256=") {
		sig = strings.TrimPrefix(sig, "sha256=")
	}
	return verifyHMACHex("sha256", sig, secret, body)
}

func (p *githubParser) Parse(raw []byte, r *http.Request) (*domain.WebhookEvent, error) {
	event := r.Header.Get("X-GitHub-Event")
	switch event {
	case "push":
		return p.parsePush(raw)
	case "pull_request":
		return p.parsePR(raw)
	default:
		return nil, ErrIgnoredEvent
	}
}

func (p *githubParser) parsePush(raw []byte) (*domain.WebhookEvent, error) {
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
		HeadCommit struct {
			Author struct{ Username string `json:"username"` } `json:"author"`
		} `json:"head_commit"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	// 删除分支：after 全 0
	if strings.Trim(payload.After, "0") == "" {
		return nil, ErrIgnoredEvent
	}
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	return &domain.WebhookEvent{
		Provider:      domain.ProviderGitHub,
		EventType:     "push",
		RepoURL:       payload.Repository.CloneURL,
		RepoWebURL:    payload.Repository.HTMLURL,
		RepoName:      payload.Repository.FullName,
		DefaultBranch: payload.Repository.Default,
		CommitSHA:     payload.After,
		BaseSHA:       payload.Before,
		SourceRef:     branch,
		TargetRef:     branch,
		Author:        payload.HeadCommit.Author.Username,
	}, nil
}

func (p *githubParser) parsePR(raw []byte) (*domain.WebhookEvent, error) {
	var payload struct {
		Action string `json:"action"`
		PR     struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			HTMLURL string `json:"html_url"`
			Head   struct {
				SHA  string `json:"sha"`
				Ref  string `json:"ref"`
				Repo struct { CloneURL string `json:"clone_url"` } `json:"repo"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
			User struct{ Login string `json:"login"` } `json:"user"`
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
	case "opened", "synchronize", "reopened":
	default:
		return nil, ErrIgnoredEvent
	}
	return &domain.WebhookEvent{
		Provider:      domain.ProviderGitHub,
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
