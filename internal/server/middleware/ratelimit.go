package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimit 按客户端 IP 做令牌桶限流。rps 为每秒补充速率，burst 为突发容量。
// 超过限额返回 429。用于保护 webhook 等公开端点。
func RateLimit(rps rate.Limit, burst int) gin.HandlerFunc {
	var (
		mu      sync.Mutex
		clients = make(map[string]*rate.Limiter)
	)
	get := func(key string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := clients[key]
		if !ok {
			l = rate.NewLimiter(rps, burst)
			clients[key] = l
		}
		return l
	}
	return func(c *gin.Context) {
		if !get(c.ClientIP()).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limited"})
			return
		}
		c.Next()
	}
}
