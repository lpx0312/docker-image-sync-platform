package middleware

import (
	"sync"
	"time"
)

// tokenBlacklist 已登出 token 的内存黑名单。
// 单实例方案：服务重启后清空，被吊销 token 最长存活到自身自然过期。
var tokenBlacklist = struct {
	sync.RWMutex
	entries map[string]time.Time // token -> 自然过期时间
}{entries: make(map[string]time.Time)}

// RevokeToken 吊销 token 直到其自然过期。
// 仅在验签通过后调用（Logout），保证入库的都是合法签发的 token。
func RevokeToken(token string, expiresAt time.Time) {
	if token == "" || expiresAt.IsZero() {
		return
	}
	tokenBlacklist.Lock()
	defer tokenBlacklist.Unlock()

	// 顺带清理已自然过期的条目，防止长期运行下无界增长
	now := time.Now()
	for t, exp := range tokenBlacklist.entries {
		if !exp.After(now) {
			delete(tokenBlacklist.entries, t)
		}
	}
	tokenBlacklist.entries[token] = expiresAt
}

// isTokenRevoked 检查 token 是否被吊销且尚未自然过期
func isTokenRevoked(token string) bool {
	tokenBlacklist.RLock()
	defer tokenBlacklist.RUnlock()
	exp, ok := tokenBlacklist.entries[token]
	return ok && exp.After(time.Now())
}
