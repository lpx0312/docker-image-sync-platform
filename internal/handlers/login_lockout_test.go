package handlers

import (
	"testing"
	"time"
)

func TestLoginLockout(t *testing.T) {
	l := newLoginLockout()

	// 未达阈值前不锁定
	for i := 1; i < loginMaxFailures; i++ {
		locked, _ := l.recordFailure("alice")
		if locked {
			t.Fatalf("第 %d 次失败不应锁定", i)
		}
		if locked, _ := l.locked("alice"); locked {
			t.Fatalf("第 %d 次失败后不应处于锁定状态", i)
		}
	}

	// 达到阈值后锁定
	locked, remaining := l.recordFailure("alice")
	if !locked || remaining <= 0 {
		t.Fatalf("第 %d 次失败应触发锁定, got locked=%v remaining=%v", loginMaxFailures, locked, remaining)
	}
	if locked, rem := l.locked("alice"); !locked || rem <= 0 {
		t.Error("锁定期间 locked() 应返回 true")
	}

	// 其他用户名不受影响
	if locked, _ := l.locked("bob"); locked {
		t.Error("其他用户名不应被连带锁定")
	}

	// 成功登录清除计数
	l.recordSuccess("alice")
	if locked, _ := l.locked("alice"); locked {
		t.Error("成功登录后应解除锁定")
	}
	locked, _ = l.recordFailure("alice")
	if locked {
		t.Error("清除后应重新计数, 不应立即锁定")
	}
}

func TestLoginLockoutExpiry(t *testing.T) {
	l := newLoginLockout()
	l.entries["carol"] = &lockoutEntry{
		failures:    loginMaxFailures,
		lockedUntil: time.Now().Add(-time.Minute), // 已过期
	}
	if locked, _ := l.locked("carol"); locked {
		t.Error("过期锁不应生效")
	}
	// 过期后重新计数，而不是立即再锁
	if locked, _ := l.recordFailure("carol"); locked {
		t.Error("锁过期后应重新计数")
	}
}
