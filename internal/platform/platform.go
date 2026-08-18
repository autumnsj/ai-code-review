// Package platform 封装各 Git 托管平台的 REST API 客户端，
// 用于「用一个 Token 扫描账号可见仓库」和「把分支/引用解析为 commit SHA」。
package platform

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// RepoInfo 平台返回的仓库元数据。
type RepoInfo struct {
	Name          string `json:"name"`           // owner/repo
	CloneURL      string `json:"clone_url"`      // https clone 地址（不含 token）
	WebURL        string `json:"web_url"`        // 浏览器访问地址
	DefaultBranch string `json:"default_branch"` // 默认分支
	Private       bool   `json:"private"`
}

// Client 统一各平台 API。
type Client interface {
	// Me 返回 token 对应的登录名，用于校验 token 有效性。
	Me(ctx context.Context) (login string, err error)
	// ListRepos 列举 token 可见的所有仓库（自动分页）。
	ListRepos(ctx context.Context) ([]RepoInfo, error)
	// GetRepo 获取单个仓库的元数据（用于同步默认分支等）。repoFullName 为 owner/repo。
	GetRepo(ctx context.Context, repoFullName string) (RepoInfo, error)
	// ResolveCommit 把 ref（分支名/tag/sha）解析为完整 commit SHA；传入已是 40 位 sha 时原样返回。
	ResolveCommit(ctx context.Context, repoFullName, ref string) (sha string, err error)
	// EnsureWebhook 确保仓库存在一个指向 targetURL 的 push webhook。
	// 重建前会删除所有指向 targetURL 的旧 hook（含历史重复项），再用最新配置创建：
	// created=false 表示本次为更新重建，created=true 表示原先没有、本次新建。
	// secret 为平台签名密钥（各 provider 放到对应字段），空串表示不签名。
	// branchFilter 非空时在支持的平台（Gitea/GitLab）设置为只触发该分支的 push 事件；
	// GitHub/Gitee 的 webhook API 不支持分支过滤，忽略此参数，由服务端按默认分支兜底过滤。
	EnsureWebhook(ctx context.Context, repoFullName, targetURL, secret, branchFilter string) (created bool, hookID string, err error)
}

// hookErr 把平台返回的 403/404 转成更友好的权限提示，其余原样返回。
func hookErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, " 403") || strings.Contains(msg, " 404") {
		return fmt.Errorf("Token 缺少 webhook 管理权限（需仓库管理员级别）: %s", msg)
	}
	return err
}

// sameHookURL 比较两个 webhook 回调地址是否等价：只比 scheme+host+path，
// 忽略 query、末尾斜杠与大小写差异（平台回显时可能带/不带尾部斜杠）。
func sameHookURL(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	if err1 != nil || err2 != nil || ua.Host == "" || ub.Host == "" {
		return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) &&
		strings.EqualFold(ua.Host, ub.Host) &&
		strings.TrimRight(ua.Path, "/") == strings.TrimRight(ub.Path, "/")
}

// New 按 provider 构造客户端。baseURL 为自建实例 API 根地址，空串用各平台默认值。
func New(provider, baseURL, token string) (Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("token 不能为空")
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		return newGitHubClient(baseURL, token), nil
	case "gitlab":
		return newGitLabClient(baseURL, token), nil
	case "gitee":
		return newGiteeClient(baseURL, token), nil
	case "gitea":
		return newGiteaClient(baseURL, token), nil
	default:
		return nil, fmt.Errorf("不支持的平台: %s", provider)
	}
}
