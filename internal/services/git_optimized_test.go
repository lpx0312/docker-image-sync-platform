package services

import (
	"strings"
	"testing"
)

// TestBuildAuthURL 验证凭证嵌入 URL 的正确性（修复 GIT_USERNAME/GIT_PASSWORD
// 环境变量不被 git CLI 认可的问题），含特殊字符的百分号编码
func TestBuildAuthURL(t *testing.T) {
	s := &GitOptimizedService{}

	tests := []struct {
		name      string
		repoURL   string
		username  string
		token     string
		repoType  string
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "github带token",
			repoURL:  "https://github.com/owner/repo.git",
			username: "alice",
			token:    "ghp_abc123",
			repoType: "github",
			want:     "https://alice:ghp_abc123@github.com/owner/repo.git",
		},
		{
			name:     "token含斜杠应被编码",
			repoURL:  "https://gitee.com/owner/repo.git",
			username: "bob",
			token:    "to/ken",
			repoType: "gitee",
			want:     "https://bob:to%2Fken@gitee.com/owner/repo.git",
		},
		{
			name:      "github无token应报错",
			repoURL:   "https://github.com/owner/repo.git",
			username:  "alice",
			token:     "",
			repoType:  "github",
			wantErr:   true,
			errSubstr: "访问令牌",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.buildAuthURL(tt.repoURL, tt.username, tt.token, tt.repoType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望报错, 实际返回 %q", got)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("错误信息 %q 不包含 %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildAuthURL 报错: %v", err)
			}
			if got != tt.want {
				t.Errorf("buildAuthURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
