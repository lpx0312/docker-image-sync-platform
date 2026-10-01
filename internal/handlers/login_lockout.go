package handlers

import (
	"fmt"
	"sync"
	"time"
)

// 登录防爆破参数：同一用户名连续失败达到阈值后锁定一段时间
const (
	loginMaxFailures = 5                // 连续失败次数阈值
	loginLockWindow  = 15 * time.Minute // 锁定时长
)

type lockoutEntry struct {
	failures    int
	lockedUntil time.Time
}

// loginLockout 内存级登录失败锁定（单实例部署足够；重启即清零）
type loginLockout struct {
	mu      sync.Mutex
	entries map[string]*lockoutEntry
}

func newLoginLockout() *loginLockout {
	return &loginLockout{entries: make(map[string]*lockoutEntry)}
}

// locked 返回该用户名当前是否处于锁定状态及剩余时长
func (l *loginLockout) locked(username string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[username]
	if !ok {
		return false, 0
	}
	if remaining := time.Until(e.lockedUntil); remaining > 0 {
		return true, remaining
	}
	return false, 0
}

// recordFailure 记录一次失败；达到阈值时锁定并返回锁定的剩余时长
func (l *loginLockout) recordFailure(username string) (locked bool, remaining time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[username]
	if e == nil {
		e = &lockoutEntry{}
		l.entries[username] = e
	}
	// 上一次锁定已过期则重新计数
	if !e.lockedUntil.IsZero() && time.Now().After(e.lockedUntil) {
		e.failures = 0
		e.lockedUntil = time.Time{}
	}
	e.failures++
	if e.failures >= loginMaxFailures {
		e.lockedUntil = time.Now().Add(loginLockWindow)
		return true, loginLockWindow
	}
	return false, 0
}

// recordSuccess 登录成功后清除计数
func (l *loginLockout) recordSuccess(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, username)
}

// formatLockMinutes 把剩余锁定时长格式化为对用户友好的分钟数（向上取整，至少 1 分钟）
func formatLockMinutes(d time.Duration) string {
	minutes := int(d / time.Minute)
	if d%time.Minute != 0 {
		minutes++
	}
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("%d 分钟", minutes)
}
