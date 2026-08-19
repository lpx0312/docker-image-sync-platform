package services

import (
	"encoding/json"
	"testing"
	"time"
)

// TestTc3Signer_Vector 用与独立参考实现对齐的固定向量校验 TC3 签名
func TestTc3Signer_Vector(t *testing.T) {
	const (
		secretID  = "TESTSID123"
		secretKey = "test-secret-key"
		wantSig   = "65dd5fb1ccca87fb12be929d2d74068d54c80a7326a2ef7bb36c4b9530fbacc6"
	)

	// 与 ccr_api.go 的调用保持一致：payload 为 map，Go json.Marshal 按键排序
	body, err := json.Marshal(map[string]interface{}{
		"Namespace": "lpx03",
		"Offset":    0,
		"Limit":     100,
	})
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	ts := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	signer := tc3Signer{SecretID: secretID, SecretKey: secretKey}
	got := signer.tc3AuthHeader("tcr.ap-guangzhou.tencentcloudapi.com", "tcr", "DescribeRepositoryFilterPersonal", body, ts)

	want := "TC3-HMAC-SHA256 Credential=" + secretID +
		"/2026-08-19/tcr/tc3_request, SignedHeaders=content-type;host;x-tc-action, Signature=" + wantSig
	if got != want {
		t.Errorf("Authorization =\n%s\nwant\n%s", got, want)
	}
}
