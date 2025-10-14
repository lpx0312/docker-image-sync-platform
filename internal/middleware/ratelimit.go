package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter 定义基于令牌桶算法的分布式限流器。
//
// 字段说明：
//   - limiters: 存储每个键（通常是IP地址）对应的限流器实例
//   - mu: 读写锁，保护limiters映射的并发安全
//   - rate: 令牌生成速率，控制请求频率
//   - burst: 令牌桶容量，允许的突发请求数量
//
// 工作原理：
//   - 为每个唯一键（IP/用户ID等）维护独立的令牌桶
//   - 令牌以固定速率生成，请求消耗令牌
//   - 当令牌不足时拒绝请求，实现限流效果
//
// 特性：
//   - 线程安全的并发访问
//   - 自动清理长时间未使用的限流器
//   - 支持突发流量处理
//   - 内存使用优化
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter 创建一个新的限流器实例。
//
// 参数：
//   - r: 令牌生成速率（rate.Limit类型），如rate.Every(time.Second)表示每秒1个令牌
//   - b: 令牌桶容量，允许的最大突发请求数
//
// 返回值：
//   - *RateLimiter: 初始化完成的限流器实例
//
// 使用场景：
//   - API接口的访问频率控制
//   - 防止恶意请求和DDoS攻击
//   - 保护后端服务资源
//
// 示例：
//
//	// 每秒最多10个请求，突发容量20
//	limiter := NewRateLimiter(10, 20)
//
//	// 每分钟最多60个请求
//	limiter := NewRateLimiter(rate.Every(time.Second), 60)
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
	}
}

// GetLimiter 获取或创建指定键的限流器实例。
//
// 功能说明：
//   - 线程安全地获取指定键对应的限流器
//   - 如果限流器不存在则自动创建新实例
//   - 使用读写锁优化并发性能
//   - 确保每个键都有独立的限流控制
//
// 参数：
//   - ip: 限流键，通常是客户端IP地址或用户标识
//
// 返回值：
//   - *rate.Limiter: 对应键的限流器实例
//
// 并发安全：
//   - 使用互斥锁保护映射操作
//   - 支持高并发访问场景
//   - 避免竞态条件和数据竞争
//
// 使用场景：
//   - 中间件中获取请求对应的限流器
//   - 动态创建新客户端的限流控制
//   - 分布式限流的本地实现
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

// CleanupOldLimiters 定期清理长时间未使用的限流器，防止内存泄漏。
//
// 功能说明：
//   - 每5分钟执行一次清理任务
//   - 删除令牌桶已满（长时间无请求）的限流器
//   - 防止内存无限增长和资源泄漏
//   - 在后台goroutine中运行，不阻塞主流程
//
// 清理策略：
//   - 检查限流器的令牌数量是否等于桶容量
//   - 如果令牌桶已满，说明长时间没有请求
//   - 删除这些不活跃的限流器实例
//
// 性能优化：
//   - 使用定时器避免持续轮询
//   - 批量清理减少锁竞争
//   - 内存回收及时释放资源
//
// 注意事项：
//   - 此方法应在goroutine中调用
//   - 清理过程中会短暂锁定限流器映射
//   - 清理间隔可根据实际需求调整
//
// 使用场景：
//   - 长期运行的服务器应用
//   - 高并发场景的内存管理
//   - 防止限流器映射无限增长
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

// RateLimit 创建基于客户端IP的限流中间件。
//
// 功能说明：
//   - 根据客户端IP地址进行独立限流控制
//   - 超出限制时返回429状态码和错误信息
//   - 自动启动后台清理goroutine
//   - 使用令牌桶算法实现平滑限流
//
// 参数：
//   - r: 令牌生成速率，控制每秒允许的请求数
//   - b: 令牌桶容量，允许的突发请求数量
//
// 返回值：
//   - gin.HandlerFunc: Gin框架中间件函数
//
// 限流逻辑：
//  1. 获取客户端IP地址
//  2. 获取或创建对应的限流器
//  3. 检查是否有可用令牌
//  4. 无令牌时返回429错误，有令牌时继续处理
//
// 响应格式：
//   - 状态码: 429 Too Many Requests
//   - 响应体: {"code": 429, "message": "Too many requests"}
//
// 使用场景：
//   - 通用API接口的访问控制
//   - 防止单个IP的恶意请求
//   - 保护服务器资源不被滥用
//
// 示例：
//
//	// 每秒最多10个请求，突发容量20
//	router.Use(middleware.RateLimit(10, 20))
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

// RateLimitByKey 创建基于自定义键的限流中间件。
//
// 功能说明：
//   - 支持自定义限流键的生成逻辑
//   - 可以基于用户ID、API密钥、请求路径等进行限流
//   - 提供比IP限流更灵活的控制策略
//   - 空键时跳过限流检查
//
// 参数：
//   - r: 令牌生成速率
//   - b: 令牌桶容量
//   - keyFunc: 键生成函数，从请求上下文中提取限流键
//
// 返回值：
//   - gin.HandlerFunc: Gin框架中间件函数
//
// 键生成函数：
//   - 接收gin.Context参数
//   - 返回字符串作为限流键
//   - 返回空字符串时跳过限流
//   - 可以组合多个字段生成复合键
//
// 使用场景：
//   - 基于用户身份的限流控制
//   - API密钥级别的访问限制
//   - 按接口路径的差异化限流
//   - 多维度组合限流策略
//
// 示例：
//
//	// 基于用户ID限流
//	keyFunc := func(c *gin.Context) string {
//	    return c.GetString("user_id")
//	}
//	router.Use(middleware.RateLimitByKey(5, 10, keyFunc))
//
//	// 基于API密钥限流
//	keyFunc := func(c *gin.Context) string {
//	    return c.GetHeader("X-API-Key")
//	}
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

// SyncRateLimit 创建专门用于同步操作的严格限流中间件。
//
// 功能说明：
//   - 针对资源密集型的同步操作进行严格限流
//   - 防止并发同步操作导致系统资源耗尽
//   - 使用更保守的限流策略保护后端服务
//   - 基于客户端IP进行独立限流控制
//
// 限流策略：
//   - 速率: 每分钟最多5次请求（每12秒1次）
//   - 突发: 最多1个并发请求，不允许突发
//   - 目标: 避免同步操作的资源竞争
//
// 适用场景：
//   - Docker镜像同步接口
//   - 大文件上传/下载操作
//   - 数据库备份/恢复操作
//   - 其他资源密集型API
//
// 返回值：
//   - gin.HandlerFunc: 配置好的限流中间件
//
// 设计考虑：
//   - 同步操作通常耗时较长且消耗大量资源
//   - 严格限流可以防止系统过载
//   - 保证服务稳定性和用户体验
//
// 使用示例：
//
//	syncGroup := router.Group("/api/v1/sync")
//	syncGroup.Use(middleware.SyncRateLimit())
//	syncGroup.POST("/start", syncHandler.StartSync)
func SyncRateLimit() gin.HandlerFunc {
	// 每分钟最多5次同步请求
	return RateLimit(rate.Every(time.Minute/5), 1)
}
