package services

// git_api_extra_test.go：GitHubAPIService / GiteeAPIService / GitAPIService 补全测试。
// apiURL 为非导出字段，同包直接构造并指向 httptest；retryWait 调小保证重试用例快速。

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ghAPISvc 构造指向 httptest 的 GitHubAPIService（重试等待 10ms）
func ghAPISvc(t *testing.T, h http.HandlerFunc) (*GitHubAPIService, *[]string) {
	t.Helper()
	var calls []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		h(w, r)
	}))
	t.Cleanup(ts.Close)
	svc := &GitHubAPIService{
		client: &http.Client{Timeout: 5 * time.Second}, token: "tk", owner: "o", repo: "r",
		branch: "main", apiURL: ts.URL, timeout: 5 * time.Second, retries: 3,
		retryWait: 10 * time.Millisecond,
	}
	return svc, &calls
}

func gitAPIJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func TestGitHubAPIService_ReadImagesFile(t *testing.T) {
	t.Run("成功解码base64内容", func(t *testing.T) {
		content := base64.StdEncoding.EncodeToString([]byte("nginx:1.25\n"))
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusOK, `{"sha":"abc","content":"`+content+`"}`)
		})
		got, err := svc.ReadImagesFile()
		require.NoError(t, err)
		assert.Equal(t, "nginx:1.25\n", got)
	})

	t.Run("404返回空内容", func(t *testing.T) {
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusNotFound, `{}`)
		})
		got, err := svc.ReadImagesFile()
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("非重试状态码立即失败", func(t *testing.T) {
		svc, calls := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusUnauthorized, `{}`)
		})
		_, err := svc.ReadImagesFile()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "401")
		require.Len(t, *calls, 1, "401 不应重试")
	})

	t.Run("500重试后仍失败", func(t *testing.T) {
		svc, calls := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusInternalServerError, `{}`)
		})
		_, err := svc.ReadImagesFile()
		require.Error(t, err)
		assert.Len(t, *calls, 3, "retries=3 次后放弃")
	})

	t.Run("500重试后成功", func(t *testing.T) {
		hits := 0
		content := base64.StdEncoding.EncodeToString([]byte("ok"))
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			hits++
			if hits == 1 {
				gitAPIJSON(w, http.StatusServiceUnavailable, `{}`)
				return
			}
			gitAPIJSON(w, http.StatusOK, `{"sha":"s","content":"`+content+`"}`)
		})
		got, err := svc.ReadImagesFile()
		require.NoError(t, err)
		assert.Equal(t, "ok", got)
	})

	t.Run("畸形JSON", func(t *testing.T) {
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusOK, `{{`)
		})
		_, err := svc.ReadImagesFile()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析")
	})
}

func TestGitHubAPIService_UpdateImagesFile(t *testing.T) {
	t.Run("带SHA更新成功", func(t *testing.T) {
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				gitAPIJSON(w, http.StatusOK, `{"sha":"old-sha"}`)
				return
			}
			gitAPIJSON(w, http.StatusOK, `{"content":{"sha":"new"},"commit":{"sha":"commit-1","url":"https://gh/c1"}}`)
		})
		sha, err := svc.UpdateImagesFile("new-content", "msg")
		require.NoError(t, err)
		assert.Equal(t, "commit-1", sha)
	})

	t.Run("文件不存在时SHA为空仍提交", func(t *testing.T) {
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				gitAPIJSON(w, http.StatusNotFound, `{}`)
				return
			}
			gitAPIJSON(w, http.StatusOK, `{"commit":{"sha":"commit-2"}}`)
		})
		sha, err := svc.UpdateImagesFile("c", "m")
		require.NoError(t, err)
		assert.Equal(t, "commit-2", sha)
	})

	t.Run("SHA请求500失败", func(t *testing.T) {
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusInternalServerError, `{}`)
		})
		_, err := svc.UpdateImagesFile("c", "m")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SHA")
	})

	t.Run("PUT非200失败", func(t *testing.T) {
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				gitAPIJSON(w, http.StatusOK, `{"sha":"s"}`)
				return
			}
			gitAPIJSON(w, http.StatusUnprocessableEntity, `{"message":"bad"}`)
		})
		_, err := svc.UpdateImagesFile("c", "m")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "422")
	})
}

func TestGitHubAPIService_GetLatestCommit(t *testing.T) {
	t.Run("成功", func(t *testing.T) {
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/contents/images.txt") {
				gitAPIJSON(w, http.StatusOK, `{"sha":"file-sha"}`)
				return
			}
			gitAPIJSON(w, http.StatusOK, `{"sha":"cs","commit":{"message":"m","author":{"name":"n","email":"e@x","date":"2026-01-01T00:00:00Z"}},"html_url":"https://gh/c"}`)
		})
		ci, err := svc.GetLatestCommit()
		require.NoError(t, err)
		assert.Equal(t, "cs", ci.SHA)
		assert.Equal(t, "n", ci.Author)
		assert.Equal(t, "m", ci.Message)
	})

	t.Run("文件不存在", func(t *testing.T) {
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusNotFound, `{}`)
		})
		_, err := svc.GetLatestCommit()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "文件不存在")
	})

	t.Run("提交查询非200", func(t *testing.T) {
		svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/contents/images.txt") {
				gitAPIJSON(w, http.StatusOK, `{"sha":"s"}`)
				return
			}
			gitAPIJSON(w, http.StatusForbidden, `{}`)
		})
		_, err := svc.GetLatestCommit()
		require.Error(t, err)
	})
}

func TestGitHubAPIService_TestConnection(t *testing.T) {
	svc, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
		gitAPIJSON(w, http.StatusOK, `{"full_name":"o/r"}`)
	})
	require.NoError(t, svc.TestConnection())

	svcErr, _ := ghAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
		gitAPIJSON(w, http.StatusUnauthorized, `{}`)
	})
	require.Error(t, svcErr.TestConnection())
}

func TestGitHubAPIService_shouldRetry(t *testing.T) {
	g := &GitHubAPIService{}
	for _, code := range []int{500, 502, 503, 504, 429} {
		assert.True(t, g.shouldRetry(code), code)
	}
	for _, code := range []int{200, 401, 403, 404, 422} {
		assert.False(t, g.shouldRetry(code), code)
	}
}

// --------------------------------------------------------------------
// GiteeAPIService
// --------------------------------------------------------------------

func giteeAPISvc(t *testing.T, h http.HandlerFunc) *GiteeAPIService {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return &GiteeAPIService{
		client: &http.Client{Timeout: 5 * time.Second}, token: "tk", owner: "o", repo: "r",
		branch: "main", apiURL: ts.URL, timeout: 5 * time.Second, retries: 3,
		retryWait: 10 * time.Millisecond,
	}
}

func TestGiteeAPIService_All(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("img:1"))

	t.Run("ReadImagesFile_成功与404", func(t *testing.T) {
		svc := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusOK, `{"sha":"s","content":"`+content+`"}`)
		})
		got, err := svc.ReadImagesFile()
		require.NoError(t, err)
		assert.Equal(t, "img:1", got)

		svc404 := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusNotFound, `{}`)
		})
		got, err = svc404.ReadImagesFile()
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("ReadImagesFile_重试耗尽", func(t *testing.T) {
		svc := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusBadGateway, `{}`)
		})
		_, err := svc.ReadImagesFile()
		require.Error(t, err)
	})

	t.Run("UpdateImagesFile_成功", func(t *testing.T) {
		svc := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				gitAPIJSON(w, http.StatusOK, `{"sha":"old"}`)
				return
			}
			gitAPIJSON(w, http.StatusOK, `{"commit":{"sha":"c-sha"}}`)
		})
		sha, err := svc.UpdateImagesFile("x", "m")
		require.NoError(t, err)
		assert.Equal(t, "c-sha", sha)
	})

	t.Run("UpdateImagesFile_SHA失败与PUT失败", func(t *testing.T) {
		svc := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusInternalServerError, `{}`)
		})
		_, err := svc.UpdateImagesFile("x", "m")
		require.Error(t, err)

		svcPut := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				gitAPIJSON(w, http.StatusOK, `{"sha":"s"}`)
				return
			}
			gitAPIJSON(w, http.StatusBadRequest, `{}`)
		})
		_, err = svcPut.UpdateImagesFile("x", "m")
		require.Error(t, err)
	})

	t.Run("GetLatestCommit", func(t *testing.T) {
		svc := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/contents/images.txt") {
				gitAPIJSON(w, http.StatusOK, `{"sha":"fs"}`)
				return
			}
			gitAPIJSON(w, http.StatusOK, `{"sha":"cs","commit":{"message":"m","author":{"name":"n","email":"e","date":"2026-01-01T00:00:00Z"}},"html_url":"u"}`)
		})
		ci, err := svc.GetLatestCommit()
		require.NoError(t, err)
		assert.Equal(t, "cs", ci.SHA)

		svc404 := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusNotFound, `{}`)
		})
		_, err = svc404.GetLatestCommit()
		require.Error(t, err)
	})

	t.Run("TestConnection", func(t *testing.T) {
		svc := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusOK, `{"full_name":"o/r"}`)
		})
		require.NoError(t, svc.TestConnection())

		svcErr := giteeAPISvc(t, func(w http.ResponseWriter, r *http.Request) {
			gitAPIJSON(w, http.StatusUnauthorized, `{}`)
		})
		require.Error(t, svcErr.TestConnection())
	})

	t.Run("shouldRetry", func(t *testing.T) {
		g := &GiteeAPIService{}
		assert.True(t, g.shouldRetry(503))
		assert.False(t, g.shouldRetry(404))
	})
}

// --------------------------------------------------------------------
// GitAPIService（工具方法）
// --------------------------------------------------------------------

func TestGitAPIService_Utils(t *testing.T) {
	svc := NewGitAPIService()

	owner, err := svc.ParseGitHubRepoURL("https://github.com/lpx0312/repo")
	require.NoError(t, err)
	assert.Equal(t, "lpx0312", owner)

	_, err = svc.ParseGitHubRepoURL("https://gitee.com/x/y")
	assert.Error(t, err)

	// ctx 立即取消 → 快速失败（不打真实 GitHub）
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = svc.TestGitHubConnection(ctx, "u", "tk")
	require.Error(t, err)
}
