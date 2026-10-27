package main

// client.go 的单元与边界测试：认证注入、401 自动重登、错误翻译、429 限流重试。
// 所有服务端点均指向 httptest，不访问真实网络。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAPIErrorMessage(t *testing.T) {
	e := &APIError{StatusCode: 500, Message: "boom"}
	if e.Error() != "boom" {
		t.Errorf("有 message 时应原样返回: %q", e.Error())
	}
	e2 := &APIError{StatusCode: 502}
	if e2.Error() != "HTTP 502" {
		t.Errorf("无 message 时应输出状态码: %q", e2.Error())
	}
}

func newTestClient(server string) *Client {
	return NewClient(&Config{Server: server, Token: "tok", Username: "u", Password: "p"})
}

func TestClientDoSuccess(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		respondJSON(w, `{"status":"success","data":{"x":1}}`)
	}))
	defer srv.Close()

	var result struct {
		Status string `json:"status"`
	}
	if err := newTestClient(srv.URL).do("GET", "/x", nil, &result); err != nil {
		t.Fatalf("do: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("响应解析 = %+v", result)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, 期望 Bearer tok", gotAuth)
	}
}

func TestClientDoNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // 立即关闭，制造连接失败

	err := newTestClient(url).do("GET", "/x", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "无法连接服务器") {
		t.Errorf("期望连接错误, got %v", err)
	}
}

func TestClientDo401NoPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(&Config{Server: srv.URL, Token: "expired"})
	err := c.do("GET", "/x", nil, nil)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("期望 *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("StatusCode = %d", apiErr.StatusCode)
	}
	// 本地无密码 → 不会自动重登，直接提示重新登录
	if apiErr.Error() != "登录已过期，请重新执行 dsync login" {
		t.Errorf("Error() = %q", apiErr.Error())
	}
}

func TestClientDoOnce401WithoutMessage(t *testing.T) {
	// 有本地密码且未重登过时，doOnce 返回裸 401（无 message），
	// 由上层 do() 触发重登。该分支只在 doOnce 直调时可见。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(&Config{Server: srv.URL, Token: "expired", Username: "u", Password: "p"})
	err := c.doOnce("GET", "/x", nil, nil, false)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != 401 {
		t.Fatalf("期望裸 401 APIError, got %v", err)
	}
	if apiErr.Message != "" || apiErr.Error() != "HTTP 401" {
		t.Errorf("该分支不应携带 message: %+v", apiErr)
	}
}

func TestClientDo401AutoRelogin(t *testing.T) {
	isolateGlobals(t)
	configDirOverride = t.TempDir()

	// /x 仅接受新 token；旧 token 401 后应自动用存量密码重登并重试
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      "fresh-token",
				"user":       map[string]any{"username": "u"},
				"expires_at": time.Now().Add(time.Hour),
			})
		default:
			if r.Header.Get("Authorization") == "Bearer fresh-token" {
				respondJSON(w, `{"ok":true}`)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()

	c := NewClient(&Config{Server: srv.URL, Token: "expired", Username: "u", Password: "p"})
	if err := c.do("GET", "/x", nil, nil); err != nil {
		t.Fatalf("自动重登后应成功: %v", err)
	}
	if c.cfg.Token != "fresh-token" {
		t.Errorf("重登后 token = %q, 期望 fresh-token", c.cfg.Token)
	}
	// Relogin 会落盘配置，验证 token 持久化
	data, err := os.ReadFile(filepath.Join(configDirOverride, "config.json"))
	if err != nil || !strings.Contains(string(data), "fresh-token") {
		t.Errorf("重登后配置应包含新 token: %v", err)
	}
}

func TestClientDo401ReloginFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			w.WriteHeader(http.StatusInternalServerError)
			respondJSON(w, `{"error":"内部错误"}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(&Config{Server: srv.URL, Token: "expired", Username: "u", Password: "p"})
	err := c.do("GET", "/x", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "自动续期登录失败") {
		t.Fatalf("期望重登失败错误, got %v", err)
	}
}

func TestClientDo401StillUnauthorized(t *testing.T) {
	// 重登成功但目标接口仍 401（如 token 被服务端吊销）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token": "t2", "user": map[string]any{}, "expires_at": time.Now().Add(time.Hour),
			})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(&Config{Server: srv.URL, Token: "expired", Username: "u", Password: "p"})
	err := c.do("GET", "/x", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "登录已过期，请重新执行 dsync login") {
		t.Fatalf("期望登录过期提示, got %v", err)
	}
}

func TestClientDo403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		respondJSON(w, `{"error":"无此权限"}`)
	}))
	defer srv.Close()

	err := newTestClient(srv.URL).do("GET", "/x", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "缺少所需权限") || !strings.Contains(err.Error(), "无此权限") {
		t.Fatalf("期望权限错误, got %v", err)
	}
}

func TestClientDoErrorBodyFormats(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"error 字段", `{"error":"e1"}`, "e1"},
		{"message 字段", `{"code":"X","message":"m1"}`, "m1"},
		{"status+message", `{"status":"error","message":"m2"}`, "m2"},
		{"畸形 JSON", "boom-not-json", "boom-not-json"},
		{"空 body", "", "未知错误"},
		{"message 非字符串", `{"message":123}`, "HTTP 错误"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			err := newTestClient(srv.URL).do("GET", "/x", nil, nil)
			if err == nil {
				t.Fatal("期望报错")
			}
			if c.name == "message 非字符串" {
				// 无可用字段时回退为整段 body 的描述
				if !strings.Contains(err.Error(), "HTTP 错误") {
					t.Errorf("got %v", err)
				}
				return
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("错误 %q 未包含 %q", err.Error(), c.want)
			}
		})
	}
}

func TestClientDo2xxMalformedBody(t *testing.T) {
	// 2xx 但响应体不是 JSON：resty 反序列化失败应报错
	// （注意必须带 JSON Content-Type，否则 resty 会跳过解析）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("this is not json"))
	}))
	defer srv.Close()

	var result map[string]any
	err := newTestClient(srv.URL).do("GET", "/x", nil, &result)
	if err == nil {
		t.Fatal("畸形 2xx 响应应报错")
	}
}

func TestClientLogin(t *testing.T) {
	isolateGlobals(t)
	configDirOverride = t.TempDir()

	t.Run("成功并回写 token", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["username"] != "u" || body["password"] != "p" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token": "login-ok", "user": map[string]any{"role_name": "管理员"},
				"expires_at": time.Now().Add(time.Hour),
			})
		}))
		defer srv.Close()

		c := NewClient(&Config{Server: srv.URL})
		resp, err := c.Login("u", "p")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Token != "login-ok" {
			t.Errorf("token = %q", resp.Token)
		}
		if c.cfg.Token != "login-ok" || c.cfg.TokenExpiresAt == "" {
			t.Errorf("cfg 未回写: %+v", c.cfg)
		}
	})

	t.Run("密码错误返回 APIError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			respondJSON(w, `{"error":"用户名或密码错误"}`)
		}))
		defer srv.Close()
		_, err := NewClient(&Config{Server: srv.URL}).Login("u", "bad")
		apiErr, ok := err.(*APIError)
		if !ok || apiErr.StatusCode != 401 {
			t.Fatalf("期望 401 APIError, got %v", err)
		}
	})

	t.Run("网络错误", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()
		_, err := NewClient(&Config{Server: url}).Login("u", "p")
		if err == nil || !strings.Contains(err.Error(), "无法连接服务器") {
			t.Fatalf("期望连接错误, got %v", err)
		}
	})
}

func TestClientRelogin(t *testing.T) {
	isolateGlobals(t)
	configDirOverride = t.TempDir()

	loginOK := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": "relogin-token", "user": map[string]any{}, "expires_at": time.Now().Add(time.Hour),
		})
	}

	t.Run("缺少本地凭据", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(loginOK))
		defer srv.Close()
		c := NewClient(&Config{Server: srv.URL})
		err := c.Relogin()
		if err == nil {
			t.Fatal("无用户名密码时应报错")
		}
	})

	t.Run("成功", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(loginOK))
		defer srv.Close()
		c := NewClient(&Config{Server: srv.URL, Username: "u", Password: "p"})
		if err := c.Relogin(); err != nil {
			t.Fatal(err)
		}
		if c.cfg.Token != "relogin-token" {
			t.Errorf("token = %q", c.cfg.Token)
		}
	})

	t.Run("登录失败", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		c := NewClient(&Config{Server: srv.URL, Username: "u", Password: "bad"})
		err := c.Relogin()
		if err == nil || !strings.Contains(err.Error(), "自动续期登录失败") {
			t.Fatalf("期望重登失败, got %v", err)
		}
	})
}

func TestPostSyncLimitedNon429ErrorImmediate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		respondJSON(w, `{"error":"服务器错误"}`)
	}))
	defer srv.Close()

	start := time.Now()
	err := newTestClient(srv.URL).postSyncLimited("/sync/submit", map[string]any{}, nil)
	if err == nil || !strings.Contains(err.Error(), "服务器错误") {
		t.Fatalf("期望立即返回错误, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("非 429 错误不应重试, 耗时 %v", elapsed)
	}
}

func TestPostSyncLimited429RetryThenSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 13s 限流重试等待")
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			respondJSON(w, `{"error":"提交过于频繁"}`)
			return
		}
		respondJSON(w, `{"task_id":"t1"}`)
	}))
	defer srv.Close()

	err := newTestClient(srv.URL).postSyncLimited("/sync/submit", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if calls != 2 {
		t.Errorf("请求次数 = %d, 期望 2", calls)
	}
}

func TestPostSyncLimited429AlwaysLimited(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 39s 限流重试等待")
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
		respondJSON(w, `{"error":"提交过于频繁"}`)
	}))
	defer srv.Close()

	err := newTestClient(srv.URL).postSyncLimited("/sync/submit", map[string]any{}, nil)
	if err == nil || !strings.Contains(err.Error(), "提交仍被限流") {
		t.Fatalf("期望限流放弃错误, got %v", err)
	}
	if calls != 4 {
		t.Errorf("请求次数 = %d, 期望 4（重试上限）", calls)
	}
}

func TestExtractErrorMessage(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"error":"优先"}`, "优先"},
		{`{"message":"m"}`, "m"},
		{`{"error":"","message":"fallback"}`, "fallback"}, // 空 error 让位给 message
		{`{"other":1}`, "HTTP 错误"},
		{`plain text`, "plain text"},
		{``, "未知错误"},
	}
	for _, c := range cases {
		if got := extractErrorMessage([]byte(c.body)); !strings.Contains(got, c.want) {
			t.Errorf("extractErrorMessage(%q) = %q, 期望包含 %q", c.body, got, c.want)
		}
	}
}
