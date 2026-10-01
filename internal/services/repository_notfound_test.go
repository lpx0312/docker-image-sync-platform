package services

import (
	"errors"
	"testing"
)

// TestIsRepositoryNotFound 验证“仓库不存在”的判定边界：
// 只有 tags/list 阶段的 404（及各自的特例）才算不存在；
// Token 获取/认证阶段的失败应归类为检查失败，而非仓库不存在
func TestIsRepositoryNotFound(t *testing.T) {
	acr := NewAcrAPIService()
	v2 := newBearerV2Client("test")
	generic := NewGenericAPIService()

	cases := []struct {
		name string
		err  error
		acr  bool // ACR：对不存在仓库可能返回 401（阿里云特例）
		v2   bool // SWR/CCR：不存在仓库走 404 或 200+nil tags
		gen  bool // Harbor/Generic：404 不存在、403 无权限
	}{
		{name: "tags列表404", err: errors.New("获取Tag列表失败: HTTP 404"), acr: true, v2: true, gen: true},
		{name: "tags列表401", err: errors.New("获取Tag列表失败: HTTP 401"), acr: true, v2: false, gen: false},
		{name: "tags列表403无权限", err: errors.New("获取Tag列表失败: HTTP 403"), acr: false, v2: false, gen: true},
		{name: "Token获取401密码错误", err: errors.New("获取Token失败: HTTP 401"), acr: false, v2: false, gen: false},
		{name: "Token认证被拒绝", err: errors.New("获取SWR Token失败: 认证被拒绝（凭证错误或无权限）"), acr: false, v2: false, gen: false},
		{name: "Basic认证被拒绝", err: errors.New("获取Tag列表失败: 认证被拒绝（HTTP 401）"), acr: false, v2: false, gen: false},
		{name: "Token用户名密码错误", err: errors.New("获取Token失败: 认证被拒绝（用户名或密码错误）"), acr: false, v2: false, gen: false},
		{name: "网络错误", err: errors.New("获取Tag列表失败: dial tcp: timeout"), acr: false, v2: false, gen: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := acr.IsRepositoryNotFound(tt.err); got != tt.acr {
				t.Errorf("AcrAPIService.IsRepositoryNotFound(%v) = %v, want %v", tt.err, got, tt.acr)
			}
			if got := v2.IsRepositoryNotFound(tt.err); got != tt.v2 {
				t.Errorf("bearerV2Client.IsRepositoryNotFound(%v) = %v, want %v", tt.err, got, tt.v2)
			}
			if got := generic.IsRepositoryNotFound(tt.err); got != tt.gen {
				t.Errorf("GenericAPIService.IsRepositoryNotFound(%v) = %v, want %v", tt.err, got, tt.gen)
			}
		})
	}

	if acr.IsRepositoryNotFound(nil) || v2.IsRepositoryNotFound(nil) || generic.IsRepositoryNotFound(nil) {
		t.Error("nil error 不应判定为仓库不存在")
	}
}
