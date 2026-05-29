package services

import "testing"

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"nginx", "nginx"},
		{"nginx:latest", "nginx"},
		{"centos", "centos"},
		{"library/nginx", "nginx"},
		{"gitlab/gitlab-ce", "gitlab-ce"},
		{"jenkins/jenkins", "jenkins"},
		{"e2e-test-images/agnhost", "agnhost"},
		{"eipwork/kuboard-spray", "kuboard-spray"},
		{"gcr.io/google-containers/pause", "pause"},
		{"registry.cn-hangzhou.aliyuncs.com/namespace/image", "image"},
		{"registry:5000/repo:tag", "repo"},
		{"", ""},
		{"  redis  ", "redis"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ExtractRepoName(tt.input); got != tt.want {
				t.Errorf("ExtractRepoName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
