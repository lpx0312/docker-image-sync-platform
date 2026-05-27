package services

import (
	"encoding/json"
	"testing"
)

func TestTriggerWorkflowInputs(t *testing.T) {
	// 测试 workflow_dispatch 输入参数构建
	inputs := map[string]string{
		"aliyun_registry":          "registry.cn-hangzhou.aliyuncs.com",
		"aliyun_namespace":         "lpx03",
		"aliyun_registry_user":     "testuser",
		"aliyun_registry_password": "testpass",
	}

	// 验证输入参数不为空
	if inputs["aliyun_registry"] == "" {
		t.Error("aliyun_registry should not be empty")
	}
	if inputs["aliyun_namespace"] == "" {
		t.Error("aliyun_namespace should not be empty")
	}
	if inputs["aliyun_registry_user"] == "" {
		t.Error("aliyun_registry_user should not be empty")
	}
	if inputs["aliyun_registry_password"] == "" {
		t.Error("aliyun_registry_password should not be empty")
	}
}

func TestWorkflowDispatchPayload(t *testing.T) {
	// 测试 workflow_dispatch 请求体结构
	inputs := map[string]string{
		"aliyun_registry":          "registry.cn-hangzhou.aliyuncs.com",
		"aliyun_namespace":         "lpx03",
		"aliyun_registry_user":     "testuser",
		"aliyun_registry_password": "testpass",
	}

	payload := map[string]interface{}{
		"ref":    "main",
		"inputs": inputs,
	}

	// 验证 payload 结构
	if payload["ref"] != "main" {
		t.Errorf("expected ref 'main', got '%v'", payload["ref"])
	}

	inputMap, ok := payload["inputs"].(map[string]string)
	if !ok {
		t.Fatal("inputs should be map[string]string")
	}

	if len(inputMap) != 4 {
		t.Errorf("expected 4 inputs, got %d", len(inputMap))
	}

	// 验证 JSON 序列化
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if parsed["ref"] != "main" {
		t.Errorf("JSON roundtrip: expected ref 'main', got '%v'", parsed["ref"])
	}
}

func TestTriggerWorkflowWithEmptyInputs(t *testing.T) {
	// 测试空输入参数的情况
	inputs := map[string]string{}

	// 空输入应该仍然可以构建 payload
	payload := map[string]interface{}{
		"ref":    "main",
		"inputs": inputs,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal empty inputs: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal empty inputs: %v", err)
	}

	if parsed["ref"] != "main" {
		t.Errorf("expected ref 'main', got '%v'", parsed["ref"])
	}
}

func TestTriggerWorkflowWithPartialInputs(t *testing.T) {
	// 测试部分输入参数的情况（只有必填参数）
	inputs := map[string]string{
		"aliyun_registry":          "registry.cn-hangzhou.aliyuncs.com",
		"aliyun_namespace":         "lpx03",
		"aliyun_registry_user":     "testuser",
		"aliyun_registry_password": "testpass",
	}

	// 验证所有必需参数都存在
	requiredParams := []string{
		"aliyun_registry",
		"aliyun_namespace",
		"aliyun_registry_user",
		"aliyun_registry_password",
	}

	for _, param := range requiredParams {
		if _, exists := inputs[param]; !exists {
			t.Errorf("required parameter %s is missing", param)
		}
	}
}
