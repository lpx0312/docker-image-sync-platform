package services

// github_service_test.go：GitHubService 全方法测试。
// 字段非导出，同包直接构造 + httptest 模拟 GitHub API，完全离线。

import (
	"crypto/rand"
	"time"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ghSvc 用 httptest server 构造 GitHubService
func ghSvc(t *testing.T, h http.HandlerFunc) (*GitHubService, *httptest.Server, *[]string) {
	t.Helper()
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		h(w, r)
	}))
	t.Cleanup(ts.Close)
	svc := &GitHubService{
		client:  resty.New().SetTimeout(5 * time.Second),
		baseURL: ts.URL,
		owner:   "own",
		repo:    "rep",
	}
	return svc, ts, &paths
}

func ghJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

const ghRunJSON = `{"id":42,"status":"completed","conclusion":"success","head_sha":"deadbeef","html_url":"https://gh/runs/42","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:01:00Z","name":"sync"}`

func TestGitHubService_GetWorkflowRun(t *testing.T) {
	t.Run("命中", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			ghJSON(w, http.StatusOK, `{"total_count":1,"workflow_runs":[{"id":42,"head_sha":"deadbeef","html_url":"https://gh/runs/42"}]}`)
		})
		runID, url, err := svc.GetWorkflowRun("deadbeef")
		require.NoError(t, err)
		assert.Equal(t, "42", runID)
		assert.Equal(t, "https://gh/runs/42", url)
	})

	t.Run("HTTP错误立即返回", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			ghJSON(w, http.StatusInternalServerError, `{"message":"boom"}`)
		})
		_, _, err := svc.GetWorkflowRun("x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("畸形JSON", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			ghJSON(w, http.StatusOK, `{bad`)
		})
		_, _, err := svc.GetWorkflowRun("x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解析")
	})

	t.Run("连接失败", func(t *testing.T) {
		svc := &GitHubService{client: resty.New().SetTimeout(time.Second), baseURL: "http://127.0.0.1:1", owner: "o", repo: "r"}
		_, _, err := svc.GetWorkflowRun("x")
		require.Error(t, err)
	})
}

func TestGitHubService_StatusConclusionDetails(t *testing.T) {
	svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
		ghJSON(w, http.StatusOK, ghRunJSON)
	})

	st, err := svc.GetWorkflowRunStatus("42")
	require.NoError(t, err)
	assert.Equal(t, "completed", st)

	concl, err := svc.GetWorkflowRunConclusion("42")
	require.NoError(t, err)
	assert.Equal(t, "success", concl)

	det, err := svc.GetWorkflowRunDetails("42")
	require.NoError(t, err)
	assert.EqualValues(t, 42, det.ID)
	assert.Equal(t, "deadbeef", det.HeadSHA)

	// 错误分支
	svcErr, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
		ghJSON(w, http.StatusNotFound, `{}`)
	})
	_, err = svcErr.GetWorkflowRunStatus("9")
	assert.Error(t, err)
	_, err = svcErr.GetWorkflowRunConclusion("9")
	assert.Error(t, err)
	_, err = svcErr.GetWorkflowRunDetails("9")
	assert.Error(t, err)

	svcBad, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
		ghJSON(w, http.StatusOK, `not-json`)
	})
	_, err = svcBad.GetWorkflowRunStatus("9")
	assert.Error(t, err)
	_, err = svcBad.GetWorkflowRunConclusion("9")
	assert.Error(t, err)
	_, err = svcBad.GetWorkflowRunDetails("9")
	assert.Error(t, err)
}

func TestGitHubService_ListWorkflowRuns(t *testing.T) {
	t.Run("带状态过滤与分页参数", func(t *testing.T) {
		var gotQuery string
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			ghJSON(w, http.StatusOK, `{"total_count":7,"workflow_runs":[{"id":1,"status":"success"}]}`)
		})
		resp, err := svc.ListWorkflowRuns(3, 25, "completed")
		require.NoError(t, err)
		assert.Equal(t, 7, resp.TotalCount)
		require.Len(t, resp.WorkflowRuns, 1)
		assert.Contains(t, gotQuery, "page=3")
		assert.Contains(t, gotQuery, "per_page=25")
		assert.Contains(t, gotQuery, "status=completed")
	})

	t.Run("错误与畸形JSON", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			ghJSON(w, http.StatusInternalServerError, `{}`)
		})
		_, err := svc.ListWorkflowRuns(1, 10, "")
		assert.Error(t, err)

		svc2, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			ghJSON(w, http.StatusOK, `[[`)
		})
		_, err = svc2.ListWorkflowRuns(1, 10, "")
		assert.Error(t, err)
	})
}

func TestGitHubService_CheckRateLimit(t *testing.T) {
	svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
		ghJSON(w, http.StatusOK, `{"rate":{"limit":5000,"remaining":4321}}`)
	})
	rl, err := svc.CheckRateLimit()
	require.NoError(t, err)
	rate := rl["rate"].(map[string]interface{})
	assert.EqualValues(t, 5000, rate["limit"])

	svcErr, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
		ghJSON(w, http.StatusForbidden, `{}`)
	})
	_, err = svcErr.CheckRateLimit()
	assert.Error(t, err)
}

func TestGitHubService_GetActionsUsage(t *testing.T) {
	t.Run("按SKU倍率与仓库聚合", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			// 先被 fetchRepoVisibility 请求 /user/repos，再查 billing
			if strings.Contains(r.URL.Path, "/user/repos") {
				ghJSON(w, http.StatusOK, `[{"name":"rep","visibility":"private"},{"name":"rep2","visibility":"public"}]`)
				return
			}
			if !strings.Contains(r.URL.Path, "/settings/billing/usage") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			ghJSON(w, http.StatusOK, `{"usageItems":[
				{"product":"actions","sku":"Standard runner","quantity":100,"unitType":"Minutes","repositoryName":"rep"},
				{"product":"actions","sku":"macOS runner","quantity":10,"unitType":"Minutes","repositoryName":"rep"},
				{"product":"actions","sku":"Windows runner","quantity":20,"unitType":"Minutes","repositoryName":"rep"},
				{"product":"actions","sku":"Standard runner","quantity":5,"unitType":"Minutes","repositoryName":"rep2"},
				{"product":"codespaces","sku":"x","quantity":999,"unitType":"Minutes","repositoryName":"rep"},
				{"product":"actions","sku":"Standard runner","quantity":1,"unitType":"GB","repositoryName":"rep"}
			]}`)
		})
		// 私有仓库可见性：/user/repos 返回 private
		usage, err := svc.GetActionsUsage()
		require.NoError(t, err)
		assert.Equal(t, 100+10*10+20*2+5, usage.TotalMinutesUsed, "全仓库加权分钟数")
		assert.Equal(t, 100+10+20, usage.MinutesByRepo["rep"], "未加权按仓库")
		assert.Equal(t, 5, usage.MinutesByRepo["rep2"])
		assert.NotEmpty(t, usage.Period)
		assert.Equal(t, "private", usage.RepoVisibility["rep"])
		// 私有/公共分钟数为未加权口径：rep(私有)=130，rep2(公共)=5
		assert.EqualValues(t, 130, usage.PrivateMinutes)
		assert.EqualValues(t, 5, usage.PublicMinutes)
	})

	t.Run("错误分支", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "billing") {
				ghJSON(w, http.StatusForbidden, `{"message":"denied"}`)
				return
			}
			ghJSON(w, http.StatusOK, `[]`)
		})
		_, err := svc.GetActionsUsage()
		assert.Error(t, err)
	})
}

func TestGitHubService_SecretAndTrigger(t *testing.T) {
	// 32 字节随机公钥（base64），满足 sealed box 加密要求
	rawKey := make([]byte, 32)
	_, _ = rand.Read(rawKey)
	pubKey := base64.StdEncoding.EncodeToString(rawKey)

	t.Run("CreateOrUpdateSecret_新建201", func(t *testing.T) {
		var putBody map[string]string
		svc, _, paths := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/public-key"):
				ghJSON(w, http.StatusOK, fmt.Sprintf(`{"key_id":"kid-1","key":%q}`, pubKey))
			case r.Method == http.MethodPut:
				_ = json.NewDecoder(r.Body).Decode(&putBody)
				w.WriteHeader(http.StatusCreated)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		})
		require.NoError(t, svc.CreateOrUpdateSecret("MY_SECRET", "s3cret"))
		assert.Equal(t, "kid-1", putBody["key_id"])
		assert.NotEmpty(t, putBody["encrypted_value"])
		assert.NotEqual(t, "s3cret", putBody["encrypted_value"], "值必须加密传输")
		require.Len(t, *paths, 2)
	})

	t.Run("CreateOrUpdateSecret_更新204", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/public-key") {
				ghJSON(w, http.StatusOK, fmt.Sprintf(`{"key_id":"k","key":%q}`, pubKey))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		require.NoError(t, svc.CreateOrUpdateSecret("S", "v"))
	})

	t.Run("CreateOrUpdateSecret_公钥获取失败", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			ghJSON(w, http.StatusUnauthorized, `{}`)
		})
		err := svc.CreateOrUpdateSecret("S", "v")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "公钥")
	})

	t.Run("CreateOrUpdateSecret_写入失败", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/public-key") {
				ghJSON(w, http.StatusOK, fmt.Sprintf(`{"key_id":"k","key":%q}`, pubKey))
				return
			}
			ghJSON(w, http.StatusUnprocessableEntity, `{"message":"bad name"}`)
		})
		err := svc.CreateOrUpdateSecret("S", "v")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "写入 secret")
	})

	t.Run("TriggerWorkflow_成功返回最新运行", func(t *testing.T) {
		var dispatchBody map[string]interface{}
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/dispatches") {
				_ = json.NewDecoder(r.Body).Decode(&dispatchBody)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if strings.Contains(r.URL.Path, "/actions/runs") {
				ghJSON(w, http.StatusOK, `{"total_count":1,"workflow_runs":[{"id":77,"head_sha":"abc","html_url":"https://gh/77"}]}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		})
		// 内部 sleep 5s 后查最新运行
		runID, runURL, err := svc.TriggerWorkflow("sync.yml", map[string]string{"k": "v"})
		require.NoError(t, err)
		assert.Equal(t, "77", runID)
		assert.Equal(t, "https://gh/77", runURL)
		assert.Equal(t, "main", dispatchBody["ref"])
	})

	t.Run("TriggerWorkflow_非204失败", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			ghJSON(w, http.StatusUnprocessableEntity, `{"message":"ref not found"}`)
		})
		_, _, err := svc.TriggerWorkflow("sync.yml", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workflow_dispatch")
	})
}

func TestGitHubService_GetWorkflowIDByName(t *testing.T) {
	t.Run("按路径匹配", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			ghJSON(w, http.StatusOK, `{"total_count":2,"workflows":[{"id":11,"path":".github/workflows/other.yml"},{"id":22,"path":".github/workflows/sync.yml"}]}`)
		})
		id, err := svc.GetWorkflowIDByName("sync.yml")
		require.NoError(t, err)
		assert.Equal(t, "22", id)
	})

	t.Run("不存在", func(t *testing.T) {
		svc, _, _ := ghSvc(t, func(w http.ResponseWriter, r *http.Request) {
			ghJSON(w, http.StatusOK, `{"total_count":0,"workflows":[]}`)
		})
		_, err := svc.GetWorkflowIDByName("nope.yml")
		require.Error(t, err)
	})
}

func TestGitHubService_helpers(t *testing.T) {
	assert.Equal(t, 3000, planIncludedMinutes("pro"))
	assert.Equal(t, 2000, planIncludedMinutes("free"))
	assert.Equal(t, 0, planIncludedMinutes("team"))

	assert.Equal(t, 10, skuBillingMultiplier("macOS-12"))
	assert.Equal(t, 2, skuBillingMultiplier("Windows 2022"))
	assert.Equal(t, 1, skuBillingMultiplier("Standard"))
}

func TestParseGitHubRepo(t *testing.T) {
	cases := []struct {
		url, owner, repo string
	}{
		{"https://github.com/own/rep.git", "own", "rep"},
		{"https://github.com/own/rep", "own", "rep"},
		{"http://github.com/own/rep", "own", "rep"},
		{"git@github.com:own/rep.git", "own", "rep"},
	}
	for _, c := range cases {
		o, r := parseGitHubRepo(c.url)
		assert.Equal(t, c.owner, o, c.url)
		assert.Equal(t, c.repo, r, c.url)
	}
	// 非法
	o, r := parseGitHubRepo("https://gitee.com/x/y")
	assert.Empty(t, o)
	assert.Empty(t, r)
}
