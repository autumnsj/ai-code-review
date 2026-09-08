// Package notifier 把审查结果推送到企业微信/飞书/钉钉机器人。
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

// formatAuthor 组合通知里展示的作者名：有成员备注时显示「真实姓名（账号）」，
// 否则回退「git 提交名 <账号>」，再退裸账号。
func formatAuthor(account, gitName, memberName string) string {
	account = strings.TrimSpace(account)
	memberName = strings.TrimSpace(memberName)
	if memberName != "" {
		if account != "" {
			return fmt.Sprintf("%s（%s）", memberName, account)
		}
		return memberName
	}
	gitName = strings.TrimSpace(gitName)
	if gitName != "" && account != "" {
		return fmt.Sprintf("%s <%s>", gitName, account)
	}
	if gitName != "" {
		return gitName
	}
	return account
}

// BuildMarkdown 生成审查通知的 markdown 内容。memberName 为成员备注的真实姓名（可为空）。
func BuildMarkdown(r *domain.Review, findings []*domain.Finding, reportURL, memberName string) string {
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
	md += fmt.Sprintf("**作者**：%s\n", formatAuthor(r.Author, "", memberName))
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
// memberName 为成员备注的真实姓名（可为空）。
func BuildAuthorMarkdown(r *domain.Review, ar *domain.ReviewAuthorReport, findings []*domain.Finding, reportURL, memberName string) string {
	md := "## ✅ 代码审查报告\n"
	md += fmt.Sprintf("**仓库**：[%s](%s)\n", r.RepoName, reportURL)
	if r.PRTitle != "" {
		md += fmt.Sprintf("**PR**：%s\n", r.PRTitle)
	}
	md += fmt.Sprintf("**Commit**：`%s`\n", shortSHA(r.CommitSHA))
	md += fmt.Sprintf("**提交者**：%s\n", formatAuthor(ar.Author, ar.AuthorName, memberName))
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

// ReportReview 日报/周报工作清单中的一条审查。
// Author 为已格式化的展示名；Title 为功能标题（PR 标题/commit 标题，空时回退分支）；
// Desc 为功能概述（AI summary 首句，可空）。
type ReportReview struct {
	Repo   string
	Title  string
	Desc   string
	Ref    string
	Commit string
	Author string
	Score  int
	Status string // succeeded | failed
	Error  string
}

// ReportFinding 日报/周报「重点问题」清单中的一条（critical/high）。
// Location 为已截短的「文件:行」；Author 为已格式化的责任人展示名（可空）。
type ReportFinding struct {
	Repo     string
	Severity string
	Location string
	Title    string
	Author   string
}

// reportPeriod 返回报告类型对应的中文词与统计周期文案（窗口 [start, end)，北京时间）。
func reportPeriod(kind string, start, end time.Time) (word, period string) {
	word, period = "日报", start.Format("2006-01-02")+"（全天）"
	if kind == "weekly" {
		word = "周报"
		period = start.Format("2006-01-02") + " ～ " + end.AddDate(0, 0, -1).Format("2006-01-02")
	}
	return word, period
}

// BuildPersonalReportMarkdown 生成某位成员的个人工作日报/周报（考核对人、一人一份）。
// reviews/findings 均已归属到该成员：功能清单 + 其名下重点问题，不含他人内容。
func BuildPersonalReportMarkdown(kind string, start, end time.Time, author string, reviews []ReportReview, findings []ReportFinding) string {
	word, period := reportPeriod(kind, start, end)
	md := fmt.Sprintf("## 📝 %s 的工作%s（%s）\n", author, word, start.Format("2006-01-02"))
	md += fmt.Sprintf("**统计周期**：%s（北京时间）\n", period)

	n := len(reviews)
	avg := 0
	if n > 0 {
		sum := 0
		for _, r := range reviews {
			sum += r.Score
		}
		avg = int(math.Round(float64(sum) / float64(n)))
	}
	crit, high := 0, 0
	for _, f := range findings {
		if f.Severity == "critical" {
			crit++
		} else {
			high++
		}
	}
	md += fmt.Sprintf("> 本期完成 **%d** 个功能，平均 <font color=\"%s\">**%d**</font> 分；重点问题 🔴 %d / 🟠 %d\n",
		n, scoreColor(avg), avg, crit, high)

	md += "\n**🧑‍💻 本期完成功能**\n"
	for _, r := range reviews {
		subject := truncateRune(firstNonEmpty(r.Title, r.Ref), 42)
		line := fmt.Sprintf("- ✅ [%s] %s", r.Repo, subject)
		if desc := truncateRune(r.Desc, 70); desc != "" {
			line += "：" + desc
		}
		line += fmt.Sprintf("（<font color=\"%s\">**%d**</font> 分）\n", scoreColor(r.Score), r.Score)
		md += line
	}

	if len(findings) > 0 {
		md += "\n**🚨 需关注的重点问题**\n"
		for _, f := range findings {
			mark := "🟠"
			if f.Severity == "critical" {
				mark = "🔴"
			}
			md += fmt.Sprintf("- %s **[%s]** `%s` %s\n", mark, f.Repo, f.Location, truncateRune(f.Title, 46))
		}
	}
	return md
}

// BuildTeamNoticeMarkdown 生成团队级简报/异常提醒（不归属任何个人考核报告）。
// hasAnyReview=false（窗口内一条审查都没有）→ 存活简报；有失败审查或无主重点问题
// （无法 blame 到作者）→ 异常提醒；都没有则返回空串表示无需发送。
func BuildTeamNoticeMarkdown(kind string, start, end time.Time, failed []ReportReview, orphanFindings []ReportFinding, hasAnyReview bool) string {
	word, period := reportPeriod(kind, start, end)
	periodLine := fmt.Sprintf("**统计周期**：%s（北京时间）\n", period)

	if !hasAnyReview {
		return fmt.Sprintf("## 📝 团队工作%s（%s）\n%s\n本周期暂无代码审查记录。\n",
			word, start.Format("2006-01-02"), periodLine)
	}
	if len(failed) == 0 && len(orphanFindings) == 0 {
		return ""
	}
	md := fmt.Sprintf("## ⚠️ 工作%s · 本期异常提醒（%s）\n%s\n", word, start.Format("2006-01-02"), periodLine)
	if len(failed) > 0 {
		md += "\n**❌ 审查失败**\n"
		for _, r := range failed {
			commit := r.Commit
			if len(commit) > 8 {
				commit = commit[:8]
			}
			md += fmt.Sprintf("- [%s] %s `%s`：%s\n",
				r.Repo, firstNonEmpty(r.Ref, "-"), commit, truncateRune(r.Error, 50))
		}
	}
	if len(orphanFindings) > 0 {
		md += "\n**🚨 未归属责任人的重点问题**\n"
		for _, f := range orphanFindings {
			mark := "🟠"
			if f.Severity == "critical" {
				mark = "🔴"
			}
			md += fmt.Sprintf("- %s **[%s]** `%s` %s\n", mark, f.Repo, f.Location, truncateRune(f.Title, 46))
		}
	}
	return md
}

// firstNonEmpty 返回第一个去空白后非空的字符串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// truncateRune 按 rune 截断字符串，超长加省略号。
func truncateRune(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
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
