package services

import (
	"os"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
)

// TestCreateOrUpdateSecretE2E 真实调用 GitHub secrets API 验证 CreateOrUpdateSecret
// （仅当环境变量 GH_E2E_TOKEN / GH_E2E_REPO 提供时运行，验证后清理）
func TestCreateOrUpdateSecretE2E(t *testing.T) {
	token := os.Getenv("GH_E2E_TOKEN")
	target := os.Getenv("GH_E2E_REPO") // 格式 owner/repo
	if token == "" || target == "" {
		t.Skip("需要 GH_E2E_TOKEN 和 GH_E2E_REPO 才运行")
	}

	owner, repo, ok := strings.Cut(target, "/")
	if !ok {
		t.Fatalf("GH_E2E_REPO 格式应为 owner/repo, got %q", target)
	}

	svc := &GitHubService{
		client:  resty.New().SetAuthToken(token),
		baseURL: "https://api.github.com",
		owner:   owner,
		repo:    repo,
	}

	const name = "Z_E2E_PROBE_SECRET"
	const value = "probe-value-可回收"
	if err := svc.CreateOrUpdateSecret(name, value); err != nil {
		t.Fatalf("CreateOrUpdateSecret 失败: %v", err)
	}

	// 列表确认存在
	listResp, err := svc.client.R().
		SetHeader("Accept", "application/vnd.github.v3+json").
		Get(svc.baseURL + "/repos/" + owner + "/" + repo + "/actions/secrets?per_page=100")
	if err != nil || listResp.StatusCode() != 200 {
		t.Fatalf("列出 secrets 失败: status=%d err=%v", listResp.StatusCode(), err)
	}
	if !strings.Contains(listResp.String(), name) {
		t.Errorf("secret %s 未出现在列表中", name)
	}

	// 清理
	delResp, err := svc.client.R().
		Delete(svc.baseURL + "/repos/" + owner + "/" + repo + "/actions/secrets/" + name)
	if err != nil || delResp.StatusCode() != 204 {
		t.Errorf("清理测试 secret 失败: status=%d err=%v", delResp.StatusCode(), err)
	}
}
