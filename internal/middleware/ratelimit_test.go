package middleware

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ---------- RateLimiter 结构 ----------

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(10), 20)
	if rl == nil {
		t.Fatal("NewRateLimiter 返回 nil")
	}
	eq(t, rate.Limit(10), rl.rate)
	eq(t, 20, rl.burst)
	isTrue(t, rl.limiters != nil, "limiters map 应已初始化")
}

func TestGetLimiter_SameKeyReturnsSameInstance(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(1), 5)

	l1 := rl.GetLimiter("1.2.3.4")
	l2 := rl.GetLimiter("1.2.3.4")
	isTrue(t, l1 == l2, "同一 key 应返回同一 limiter 实例")

	l3 := rl.GetLimiter("5.6.7.8")
	isTrue(t, l1 != l3, "不同 key 应返回不同 limiter 实例")

	// 空字符串也是合法 key
	l4 := rl.GetLimiter("")
	l5 := rl.GetLimiter("")
	isTrue(t, l4 == l5, "空 key 也应有独立 limiter")
}

func TestGetLimiter_Concurrent(t *testing.T) {
	rl := NewRateLimiter(rate.Every(time.Hour), 100)

	// 并发获取同一 key，配合 -race 验证互斥锁保护
	var wg sync.WaitGroup
	results := make([]*rate.Limiter, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = rl.GetLimiter("same-key")
		}(i)
	}
	wg.Wait()

	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Fatalf("并发获取应得到同一实例: results[%d] != results[0]", i)
		}
	}
}

// ---------- RateLimit 中间件（e2e） ----------

func TestRateLimit_E2E(t *testing.T) {
	// 速率极慢（每小时 1 个令牌）保证测试窗口内不补充令牌，burst=2
	engine := newTestEngine()
	engine.Use(RateLimit(rate.Every(time.Hour), 2))
	engine.GET("/r", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// 前两个请求（突发容量）通过
	for i := 0; i < 2; i++ {
		rec := performRequest(engine, http.MethodGet, "/r", nil, nil)
		ensureStatus(t, rec, http.StatusOK)
	}

	// 第三个请求触发限流
	rec := performRequest(engine, http.MethodGet, "/r", nil, nil)
	ensureStatus(t, rec, http.StatusTooManyRequests)
	body := decodeJSON(t, rec)
	eq(t, float64(http.StatusTooManyRequests), body["code"])
	eq(t, "Too many requests", body["message"])

	// 后续请求持续被拒
	rec = performRequest(engine, http.MethodGet, "/r", nil, nil)
	ensureStatus(t, rec, http.StatusTooManyRequests)
}

func TestRateLimit_DifferentIPsIndependent(t *testing.T) {
	engine := newTestEngine()
	engine.Use(RateLimit(rate.Every(time.Hour), 1))
	engine.GET("/r", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ip": c.ClientIP()}) })

	// IP A 耗尽配额
	recA1 := performRequest(engine, http.MethodGet, "/r", nil, nil) // RemoteAddr 固定 192.0.2.10
	ensureStatus(t, recA1, http.StatusOK)
	recA2 := performRequest(engine, http.MethodGet, "/r", nil, nil)
	ensureStatus(t, recA2, http.StatusTooManyRequests)

	// IP B 仍有独立配额
	req := newRequestNilBody(http.MethodGet, "/r")
	req.RemoteAddr = "198.51.100.7:999"
	rec := httptestRecorder()
	engine.ServeHTTP(rec, req)
	ensureStatus(t, rec, http.StatusOK)
}

// ---------- RateLimitByKey 中间件（e2e） ----------

func TestRateLimitByKey_E2E(t *testing.T) {
	engine := newTestEngine()
	engine.Use(RateLimitByKey(rate.Every(time.Hour), 1, func(c *gin.Context) string {
		return c.GetHeader("X-API-Key")
	}))
	engine.GET("/r", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// key=alpha：首个通过，第二个被拒
	h := map[string]string{"X-API-Key": "alpha"}
	rec := performRequest(engine, http.MethodGet, "/r", nil, h)
	ensureStatus(t, rec, http.StatusOK)
	rec = performRequest(engine, http.MethodGet, "/r", nil, h)
	ensureStatus(t, rec, http.StatusTooManyRequests)

	// key=beta：独立配额
	h = map[string]string{"X-API-Key": "beta"}
	rec = performRequest(engine, http.MethodGet, "/r", nil, h)
	ensureStatus(t, rec, http.StatusOK)

	// 无 key：跳过限流，全部放行
	for i := 0; i < 5; i++ {
		rec = performRequest(engine, http.MethodGet, "/r", nil, nil)
		ensureStatus(t, rec, http.StatusOK)
	}
}

// ---------- SyncRateLimit（e2e） ----------

func TestSyncRateLimit_E2E(t *testing.T) {
	engine := newTestEngine()
	engine.Use(SyncRateLimit())
	engine.POST("/sync/start", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// 突发容量 1：首个请求通过
	rec := performRequest(engine, http.MethodPost, "/sync/start", nil, nil)
	ensureStatus(t, rec, http.StatusOK)

	// 立即重试被限流（速率每 12 秒 1 个令牌）
	rec = performRequest(engine, http.MethodPost, "/sync/start", nil, nil)
	ensureStatus(t, rec, http.StatusTooManyRequests)
	eq(t, "Too many requests", decodeJSON(t, rec)["message"])
}

// ---------- 限流器与真实令牌桶语义 ----------

func TestRateLimiter_TokenBucketBehavior(t *testing.T) {
	rl := NewRateLimiter(rate.Every(10*time.Millisecond), 3)
	l := rl.GetLimiter("bucket")

	// 突发 3 个全部允许
	allowed := 0
	for i := 0; i < 3; i++ {
		if l.Allow() {
			allowed++
		}
	}
	eq(t, 3, allowed)

	// 第 4 个被拒
	isFalse(t, l.Allow(), "令牌耗尽后应拒绝")

	// 等待令牌按速率补充
	time.Sleep(30 * time.Millisecond)
	isTrue(t, l.Allow(), "等待后应重新获得令牌")
}
