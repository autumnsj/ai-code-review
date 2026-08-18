package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LoginGuard 按客户端 IP 记录连续登录失败次数，超过阈值后锁定一段时间，
// 用于防御密码暴力破解。进程内实现（单实例足够），不依赖外部存储。
type LoginGuard struct {
	maxFailures int
	lockout     time.Duration

	mu    sync.Mutex
	state map[string]loginState
}

type loginState struct {
	failures   int
	lockedUntil time.Time
}

// NewLoginGuard 创建登录防护器。failures 为触发锁定的连续失败次数，
// lockout 为锁定时长（如 5 次失败锁 10 分钟）。
func NewLoginGuard(failures int, lockout time.Duration) *LoginGuard {
	return &LoginGuard{
		maxFailures: failures,
		lockout:     lockout,
		state:       make(map[string]loginState),
	}
}

// Middleware 在登录前检查该 IP 是否处于锁定状态。锁定中直接拒绝并返回剩余秒数。
func (g *LoginGuard) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if wait, locked := g.lockedWait(ip); locked {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":         "尝试次数过多，请稍后再试",
				"retry_after":   int(wait.Seconds()),
			})
			return
		}
		c.Next()
	}
}

// Fail 记录一次登录失败；累计达到阈值则锁定该 IP。
func (g *LoginGuard) Fail(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.state[ip]
	st.failures++
	if st.failures >= g.maxFailures {
		st.lockedUntil = time.Now().Add(g.lockout)
		st.failures = 0 // 锁定期间不再累计；锁定期满重新计数
	}
	g.state[ip] = st
}

// Success 登录成功后清空该 IP 的失败记录（成功即解除锁定计数）。
func (g *LoginGuard) Success(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.state, ip)
}

// lockedWait 返回该 IP 距解锁的剩余时间。锁定期满自动清除。
func (g *LoginGuard) lockedWait(ip string) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.state[ip]
	if !ok || st.lockedUntil.IsZero() {
		return 0, false
	}
	if wait := time.Until(st.lockedUntil); wait > 0 {
		return wait, true
	}
	// 锁定期满，清除记录，允许重新尝试。
	delete(g.state, ip)
	return 0, false
}
