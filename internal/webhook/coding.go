package webhook

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ai-code-review/aicr/internal/domain"
)

type codingParser struct{}

func (p *codingParser) Match(r *http.Request) bool {
	return r.Header.Get("X-Coding-Event") != ""
}

// Verify 校验 X-Coding-Signature（格式 sha256=<hex>，使用 webhook token 做 HMAC-SHA256）。
func (p *codingParser) Verify(secret string, body []byte, r *http.Request) bool {
	if secret == "" {
		return true
	}
	sig := r.Header.Get("X-Coding-Signature")
	if strings.HasPrefix(sig, "sha256=") {
		sig = strings.TrimPrefix(sig, "sha256=")
	}
	return verifyHMACHex("sha256", sig, secret, body)
}

func (p *codingParser) Parse(raw []byte, r *http.Request) (*domain.WebhookEvent, error) {
	event := r.Header.Get("X-Coding-Event")
	switch event {
	case "push":
		return p.parsePush(raw)
	case "merge_request", "pull_request":
		return p.parseMR(raw)
	default:
		// 兜底：从 body 的 object_kind / event_key 判断
		var head struct {
			ObjectKind string `json:"object_kind"`
			EventKey   string `json:"event_key"`
		}
		_ = json.Unmarshal(raw, &head)
		switch {
		case head.ObjectKind == "merge_request":
			return p.parseMR(raw)
		case head.EventKey == "push" || strings.HasPrefix(head.EventKey, "push:"):
			return p.parsePush(raw)
		default:
			return nil, ErrIgnoredEvent
		}
	}
}

func (p *codingParser) parsePush(raw []byte) (*domain.WebhookEvent, error) {
	var payload struct {
		Ref     string `json:"ref"`
		After   string `json:"after"`
		Before  string `json:"before"`
		Deleted bool   `json:"deleted"`
		Pusher  struct {
			Name string `json:"name"`
		} `json:"pusher"`
		Repository struct {
			HTTPURL       string `json:"http_url"`
			HTTPSURL      string `json:"https"`
			WebURL        string `json:"web_url"`
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.Deleted || strings.Trim(payload.After, "0") == "" {
		return nil, ErrIgnoredEvent
	}
	cloneURL := payload.Repository.HTTPSURL
	if cloneURL == "" {
		cloneURL = payload.Repository.HTTPURL
	}
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	return &domain.WebhookEvent{
		Provider:      domain.ProviderCoding,
		EventType:     "push",
		RepoURL:       cloneURL,
		RepoWebURL:    payload.Repository.WebURL,
		RepoName:      payload.Repository.FullName,
		DefaultBranch: payload.Repository.DefaultBranch,
		CommitSHA:     payload.After,
		BaseSHA:       payload.Before,
		SourceRef:     branch,
		TargetRef:     branch,
		Author:        payload.Pusher.Name,
	}, nil
}

func (p *codingParser) parseMR(raw []byte) (*domain.WebhookEvent, error) {
	var payload struct {
		ObjectAttributes struct {
			Action         string `json:"action"`
			IID            int    `json:"iid"`
			Number         int    `json:"number"`
			Title          string `json:"title"`
			URL            string `json:"url"`
			SourceBranch   string `json:"source_branch"`
			TargetBranch   string `json:"target_branch"`
			HeadCommit     string `json:"head_commit_sha"`
			LastCommit     struct {
				Sha string `json:"sha"`
			} `json:"last_commit"`
			MergeCommitSHA string `json:"merge_commit_sha"`
		} `json:"object_attributes"`
		Repository struct {
			HTTPURL       string `json:"http_url"`
			HTTPSURL      string `json:"https"`
			WebURL        string `json:"web_url"`
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
		} `json:"repository"`
		User struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	switch payload.ObjectAttributes.Action {
	case "create", "update", "synchronize", "open", "reopen":
	default:
		return nil, ErrIgnoredEvent
	}
	cloneURL := payload.Repository.HTTPSURL
	if cloneURL == "" {
		cloneURL = payload.Repository.HTTPURL
	}
	sha := payload.ObjectAttributes.HeadCommit
	if sha == "" {
		sha = payload.ObjectAttributes.LastCommit.Sha
	}
	iid := payload.ObjectAttributes.IID
	if iid == 0 {
		iid = payload.ObjectAttributes.Number
	}
	return &domain.WebhookEvent{
		Provider:      domain.ProviderCoding,
		EventType:     "merge_request",
		RepoURL:       cloneURL,
		RepoWebURL:    payload.Repository.WebURL,
		RepoName:      payload.Repository.FullName,
		DefaultBranch: payload.Repository.DefaultBranch,
		CommitSHA:     sha,
		TargetRef:     payload.ObjectAttributes.TargetBranch,
		SourceRef:     payload.ObjectAttributes.SourceBranch,
		PRNumber:      iid,
		PRTitle:       payload.ObjectAttributes.Title,
		PRURL:         payload.ObjectAttributes.URL,
		Author:        payload.User.Name,
	}, nil
}
