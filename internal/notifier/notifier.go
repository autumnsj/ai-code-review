// Package notifier 把审查结果推送到企业微信/飞书/钉钉机器人。
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ai-code-review/aicr/internal/domain"
)

// Channel 一个通知渠道配置。
type Channel struct {
	Type       string `json:"type"`        // wecom | feishu | dingtalk
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret"`      // 加签密钥（钉钉/飞书）
	Enabled    bool   `json:"enabled"`
}

// Notifier 发送单条通知。
type Notifier interface {
	Send(ctx context.Context, title, markdown string) error
}

// New 根据渠道类型构造 Notifier。
func New(ch Channel) (Notifier, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	switch ch.Type {
	case "wecom":
		return &weComNotifier{webhook: ch.WebhookURL, client: client}, nil
	case "dingtalk":
		return &dingTalkNotifier{webhook: ch.WebhookURL, secret: ch.Secret, client: client}, nil
	case "feishu":
		return &feiShuNotifier{webhook: ch.WebhookURL, secret: ch.Secret, client: client}, nil
	default:
		return nil, fmt.Errorf("unknown notifier type: %s", ch.Type)
	}
}

func postJSON(ctx context.Context, client *http.Client, url string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}
	// 各平台成功时 body 含 errcode:0 / StatusCode:0 / code:0，简单检查下
	if bytes.Contains(respBody, []byte(`"errcode":0`)) || bytes.Contains(respBody, []byte(`"StatusCode":0`)) ||
		bytes.Contains(respBody, []byte(`"code":0`)) || bytes.Contains(respBody, []byte(`"StatusMessage":"success"`)) {
		return nil
	}
	// 没匹配到明确成功标识也不报错（有些平台响应结构不同）
	return nil
}

// BuildMarkdown 生成审查通知的 markdown 内容。
func BuildMarkdown(r *domain.Review, findings []*domain.Finding, reportURL string) string {
	status := "✅ 审查完成"
	if r.Status == "failed" {
		status = "❌ 审查失败"
	}
	score := fmt.Sprintf("%d", r.ScoreTotal)
	md := fmt.Sprintf("## %s\n", status)
	md += fmt.Sprintf("**仓库**：[%s](%s)\n", r.RepoName, reportURL)
	if r.PRTitle != "" {
		md += fmt.Sprintf("**PR**：%s\n", r.PRTitle)
	}
	md += fmt.Sprintf("**Commit**：`%s`\n", shortSHA(r.CommitSHA))
	md += fmt.Sprintf("**作者**：%s\n", r.Author)
	if r.Status == "succeeded" {
		md += fmt.Sprintf("**综合评分**：<font color=\"%s\">**%s**</font>\n", scoreColor(r.ScoreTotal), score)
		if len(r.ScoreDimensions) > 0 {
			keys := make([]string, 0, len(r.ScoreDimensions))
			for k := range r.ScoreDimensions {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				d := r.ScoreDimensions[k]
				label := d.Label
				if label == "" {
					label = k
				}
				parts = append(parts, fmt.Sprintf("%s %d", label, d.Score))
			}
			md += "> " + strings.Join(parts, " ｜ ") + "\n"
		} else {
			md += fmt.Sprintf("> 架构 %d ｜ 质量 %d ｜ 安全 %d ｜ 可维护 %d\n",
				r.ScoreArch, r.ScoreQuality, r.ScoreSecurity, r.ScoreMaint)
		}
		if r.Summary != "" {
			md += "\n" + r.Summary + "\n"
		}
		// 列出最高严重度的问题（最多 5 条）
		if top := topFindings(findings, 5); len(top) > 0 {
			md += "\n**主要问题**：\n"
			for _, f := range top {
				md += fmt.Sprintf("- [%s] %s (`%s:%d`)\n", f.Severity, f.Title, f.FilePath, f.LineStart)
			}
		}
	} else if r.Error != "" {
		md += fmt.Sprintf("\n> 错误：%s\n", r.Error)
	}
	md += fmt.Sprintf("\n[查看完整报告](%s)", reportURL)
	return md
}

// BuildAuthorMarkdown 生成按作者拆分的审查通知 markdown（每位参与者一条）。
func BuildAuthorMarkdown(r *domain.Review, ar *domain.ReviewAuthorReport, findings []*domain.Finding, reportURL string) string {
	display := ar.AuthorName
	if display != "" && ar.Author != "" {
		display = fmt.Sprintf("%s <%s>", ar.AuthorName, ar.Author)
	} else if ar.Author != "" {
		display = ar.Author
	}
	md := "## ✅ 代码审查报告（你的提交）\n"
	md += fmt.Sprintf("**仓库**：[%s](%s)\n", r.RepoName, reportURL)
	if r.PRTitle != "" {
		md += fmt.Sprintf("**PR**：%s\n", r.PRTitle)
	}
	md += fmt.Sprintf("**Commit**：`%s`\n", shortSHA(r.CommitSHA))
	md += fmt.Sprintf("**提交者**：%s\n", display)
	md += fmt.Sprintf("**你的评分**：<font color=\"%s\">**%d**</font>\n", scoreColor(ar.ScoreTotal), ar.ScoreTotal)
	if len(ar.ScoreDimensions) > 0 {
		keys := make([]string, 0, len(ar.ScoreDimensions))
		for k := range ar.ScoreDimensions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			d := ar.ScoreDimensions[k]
			label := d.Label
			if label == "" {
				label = k
			}
			parts = append(parts, fmt.Sprintf("%s %d", label, d.Score))
		}
		md += "> " + strings.Join(parts, " ｜ ") + "\n"
	} else {
		md += fmt.Sprintf("> 架构 %d ｜ 质量 %d ｜ 安全 %d ｜ 可维护 %d\n",
			ar.ScoreArch, ar.ScoreQuality, ar.ScoreSecurity, ar.ScoreMaint)
	}
	md += fmt.Sprintf("> 改动 +%d / -%d，涉及 %d 个文件\n", ar.Additions, ar.Deletions, ar.FilesChanged)
	if top := topFindings(findings, 5); len(top) > 0 {
		md += "\n**需要你关注的问题**：\n"
		for _, f := range top {
			md += fmt.Sprintf("- [%s] %s (`%s:%d`)\n", f.Severity, f.Title, f.FilePath, f.LineStart)
		}
	}
	md += fmt.Sprintf("\n[查看你的完整报告](%s)", reportURL)
	return md
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func scoreColor(s int) string {
	switch {
	case s >= 80:
		return "info" // 企微 markdown 不支持绿色字，用 info
	case s >= 60:
		return "comment"
	default:
		return "warning"
	}
}

func topFindings(fs []*domain.Finding, n int) []*domain.Finding {
	out := make([]*domain.Finding, 0, n)
	for _, f := range fs {
		if f.Severity == "info" || f.Severity == "low" {
			continue
		}
		out = append(out, f)
		if len(out) >= n {
			break
		}
	}
	return out
}
