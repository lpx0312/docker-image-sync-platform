package middleware

import (
	"sync"
	"testing"
	"time"
)

func TestRevokeToken_IgnoresInvalidInput(t *testing.T) {
	resetTokenBlacklist(t)

	// 空 token / 零值过期时间均不入库
	RevokeToken("", time.Now().Add(time.Hour))
	RevokeToken("some-token", time.Time{})

	tokenBlacklist.RLock()
	n := len(tokenBlacklist.entries)
	tokenBlacklist.RUnlock()
	isTrue(t, n == 0, "无效输入不应写入黑名单")
}

func TestRevokeToken_AndIsTokenRevoked(t *testing.T) {
	resetTokenBlacklist(t)

	// 未登记的 token
	isFalse(t, isTokenRevoked("unknown-token"), "未登记 token 不应被吊销")
	// 空字符串
	isFalse(t, isTokenRevoked(""), "空 token 不应被吊销")

	// 登记 future 过期 -> 吊销生效
	RevokeToken("tok-1", time.Now().Add(time.Hour))
	isTrue(t, isTokenRevoked("tok-1"), "tok-1 应被吊销")

	// 同一 token 重复登记（覆盖过期时间）
	RevokeToken("tok-1", time.Now().Add(2*time.Hour))
	isTrue(t, isTokenRevoked("tok-1"), "tok-1 覆盖后仍应被吊销")

	// 其他 token 不受影响
	isFalse(t, isTokenRevoked("tok-2"), "tok-2 不应被吊销")
}

func TestIsTokenRevoked_NaturallyExpired(t *testing.T) {
	resetTokenBlacklist(t)

	// 已自然过期的登记条目：不视为吊销
	RevokeToken("expired-tok", time.Now().Add(-time.Minute))
	isFalse(t, isTokenRevoked("expired-tok"), "已自然过期的 token 不应视为吊销")

	// 恰好等于当前时刻（exp.After(now) 为 false）
	RevokeToken("now-tok", time.Now())
	isFalse(t, isTokenRevoked("now-tok"), "过期时刻等于现在的 token 不应视为吊销")
}

func TestRevokeToken_CleansExpiredEntries(t *testing.T) {
	resetTokenBlacklist(t)

	// 写入若干已过期条目
	for i := 0; i < 5; i++ {
		RevokeToken("stale-"+time.Duration(i).String(), time.Now().Add(-time.Duration(i+1)*time.Minute))
	}
	// 再写入一条有效条目，触发顺带清理
	RevokeToken("live-tok", time.Now().Add(time.Hour))

	tokenBlacklist.RLock()
	n := len(tokenBlacklist.entries)
	_, liveExists := tokenBlacklist.entries["live-tok"]
	tokenBlacklist.RUnlock()

	isTrue(t, n == 1, "已过期条目应被顺带清理，仅剩有效条目, got %d", n)
	isTrue(t, liveExists, "有效条目不应被清理")

	// 注：过期条目在写入时仍会先入库（RevokeToken 不拒绝过期时间），
	// 由下一次 RevokeToken 或查询语义（exp.After(now)）兜底，
	// 因此这里清理前的瞬时状态可能大于 1，清理动作发生在下一次写入。
}

func TestRevokeToken_ExpiredEntryStillWritten(t *testing.T) {
	resetTokenBlacklist(t)

	// 按当前实现：RevokeToken 不校验 expiresAt 是否已过去，直接写入
	RevokeToken("past-tok", time.Now().Add(-time.Hour))
	tokenBlacklist.RLock()
	_, exists := tokenBlacklist.entries["past-tok"]
	tokenBlacklist.RUnlock()
	isTrue(t, exists, "当前实现会写入已过期的条目（由清理逻辑兜底）")
}

func TestTokenBlacklist_ConcurrentAccess(t *testing.T) {
	resetTokenBlacklist(t)

	// 并发写入 + 读取，配合 -race 验证锁保护
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				RevokeToken("tok", time.Now().Add(time.Duration(n+1)*time.Minute))
			}
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = isTokenRevoked("tok")
			}
		}()
	}
	wg.Wait()

	// 并发结束后应处于一致状态：tok 已吊销
	isTrue(t, isTokenRevoked("tok"), "并发写入后 tok 应被吊销")
}
