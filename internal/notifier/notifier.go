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
	"strconv"
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

// ReportTotals 定时报告一个统计周期内的总览指标。
type ReportTotals struct {
	ReviewCount  int64
	Succeeded    int64
	Failed       int64
	AvgScore     float64
	Additions    int64
	Deletions    int64
	FilesChanged int64
	Critical     int64
	High         int64
	Medium       int64
	Low          int64
}

// ReportAuthor 定时报告榜单中的一行作者（Name 已格式化为「真名（账号）」或裸账号）。
type ReportAuthor struct {
	Name      string
	Reviews   int64
	AvgScore  float64
	Additions int64
	Deletions int64
	Findings  int64
	Critical  int64
	High      int64
}

// BuildReportMarkdown 生成定时日报/周报的 markdown。窗口为 [start, end)（北京时间），
// kind 为 "daily" 或 "weekly"；三个榜单分别为评分/代码量/问题数 Top N。
func BuildReportMarkdown(kind string, start, end time.Time, t ReportTotals, topScore, topChurn, topFindings []ReportAuthor) string {
	titleWord, period := "日报", start.Format("2006-01-02")+"（全天）"
	if kind == "weekly" {
		titleWord = "周报"
		period = start.Format("2006-01-02") + " ～ " + end.AddDate(0, 0, -1).Format("2006-01-02")
	}
	md := fmt.Sprintf("## 📊 代码审查%s（%s）\n", titleWord, start.Format("2006-01-02"))
	md += fmt.Sprintf("**统计周期**：%s（北京时间）\n", period)

	if t.ReviewCount == 0 {
		md += "\n本周期暂无已完成的代码审查记录。\n"
		return md
	}

	md += fmt.Sprintf("> 审查 **%s** 次：✅ %s 成功 ｜ ❌ %s 失败\n",
		formatInt(t.ReviewCount), formatInt(t.Succeeded), formatInt(t.Failed))
	md += fmt.Sprintf("> 平均综合评分 <font color=\"%s\">**%.1f**</font> ｜ 改动 **+%s / -%s**（%s 个文件）\n",
		scoreColor(int(t.AvgScore)), t.AvgScore, formatInt(t.Additions), formatInt(t.Deletions), formatInt(t.FilesChanged))
	md += fmt.Sprintf("> 发现问题 **%s** 个：critical %s ｜ high %s ｜ medium %s ｜ low %s\n",
		formatInt(t.Critical+t.High+t.Medium+t.Low),
		formatInt(t.Critical), formatInt(t.High), formatInt(t.Medium), formatInt(t.Low))

	if len(topScore) > 0 {
		md += "\n**🏆 综合评分 Top " + itoa(len(topScore)) + "**\n"
		for i, a := range topScore {
			md += fmt.Sprintf("%d. %s：**%.1f** 分（%s 次审查）\n", i+1, a.Name, a.AvgScore, formatInt(a.Reviews))
		}
	}
	if len(topChurn) > 0 {
		md += "\n**💻 代码量 Top " + itoa(len(topChurn)) + "**\n"
		for i, a := range topChurn {
			md += fmt.Sprintf("%d. %s：+%s / -%s（%s 次审查）\n",
				i+1, a.Name, formatInt(a.Additions), formatInt(a.Deletions), formatInt(a.Reviews))
		}
	}
	if len(topFindings) > 0 {
		md += "\n**🔍 问题数 Top " + itoa(len(topFindings)) + "**\n"
		for i, a := range topFindings {
			suffix := ""
			if a.Critical > 0 || a.High > 0 {
				suffix = fmt.Sprintf("（critical %s，high %s）", formatInt(a.Critical), formatInt(a.High))
			}
			md += fmt.Sprintf("%d. %s：%s 个%s\n", i+1, a.Name, formatInt(a.Findings), suffix)
		}
	}
	return md
}

// formatInt 千分位格式化整数。
func formatInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func itoa(n int) string { return strconv.Itoa(n) }

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
