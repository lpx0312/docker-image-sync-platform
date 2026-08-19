package services

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

// TestSwrSigner_Vector 用与独立参考实现对齐的固定向量校验签名
func TestSwrSigner_Vector(t *testing.T) {
	const (
		ak       = "TESTAK123EXAMPLE"
		sk       = "test-secret-key-for-vector"
		wantSig  = "d4f291cb92cde1933fc8cfe6ed31f2abee9ff5794b84f6421fe94af7dc236c04"
		wantDate = "20260819T000000Z"
	)

	parsed, err := url.Parse("https://swr-api.cn-east-4.myhuaweicloud.com/v2/manage/namespaces/lpx03/repos?limit=100&offset=0")
	if err != nil {
		t.Fatalf("构造 URL 失败: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}

	ts, err := time.Parse(time.RFC3339, "2026-08-19T00:00:00Z")
	if err != nil {
		t.Fatalf("解析固定时间失败: %v", err)
	}

	signer := swrSigner{AccessKey: ak, SecretKey: sk}
	signer.sign(req, ts)

	if got := req.Header.Get("x-sdk-date"); got != wantDate {
		t.Errorf("x-sdk-date = %s, want %s", got, wantDate)
	}
	wantAuth := "SDK-HMAC-SHA256 Access=" + ak +
		", SignedHeaders=host;x-sdk-date, Signature=" + wantSig
	if got := req.Header.Get("Authorization"); got != wantAuth {
		t.Errorf("Authorization = %s\nwant %s", got, wantAuth)
	}
}

// TestSwrAKFromUsername 校验登录用户名（区域@AK）的 AK 提取
func TestSwrAKFromUsername(t *testing.T) {
	cases := map[string]string{
		"cn-east-4@HPUASH3JTSEB73DUQOSS": "HPUASH3JTSEB73DUQOSS",
		"plainuser":                      "plainuser",
		"a@b@c":                          "c",
	}
	for in, want := range cases {
		if got := swrAKFromUsername(in); got != want {
			t.Errorf("swrAKFromUsername(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSwrRegion 校验区域推断
func TestSwrRegion(t *testing.T) {
	cases := []struct {
		registry, username, want string
	}{
		{"swr.cn-east-4.myhuaweicloud.com", "cn-east-4@AK123", "cn-east-4"},
		{"custom.registry.example.com", "cn-north-4@AK123", "cn-north-4"},
		{"custom.registry.example.com", "nouser", ""},
	}
	for _, c := range cases {
		if got := swrRegion(c.registry, c.username); got != c.want {
			t.Errorf("swrRegion(%q, %q) = %q, want %q", c.registry, c.username, got, c.want)
		}
	}
}
