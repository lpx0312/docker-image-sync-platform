package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter 限流器结构
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter 创建新的限流器
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
	}
}

// GetLimiter 获取指定IP的限流器
func (rl *RateLimiter) GetLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[ip] = limiter
	}

	return limiter
}

// CleanupOldLimiters 清理旧的限流器
func (rl *RateLimiter) CleanupOldLimiters() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for ip, limiter := range rl.limiters {
			// 如果限流器5分钟内没有请求，则删除
			if limiter.Tokens() == float64(rl.burst) {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit 限流中间件
func RateLimit(r rate.Limit, b int) gin.HandlerFunc {
	limiter := NewRateLimiter(r, b)
	
	// 启动清理协程
	go limiter.CleanupOldLimiters()

	return func(c *gin.Context) {
		ip := c.ClientIP()
		l := limiter.GetLimiter(ip)

		if !l.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    http.StatusTooManyRequests,
				"message": "Too many requests",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RateLimitByKey 根据自定义键限流
func RateLimitByKey(r rate.Limit, b int, keyFunc func(*gin.Context) string) gin.HandlerFunc {
	limiter := NewRateLimiter(r, b)
	
	// 启动清理协程
	go limiter.CleanupOldLimiters()

	return func(c *gin.Context) {
		key := keyFunc(c)
		if key == "" {
			c.Next()
			return
		}

		l := limiter.GetLimiter(key)

		if !l.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    http.StatusTooManyRequests,
				"message": "Too many requests",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// SyncRateLimit 同步操作专用限流（更严格）
func SyncRateLimit() gin.HandlerFunc {
	// 每分钟最多5次同步请求
	return RateLimit(rate.Every(time.Minute/5), 1)
}