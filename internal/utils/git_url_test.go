package utils

import (
	"strings"
	"testing"
)

func TestParseGitHubRepoURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		// 正常路径
		{"https 标准格式", "https://github.com/owner/repo", "owner", "repo", false},
		{"https 带 .git", "https://github.com/owner/repo.git", "owner", "repo", false},
		{"http 协议", "http://github.com/owner/repo.git", "owner", "repo", false},
		{"SSH 格式", "git@github.com:owner/repo.git", "owner", "repo", false},
		{"简写格式", "owner/repo", "owner", "repo", false},
		{"大写 owner", "https://github.com/Owner/Repo", "Owner", "Repo", false},
		{"unicode owner", "https://github.com/中文/仓库", "中文", "仓库", false},
		{"超长 URL 仍可解析", "https://github.com/" + strings.Repeat("a", 3000) + "/" + strings.Repeat("b", 3000), strings.Repeat("a", 3000), strings.Repeat("b", 3000), false},
		{"特殊字符", "https://github.com/ow!@#/re#$%", "ow!@#", "re#$%", false},

		// 边界与非法输入（按当前实际行为： 尾斜杠会 split 出 3 段导致报错）
		{"空字符串", "", "", "", true},
		{"仅空格", "   ", "", "", true},
		{"多级路径", "https://github.com/a/b/c", "", "", true},
		{"带尾斜杠", "https://github.com/owner/repo/", "", "", true},
		{"只有 owner", "https://github.com/owner", "", "", true},
		{"owner 为空", "https://github.com//repo", "", "", true},
		{"repo 为空", "https://github.com/owner/", "", "", true},
		{"空格分隔", "owner repo", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseGitHubRepoURL(tt.url)
			if tt.wantErr {
				isErr(t, err)
				isEmpty(t, owner)
				isEmpty(t, repo)
				return
			}
			noErr(t, err)
			eq(t, tt.wantOwner, owner)
			eq(t, tt.wantRepo, repo)
		})
	}
}

func TestParseGitHubRepoURL_ActualBehavior(t *testing.T) {
	// 带空格的输入按当前实际行为断言： TrimSpace 后 owner/repo 均非空即可通过
	owner, repo, err := ParseGitHubRepoURL(" owner / repo ")
	noErr(t, err)
	eq(t, "owner", owner)
	eq(t, "repo", repo)
}

func TestParseGitHubRepoURLSimple(t *testing.T) {
	owner, repo := ParseGitHubRepoURLSimple("https://github.com/octocat/hello-world")
	eq(t, "octocat", owner)
	eq(t, "hello-world", repo)

	// 解析失败返回空字符串
	owner, repo = ParseGitHubRepoURLSimple("")
	isEmpty(t, owner)
	isEmpty(t, repo)

	owner, repo = ParseGitHubRepoURLSimple("not-a-valid-url")
	isEmpty(t, owner)
	isEmpty(t, repo)
}

func TestParseGiteeRepoURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		// 正常路径
		{"https 标准格式", "https://gitee.com/owner/repo", "owner", "repo", false},
		{"https 带 .git", "https://gitee.com/owner/repo.git", "owner", "repo", false},
		{"SSH 格式", "git@gitee.com:owner/repo.git", "owner", "repo", false},
		{"简写 owner/repo", "owner/repo", "owner", "repo", false},

		// 边界与非法输入
		{"空字符串", "", "", "", true},
		{"多级路径", "https://gitee.com/a/b/c", "", "", true},
		{"只有 owner", "gitee.com/owner", "", "", true},
		{"owner 为空", "gitee.com//repo", "", "", true},
		{"非法字符 url", "https://gitee.com/!!/??", "!!", "??", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseGiteeRepoURL(tt.url)
			if tt.wantErr {
				isErr(t, err)
				return
			}
			noErr(t, err)
			eq(t, tt.wantOwner, owner)
			eq(t, tt.wantRepo, repo)
		})
	}
}

func TestParseGitRepoURL(t *testing.T) {
	// GitHub
	owner, repo, err := ParseGitRepoURL("https://github.com/octocat/hello")
	noErr(t, err)
	eq(t, "octocat", owner)
	eq(t, "hello", repo)

	// Gitee
	owner, repo, err = ParseGitRepoURL("git@gitee.com:foo/bar.git")
	noErr(t, err)
	eq(t, "foo", owner)
	eq(t, "bar", repo)

	// 不支持的仓库
	_, _, err = ParseGitRepoURL("https://gitlab.com/foo/bar")
	errContains(t, err, "不支持的 Git 仓库 URL")

	// 空字符串： 未包含 github.com / gitee.com，走不支持分支
	_, _, err = ParseGitRepoURL("")
	isErr(t, err)

	// 包含 github.com 但格式非法
	_, _, err = ParseGitRepoURL("https://github.com/a/b/c")
	isErr(t, err)
}
