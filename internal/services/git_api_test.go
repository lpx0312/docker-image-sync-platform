package services

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"docker-image-sync-platform/internal/logger"

	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	if logger.Logger == nil {
		logger.Logger = zap.NewNop()
	}
	m.Run()
}

// TestGitAPIRetryRebuildsBody 验证重试时通过 GetBody 重建请求体：
// 修复前 PUT 重试会复用已消费的 req.Body，发出空 body 导致必失败
func TestGitAPIRetryRebuildsBody(t *testing.T) {
	const payload = `{"content":"dGVzdCBjb250ZW50","message":"sync images"}`

	var receivedBodies []string
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	newReq := func() *http.Request {
		req, err := http.NewRequest(http.MethodPut, ts.URL+"/repos/owner/repo/contents/images.txt", bytes.NewBufferString(payload))
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		return req
	}

	services := map[string]interface {
		executeRequestWithRetry(req *http.Request) (*http.Response, error)
	}{
		"github": &GitHubAPIService{client: ts.Client(), retries: 3, retryWait: time.Millisecond},
		"gitee":  &GiteeAPIService{client: ts.Client(), retries: 3, retryWait: time.Millisecond},
	}

	for name, svc := range services {
		t.Run(name, func(t *testing.T) {
			hits = 0
			receivedBodies = nil

			resp, err := svc.executeRequestWithRetry(newReq())
			if err != nil {
				t.Fatalf("重试后仍失败: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("最终状态码 = %d, want 200", resp.StatusCode)
			}
			if hits != 2 {
				t.Errorf("服务端收到请求数 = %d, want 2", hits)
			}
			if receivedBodies[1] != payload {
				t.Errorf("重试请求的 body = %q, want %q（重试时未重建请求体）", receivedBodies[1], payload)
			}
		})
	}
}
