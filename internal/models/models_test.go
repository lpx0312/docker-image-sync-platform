package models

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"
)

// 说明：本仓库 go.mod 中 testify v1.12.1 依赖的 go.yaml.in/yaml/v3 模块未在
// go.mod 声明，testify 实际不可用（internal/utils 既有测试同样无法编译）。
// 按约定不改 go.mod/go.sum，因此测试统一使用标准库 + 下列轻量断言助手。

// 固定时间：无单调时钟、UTC，保证 JSON 往返后 DeepEqual 成立
func fixedTime() time.Time {
	return time.Date(2024, 5, 1, 8, 30, 0, 123456789, time.UTC)
}

func fixedTimePtr() *time.Time {
	t := fixedTime()
	return &t
}

// ---------- 轻量断言助手（仅测试使用） ----------

func eq(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("不相等:\nwant: %#v\n got: %#v", want, got)
	}
}

func ne(t *testing.T, a, b any) {
	t.Helper()
	if reflect.DeepEqual(a, b) {
		t.Errorf("不应相等: %#v", a)
	}
}

func isTrue(t *testing.T, cond bool, format string, args ...any) {
	t.Helper()
	if !cond {
		t.Errorf("条件为假: "+format, args...)
	}
}

func isFalse(t *testing.T, cond bool, format string, args ...any) {
	t.Helper()
	if cond {
		t.Errorf("条件为真: "+format, args...)
	}
}

func ok(t *testing.T, err error, format string, args ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf("意外错误: %v; "+format, append([]any{err}, args...)...)
	}
}

func bad(t *testing.T, err error, format string, args ...any) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误但得到 nil; "+format, args...)
	}
}

func hasLen(t *testing.T, x any, want int) {
	t.Helper()
	v := reflect.ValueOf(x)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Map && v.Kind() != reflect.String {
		t.Fatalf("hasLen 不支持的类型 %v", v.Kind())
	}
	if v.Len() != want {
		t.Errorf("长度不符: want %d got %d", want, v.Len())
	}
}

// ---------- 常量与枚举 ----------

func TestRoleConstants(t *testing.T) {
	eq(t, "admin", RoleAdmin)
	eq(t, "operator", RoleOperator)
	eq(t, "user", RoleUser)
	ne(t, RoleAdmin, RoleOperator)
	ne(t, RoleOperator, RoleUser)
}

func TestPermissionConstants(t *testing.T) {
	perms := []string{PermSync, PermImages, PermGitHub, PermConfig, PermUsers, PermRoles}
	eq(t, []string{"sync", "images", "github", "config", "users", "roles"}, perms)
	seen := map[string]bool{}
	for _, p := range perms {
		isFalse(t, seen[p], "权限常量重复: %s", p)
		seen[p] = true
	}
}

func TestUserStatusConstants(t *testing.T) {
	eq(t, "active", UserStatusActive)
	eq(t, "disabled", UserStatusDisabled)
	ne(t, UserStatusActive, UserStatusDisabled)
}

func TestLoginStatusConstants(t *testing.T) {
	eq(t, "success", LoginStatusSuccess)
	eq(t, "failed", LoginStatusFailed)
	ne(t, LoginStatusSuccess, LoginStatusFailed)
}

func TestSyncStatusConstants(t *testing.T) {
	all := []string{
		SyncStatusPending, SyncStatusSyncing, SyncStatusSuccess,
		SyncStatusFailed, SyncStatusRetrying, SyncStatusSkipped,
	}
	eq(t, []string{"pending", "syncing", "success", "failed", "retrying", "skipped"}, all)
	seen := map[string]bool{}
	for _, s := range all {
		isFalse(t, seen[s], "同步状态常量重复: %s", s)
		seen[s] = true
	}
}

func TestTaskStatusConstants(t *testing.T) {
	eq(t, "pending", TaskStatusPending)
	eq(t, "running", TaskStatusRunning)
	eq(t, "completed", TaskStatusCompleted)
	eq(t, "failed", TaskStatusFailed)
	eq(t, "partial_success", TaskStatusPartialSuccess)
	eq(t, "paused", TaskStatusPaused)
	// TaskStatusSuccess 是 completed 的向后兼容别名
	eq(t, TaskStatusCompleted, TaskStatusSuccess)
}

// ---------- Registry 类型 ----------

func TestRegistryTypeConstants(t *testing.T) {
	eq(t, "acr", RegistryTypeACR)
	eq(t, "swr", RegistryTypeSWR)
	eq(t, "ccr", RegistryTypeCCR)
	eq(t, "harbor", RegistryTypeHarbor)
	eq(t, "generic", RegistryTypeGeneric)
}

func TestRegistryTypeLabels(t *testing.T) {
	for _, rt := range []string{RegistryTypeACR, RegistryTypeSWR, RegistryTypeCCR, RegistryTypeHarbor, RegistryTypeGeneric} {
		label, exists := RegistryTypeLabels[rt]
		isTrue(t, exists, "缺少类型 %s 的标签", rt)
		isFalse(t, label == "", "类型 %s 标签为空", rt)
	}
	hasLen(t, RegistryTypeLabels, 5)
}

func TestRegistryTypeLabelsConcurrentReads(t *testing.T) {
	// 只读 map 的并发读取应安全（配合 -race 验证）
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = RegistryTypeLabels[RegistryTypeACR]
			}
		}()
	}
	wg.Wait()
}

func TestIsValidRegistryType(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		// 全部合法类型
		{RegistryTypeACR, true},
		{RegistryTypeSWR, true},
		{RegistryTypeCCR, true},
		{RegistryTypeHarbor, true},
		{RegistryTypeGeneric, true},
		// 边界：空串、大小写、前后空格
		{"", false},
		{"ACR", false},
		{"Harbor", false},
		{" acr", false},
		{"acr ", false},
		// 非法值
		{"unknown", false},
		{"ecr", false},
		{"docker", false},
		{"acr,swr", false},
		// unicode
		{"阿里云", false},
		{"ａｃｒ", false}, // 全角字符
		// 超长输入
		{string(make([]byte, 1<<16)), false},
		// 带特殊字符
		{"acr\n", false},
		{"acr\x00", false},
	}
	for _, tt := range tests {
		got := IsValidRegistryType(tt.in)
		if got != tt.want {
			t.Errorf("IsValidRegistryType(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// ---------- TableName ----------

func TestTableNames(t *testing.T) {
	eq(t, "image_sync_records", ImageSyncRecord{}.TableName())
	eq(t, "sync_tasks", SyncTask{}.TableName())
	eq(t, "system_configs", SystemConfig{}.TableName())
	eq(t, "users", User{}.TableName())
	eq(t, "login_logs", LoginLog{}.TableName())
	eq(t, "permissions", Permission{}.TableName())
	eq(t, "roles", Role{}.TableName())
}

// ---------- Seed 数据完整性 ----------

func TestDefaultPermissionSeedsIntegrity(t *testing.T) {
	if len(DefaultPermissionSeeds) == 0 {
		t.Fatal("DefaultPermissionSeeds 为空")
	}

	codeSet := map[string]bool{}
	sortOrders := map[int]bool{}
	for _, p := range DefaultPermissionSeeds {
		isFalse(t, p.Code == "", "权限 code 为空")
		isFalse(t, p.Name == "", "权限 %s 名称为空", p.Code)
		isFalse(t, codeSet[p.Code], "权限 code 重复: %s", p.Code)
		codeSet[p.Code] = true

		isFalse(t, sortOrders[p.SortOrder], "权限 %s SortOrder 重复: %d", p.Code, p.SortOrder)
		sortOrders[p.SortOrder] = true
		isTrue(t, p.SortOrder > 0, "权限 %s SortOrder 应为正数", p.Code)
	}

	// seed 必须覆盖全部权限常量
	for _, c := range []string{PermSync, PermImages, PermGitHub, PermConfig, PermUsers, PermRoles} {
		isTrue(t, codeSet[c], "权限 seed 缺少常量 %s", c)
	}
}

func TestDefaultRoleSeedsIntegrity(t *testing.T) {
	if len(DefaultRoleSeeds) != 3 {
		t.Fatalf("DefaultRoleSeeds 应有 3 个角色, got %d", len(DefaultRoleSeeds))
	}

	// 收集全部已注册权限 code
	permSet := map[string]bool{}
	for _, p := range DefaultPermissionSeeds {
		permSet[p.Code] = true
	}

	codeSet := map[string]bool{}
	for _, r := range DefaultRoleSeeds {
		isFalse(t, r.Code == "", "角色 code 为空")
		isFalse(t, r.Name == "", "角色 %s 名称为空", r.Code)
		isFalse(t, codeSet[r.Code], "角色 code 重复: %s", r.Code)
		codeSet[r.Code] = true
		isTrue(t, r.IsSystem, "seed 角色 %s 应为系统角色", r.Code)
		if len(r.Permissions) == 0 {
			t.Fatalf("角色 %s 未配置任何权限", r.Code)
		}

		for _, p := range r.Permissions {
			isTrue(t, permSet[p], "角色 %s 引用了不存在的权限 %s", r.Code, p)
		}
	}

	// 三个默认角色齐备
	for _, c := range []string{RoleAdmin, RoleOperator, RoleUser} {
		isTrue(t, codeSet[c], "缺少默认角色 %s", c)
	}

	// admin 拥有全部权限，user 仅 sync
	for _, r := range DefaultRoleSeeds {
		switch r.Code {
		case RoleAdmin:
			eq(t, len(DefaultPermissionSeeds), len(r.Permissions))
		case RoleUser:
			eq(t, []string{PermSync}, r.Permissions)
		}
	}
}

// ---------- JSON 序列化往返 ----------

func jsonKeys(t *testing.T, v any) map[string]json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	ok(t, err, "Marshal 失败")
	m := map[string]json.RawMessage{}
	ok(t, json.Unmarshal(data, &m), "Unmarshal 为 map 失败")
	return m
}

func jsonRoundTrip(t *testing.T, in any, out any) {
	t.Helper()
	data, err := json.Marshal(in)
	ok(t, err, "Marshal 失败")
	ok(t, json.Unmarshal(data, out), "Unmarshal 失败")
}

func TestImageSyncRecordJSONRoundTrip(t *testing.T) {
	started, completed := fixedTimePtr(), fixedTimePtr()
	in := ImageSyncRecord{
		ID:               42,
		OriginalImage:    "docker.io/library/nginx:1.25",
		ACRImage:         "registry.example.com/team/nginx:1.25",
		Tag:              "1.25",
		Architecture:     "amd64",
		AcrArchitectures: `["amd64","arm64"]`,
		AcrRegistryID:    7,
		OriginalInput:    "nginx:1.25",
		InputOrder:       3,
		SyncStatus:       SyncStatusSuccess,
		ErrorMessage:     "",
		Description:      "基础镜像",
		TaskID:           "task-abc",
		Priority:         10,
		StartedAt:        started,
		CompletedAt:      completed,
		Duration:         120,
		ImageSize:        1024,
		CreatedAt:        fixedTime(),
		UpdatedAt:        fixedTime(),
	}

	var out ImageSyncRecord
	jsonRoundTrip(t, &in, &out)
	if !reflect.DeepEqual(in, out) {
		t.Errorf("ImageSyncRecord JSON 往返不一致:\nin=%+v\nout=%+v", in, out)
	}

	keys := jsonKeys(t, &in)
	// snake_case 字段名
	for _, k := range []string{
		"id", "original_image", "acr_image", "tag", "architecture", "acr_architectures",
		"acr_registry_id", "original_input", "input_order", "sync_status", "error_message",
		"description", "task_id", "priority", "started_at", "completed_at", "duration",
		"image_size", "created_at", "updated_at",
	} {
		if _, exists := keys[k]; !exists {
			t.Errorf("JSON 缺少字段 %s", k)
		}
	}
	// 关联字段 omitempty：未设置时不出现在 JSON 中
	if _, exists := keys["acr_registry"]; exists {
		t.Error("acr_registry 应为 omitempty")
	}
}

func TestSyncTaskJSONRoundTrip(t *testing.T) {
	in := SyncTask{
		ID: 1, TaskID: "t-1", ImagesJSON: `["a","b"]`, Status: TaskStatusRunning,
		GitHubActionURL: "https://github.com/x/y/actions/runs/1",
		GitHubRunID:     "1", CommitSHA: "abc123",
		StartedAt: fixedTimePtr(), CompletedAt: nil,
		ErrorMessage: "", AcrRegistryID: 2, Description: "desc",
		MaxConcurrent: 3, TotalImages: 10, CompletedImages: 5, FailedImages: 1,
		Progress: 50.5, CreatedAt: fixedTime(), UpdatedAt: fixedTime(),
	}
	var out SyncTask
	jsonRoundTrip(t, &in, &out)
	if !reflect.DeepEqual(in, out) {
		t.Errorf("SyncTask JSON 往返不一致:\nin=%+v\nout=%+v", in, out)
	}

	keys := jsonKeys(t, &in)
	for _, k := range []string{
		"id", "task_id", "images_json", "status", "github_action_url", "github_run_id",
		"commit_sha", "started_at", "completed_at", "error_message", "acr_registry_id",
		"description", "max_concurrent", "total_images", "completed_images",
		"failed_images", "progress", "created_at", "updated_at",
	} {
		if _, exists := keys[k]; !exists {
			t.Errorf("JSON 缺少字段 %s", k)
		}
	}
	// completed_at 为 nil 时序列化为 null
	eq(t, "null", string(keys["completed_at"]))
}

func TestSystemConfigJSONRoundTrip(t *testing.T) {
	in := SystemConfig{
		ID: 1, ConfigKey: "acr.registry", ConfigValue: "registry.example.com",
		Description: "ACR 地址", IsEncrypted: true, ConfigGroup: "default",
		DisplayOrder: 2, CreatedAt: fixedTime(), UpdatedAt: fixedTime(),
	}
	var out SystemConfig
	jsonRoundTrip(t, &in, &out)
	if !reflect.DeepEqual(in, out) {
		t.Errorf("SystemConfig JSON 往返不一致")
	}

	keys := jsonKeys(t, &in)
	for _, k := range []string{"id", "config_key", "config_value", "description", "is_encrypted", "config_group", "display_order"} {
		if _, exists := keys[k]; !exists {
			t.Errorf("JSON 缺少字段 %s", k)
		}
	}
	eq(t, "true", string(keys["is_encrypted"]))
}

func TestUserJSONHidesPasswordHash(t *testing.T) {
	in := User{
		ID: 1, Username: "alice", PasswordHash: "$2a$10$secret",
		Email: "alice@example.com", RoleID: 2,
		Role:   &Role{ID: 2, Code: RoleAdmin, Name: "管理员"},
		Status: UserStatusActive, LastLoginAt: fixedTimePtr(),
		CreatedAt: fixedTime(), UpdatedAt: fixedTime(),
	}
	keys := jsonKeys(t, &in)
	// 敏感字段不序列化
	if _, exists := keys["password_hash"]; exists {
		t.Error("password_hash 不应出现在 JSON 中")
	}
	if _, exists := keys["username"]; !exists {
		t.Error("JSON 缺少 username")
	}
	if _, exists := keys["role"]; !exists {
		t.Error("JSON 应包含非空 role")
	}
	if _, exists := keys["last_login_at"]; !exists {
		t.Error("JSON 缺少 last_login_at")
	}

	// Role 为 nil 时 omitempty
	keys = jsonKeys(t, &User{ID: 2, Username: "bob", RoleID: 1})
	if _, exists := keys["role"]; exists {
		t.Error("nil Role 应为 omitempty")
	}
	if _, exists := keys["password_hash"]; exists {
		t.Error("password_hash 不应出现在 JSON 中")
	}
}

func TestLoginLogJSONRoundTrip(t *testing.T) {
	in := LoginLog{
		ID: 1, UserID: 2, Username: "alice", IP: "192.168.1.1",
		UserAgent: "unit-test", Status: LoginStatusSuccess, Message: "ok",
		CreatedAt: fixedTime(),
	}
	var out LoginLog
	jsonRoundTrip(t, &in, &out)
	if !reflect.DeepEqual(in, out) {
		t.Errorf("LoginLog JSON 往返不一致")
	}
}

func TestPermissionRoleJSON(t *testing.T) {
	p := Permission{
		ID: 1, Code: PermSync, Name: "镜像同步",
		Description: "提交同步任务与查看同步历史", SortOrder: 1,
		CreatedAt: fixedTime(), UpdatedAt: fixedTime(),
	}
	var pOut Permission
	jsonRoundTrip(t, &p, &pOut)
	if !reflect.DeepEqual(p, pOut) {
		t.Errorf("Permission JSON 往返不一致")
	}

	r := Role{ID: 1, Code: RoleAdmin, Name: "管理员", IsSystem: true, Permissions: []Permission{p}}
	var rOut Role
	jsonRoundTrip(t, &r, &rOut)
	if !reflect.DeepEqual(r, rOut) {
		t.Errorf("Role JSON 往返不一致")
	}

	// Permissions 为 nil 时 omitempty
	keys := jsonKeys(t, &Role{ID: 2, Code: RoleUser, Name: "普通用户"})
	if _, exists := keys["permissions"]; exists {
		t.Error("nil Permissions 应为 omitempty")
	}
}

func TestAcrRegistryJSONHidesSecrets(t *testing.T) {
	in := AcrRegistry{
		ID: 1, RegistryURL: "registry.example.com", Namespace: "team",
		Alias: "prod", Username: "user", Password: "plain-secret",
		AuthServer: "auth.example.com", DockerService: "service",
		RegistryType: RegistryTypeACR, AccessKey: "AK", SecretKey: "SK",
		IsDefault: true, CreatedAt: fixedTime(), UpdatedAt: fixedTime(),
	}
	keys := jsonKeys(t, &in)
	// Password / SecretKey 均为 json:"-"，不得泄漏
	if _, exists := keys["password"]; exists {
		t.Error("password 不应出现在 JSON 中")
	}
	if _, exists := keys["secret_key"]; exists {
		t.Error("secret_key 不应出现在 JSON 中")
	}
	if _, exists := keys["access_key"]; !exists {
		t.Error("JSON 缺少 access_key")
	}
	if _, exists := keys["registry_type"]; !exists {
		t.Error("JSON 缺少 registry_type")
	}

	var out AcrRegistry
	jsonRoundTrip(t, &in, &out)
	eq(t, in.ID, out.ID)
	eq(t, in.RegistryURL, out.RegistryURL)
	// json:"-" 字段往返后为空
	if out.Password != "" || out.SecretKey != "" {
		t.Error("json:\"-\" 字段往返后应为空")
	}
}

func TestRequestResponseJSONRoundTrip(t *testing.T) {
	t.Run("ImageRequest", func(t *testing.T) {
		in := ImageRequest{Images: []string{"nginx", "redis:7"}, Architecture: "arm64"}
		var out ImageRequest
		jsonRoundTrip(t, &in, &out)
		eq(t, in, out)
	})

	t.Run("SyncRequest", func(t *testing.T) {
		in := SyncRequest{
			Images: []string{"nginx"}, Architecture: "amd64",
			Description: "d", AcrRegistryID: 3,
		}
		var out SyncRequest
		jsonRoundTrip(t, &in, &out)
		eq(t, in, out)
	})

	t.Run("BatchSyncRequest", func(t *testing.T) {
		in := BatchSyncRequest{
			Images: []ImageSyncItem{{
				SourceImage: "nginx", TargetTag: "latest",
				Architecture: "amd64", Priority: 5, Description: "x",
			}},
			MaxConcurrent: 4, AcrRegistryID: 1,
		}
		var out BatchSyncRequest
		jsonRoundTrip(t, &in, &out)
		eq(t, in, out)
	})

	t.Run("CheckAcrRequest", func(t *testing.T) {
		in := CheckAcrRequest{Images: []string{"a", "b"}, AcrRegistryID: 9}
		var out CheckAcrRequest
		jsonRoundTrip(t, &in, &out)
		eq(t, in, out)
	})

	t.Run("AcrRegistryRequest", func(t *testing.T) {
		in := AcrRegistryRequest{
			RegistryURL: "u", Namespace: "n", Alias: "a", Username: "user",
			Password: "p", AuthServer: "s", DockerService: "d",
			RegistryType: RegistryTypeSWR, AccessKey: "ak", SecretKey: "sk",
		}
		var out AcrRegistryRequest
		jsonRoundTrip(t, &in, &out)
		eq(t, in, out)
	})

	t.Run("AcrRegistryUpdateRequest", func(t *testing.T) {
		in := AcrRegistryUpdateRequest{RegistryURL: "u", Namespace: "n", Alias: "a"}
		var out AcrRegistryUpdateRequest
		jsonRoundTrip(t, &in, &out)
		eq(t, in, out)
	})

	t.Run("AcrRepositoryRequests", func(t *testing.T) {
		in := AcrRepositoryRequest{AcrRegistryID: 1, RepositoryName: "nginx"}
		var out AcrRepositoryRequest
		jsonRoundTrip(t, &in, &out)
		eq(t, in, out)

		bIn := AcrRepositoryBatchRequest{AcrRegistryID: 1, RepositoryNames: []string{"a", "b"}}
		var bOut AcrRepositoryBatchRequest
		jsonRoundTrip(t, &bIn, &bOut)
		eq(t, bIn, bOut)

		sIn := AcrRepositorySyncRequest{AcrRegistryID: 2}
		var sOut AcrRepositorySyncRequest
		jsonRoundTrip(t, &sIn, &sOut)
		eq(t, sIn, sOut)

		dIn := AcrRepositoryBatchDeleteRequest{IDs: []uint{1, 2, 3}}
		var dOut AcrRepositoryBatchDeleteRequest
		jsonRoundTrip(t, &dIn, &dOut)
		eq(t, dIn, dOut)

		cIn := AcrRepositoryCleanInvalidRequest{AcrRegistryID: 5}
		var cOut AcrRepositoryCleanInvalidRequest
		jsonRoundTrip(t, &cIn, &cOut)
		eq(t, cIn, cOut)
	})

	t.Run("BatchSyncResponse", func(t *testing.T) {
		in := BatchSyncResponse{TaskID: "t-1", Message: "ok", ImageCount: 3, EstimatedTime: "10m"}
		var out BatchSyncResponse
		jsonRoundTrip(t, &in, &out)
		eq(t, in, out)

		keys := jsonKeys(t, &in)
		for _, k := range []string{"task_id", "message", "image_count", "estimated_time"} {
			if _, exists := keys[k]; !exists {
				t.Errorf("JSON 缺少字段 %s", k)
			}
		}
	})

	t.Run("SyncResponse", func(t *testing.T) {
		in := SyncResponse{TaskID: "t-2", Message: "ok"}
		var out SyncResponse
		jsonRoundTrip(t, &in, &out)
		eq(t, in, out)
	})

	t.Run("ImageListResponse", func(t *testing.T) {
		in := ImageListResponse{Total: 1, Data: []*ImageSyncRecord{{ID: 1, OriginalImage: "nginx"}}}
		var out ImageListResponse
		jsonRoundTrip(t, &in, &out)
		if len(out.Data) != 1 {
			t.Fatalf("Data 长度不符: %d", len(out.Data))
		}
		eq(t, uint(1), out.Data[0].ID)
	})

	t.Run("TaskStatusResponse", func(t *testing.T) {
		in := TaskStatusResponse{
			TaskID: "t", Status: TaskStatusCompleted, SourceImage: "nginx",
			TargetImage: "acr/nginx", Architecture: "amd64",
			GitHubActionURL: "https://github.com", StartedAt: fixedTimePtr(),
			CompletedAt: fixedTimePtr(), ErrorMessage: "", Images: []string{"nginx"},
		}
		var out TaskStatusResponse
		jsonRoundTrip(t, &in, &out)
		eq(t, in.TaskID, out.TaskID)
		eq(t, in.Status, out.Status)
		eq(t, in.Images, out.Images)
	})

	t.Run("BatchTaskStatusResponse", func(t *testing.T) {
		in := BatchTaskStatusResponse{
			TaskID: "t", Status: TaskStatusRunning, Description: "d",
			TotalImages: 10, CompletedImages: 4, FailedImages: 1,
			Progress: 40.0, MaxConcurrent: 3, GitHubActionURL: "u",
			StartedAt: fixedTimePtr(), ErrorMessage: "",
			ImageDetails: []ImageSyncDetailResponse{{
				ID: 1, OriginalImage: "nginx", ACRImage: "acr/nginx", Tag: "latest",
				Architecture: "amd64", SyncStatus: SyncStatusSuccess, ErrorMessage: "",
				Priority: 1, StartedAt: fixedTimePtr(), CompletedAt: fixedTimePtr(),
				Duration: 30, ImageSize: 2048,
			}},
			EstimatedTime: "5m",
		}
		var out BatchTaskStatusResponse
		jsonRoundTrip(t, &in, &out)
		if len(out.ImageDetails) != 1 {
			t.Fatalf("ImageDetails 长度不符: %d", len(out.ImageDetails))
		}
		eq(t, in.ImageDetails[0], out.ImageDetails[0])
		eq(t, in.Progress, out.Progress)
	})
}

// TestJSONZeroValues 零值 / nil / 空 切片的序列化行为
func TestJSONZeroValues(t *testing.T) {
	t.Run("TaskStatusResponse 空切片序列化为 null", func(t *testing.T) {
		keys := jsonKeys(t, &TaskStatusResponse{})
		if raw, exists := keys["images"]; !exists || string(raw) != "null" {
			t.Errorf("空 images 应为 null, got %v", string(raw))
		}
	})

	t.Run("ImageSyncRecord 零值全部字段存在且 DeletedAt 不出现", func(t *testing.T) {
		keys := jsonKeys(t, &ImageSyncRecord{})
		if _, exists := keys["id"]; !exists {
			t.Error("JSON 缺少 id")
		}
		if _, exists := keys["sync_status"]; !exists {
			t.Error("JSON 缺少 sync_status")
		}
		eq(t, "0", string(keys["id"]))
		if _, exists := keys["DeletedAt"]; exists {
			t.Error("DeletedAt json:\"-\" 不应出现")
		}
	})

	t.Run("unicode 与特殊字符", func(t *testing.T) {
		in := ImageSyncRecord{
			OriginalImage: "镜像/中文:标签",
			Description:   "emoji 🐳 测试 — &<>\"'",
			ErrorMessage:  "多字节 \x00 控制符",
		}
		var out ImageSyncRecord
		jsonRoundTrip(t, &in, &out)
		eq(t, in.OriginalImage, out.OriginalImage)
		eq(t, in.Description, out.Description)
		eq(t, in.ErrorMessage, out.ErrorMessage)
	})

	t.Run("超长字符串", func(t *testing.T) {
		long := make([]byte, 10000)
		for i := range long {
			long[i] = 'a' + byte(i%26)
		}
		in := ImageSyncRecord{OriginalImage: string(long)}
		var out ImageSyncRecord
		jsonRoundTrip(t, &in, &out)
		eq(t, in.OriginalImage, out.OriginalImage)
	})
}
