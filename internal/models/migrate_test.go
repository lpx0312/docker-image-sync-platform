package models

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 与生产 tag 等价的 SQLite DDL：ImageSyncRecord / SyncTask 的 gorm tag 使用了
// MySQL 专用的 type:enum(...)，glebarez/sqlite 解析失败（见 TestAutoMigrateOnSQLite）。
// 为继续验证这两个模型的列名映射、默认值、软删除与唯一约束，这里用等价 DDL 手工建表。
const imageSyncRecordsDDL = `
CREATE TABLE IF NOT EXISTS "image_sync_records" (
	"id" integer PRIMARY KEY AUTOINCREMENT,
	"original_image" varchar(500) NOT NULL,
	"acr_image" varchar(500),
	"tag" varchar(100),
	"architecture" varchar(50) DEFAULT '',
	"acr_architectures" text,
	"acr_registry_id" integer,
	"original_input" varchar(600),
	"input_order" integer DEFAULT 0,
	"sync_status" varchar(20) DEFAULT 'pending',
	"error_message" text,
	"description" varchar(500),
	"task_id" varchar(100),
	"priority" integer DEFAULT 0,
	"started_at" datetime,
	"completed_at" datetime,
	"duration" integer DEFAULT 0,
	"image_size" integer DEFAULT 0,
	"created_at" datetime,
	"updated_at" datetime,
	"deleted_at" datetime
)`

const syncTasksDDL = `
CREATE TABLE IF NOT EXISTS "sync_tasks" (
	"id" integer PRIMARY KEY AUTOINCREMENT,
	"task_id" varchar(100) NOT NULL UNIQUE,
	"images_json" text,
	"status" varchar(20) DEFAULT 'pending',
	"github_action_url" varchar(500),
	"github_run_id" varchar(100),
	"commit_sha" varchar(100),
	"started_at" datetime,
	"completed_at" datetime,
	"error_message" text,
	"acr_registry_id" integer DEFAULT 0,
	"description" varchar(500),
	"max_concurrent" integer DEFAULT 3,
	"total_images" integer DEFAULT 0,
	"completed_images" integer DEFAULT 0,
	"failed_images" integer DEFAULT 0,
	"progress" numeric DEFAULT 0.00,
	"created_at" datetime,
	"updated_at" datetime,
	"deleted_at" datetime
)`

// newModelTestDB 打开纯 Go 内存 SQLite 并迁移全部模型
func newModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	ok(t, err, "打开内存 SQLite 失败")
	// 无 enum 类型的模型直接 AutoMigrate
	ok(t, db.AutoMigrate(
		&SystemConfig{},
		&User{},
		&LoginLog{},
		&Permission{},
		&Role{},
		&AcrRegistry{},
		&AcrRepository{},
	), "AutoMigrate 非 enum 模型失败")
	// 含 MySQL enum tag 的两个模型 AutoMigrate 在 SQLite 上必然失败，改用等价 DDL
	ok(t, db.Exec(imageSyncRecordsDDL).Error, "手工建表 image_sync_records 失败")
	ok(t, db.Exec(syncTasksDDL).Error, "手工建表 sync_tasks 失败")
	return db
}

// TestAutoMigrateOnSQLite 验证全部模型的建表行为（glebarez 内存 SQLite）
//
// 当前实际行为：7 个模型 AutoMigrate 成功；ImageSyncRecord / SyncTask 因 gorm tag
// 使用 MySQL 专用 type:enum(...) 语法，SQLite 解析失败（疑似生产问题：模型不可移植，
// 也是 internal/services、internal/handlers 中 newTestDB 脚手架从未被调用的原因）。
func TestAutoMigrateOnSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	ok(t, err, "打开内存 SQLite 失败")

	// 1) 非 enum 模型：全部建表成功
	goodModels := []struct {
		model any
		table string
	}{
		{&SystemConfig{}, SystemConfig{}.TableName()},
		{&User{}, User{}.TableName()},
		{&LoginLog{}, LoginLog{}.TableName()},
		{&Permission{}, Permission{}.TableName()},
		{&Role{}, Role{}.TableName()},
		// AcrRegistry / AcrRepository 无自定义 TableName，使用 GORM 复数默认值
		{&AcrRegistry{}, "acr_registries"},
		{&AcrRepository{}, "acr_repositories"},
	}
	for _, m := range goodModels {
		ok(t, db.AutoMigrate(m.model), "AutoMigrate %s 失败", m.table)
		isTrue(t, db.Migrator().HasTable(m.table), "表 %s 未创建", m.table)
	}

	// 2) many2many 关联表由 Role 自动创建
	isTrue(t, db.Migrator().HasTable("role_permissions"), "表 role_permissions 未创建")

	// 3) 含 enum tag 的模型：AutoMigrate 失败（按当前实际行为断言）
	enumErr := db.AutoMigrate(&ImageSyncRecord{})
	bad(t, enumErr, "ImageSyncRecord 的 enum tag 应使 SQLite AutoMigrate 失败")
	isTrue(t, containsStr(enumErr.Error(), "syntax error"),
		"ImageSyncRecord AutoMigrate 错误应为语法错误, got: %v", enumErr)

	enumErr2 := db.AutoMigrate(&SyncTask{})
	bad(t, enumErr2, "SyncTask 的 enum tag 应使 SQLite AutoMigrate 失败")
	isTrue(t, containsStr(enumErr2.Error(), "syntax error"),
		"SyncTask AutoMigrate 错误应为语法错误, got: %v", enumErr2)
}

func containsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}

// TestAutoMigrateAllModels 验证全部模型都能在内存 SQLite 中建表成功
func TestAutoMigrateAllModels(t *testing.T) {
	db := newModelTestDB(t)

	expectedTables := []string{
		ImageSyncRecord{}.TableName(),
		SyncTask{}.TableName(),
		SystemConfig{}.TableName(),
		User{}.TableName(),
		LoginLog{}.TableName(),
		Permission{}.TableName(),
		Role{}.TableName(),
		// AcrRegistry / AcrRepository 无自定义 TableName，使用 GORM 复数默认值
		"acr_registries",
		"acr_repositories",
		// many2many 关联表由 Role 自动创建
		"role_permissions",
	}
	for _, table := range expectedTables {
		if !db.Migrator().HasTable(table) {
			t.Errorf("表 %s 未创建", table)
		}
	}
}

func TestImageSyncRecordCRUD(t *testing.T) {
	db := newModelTestDB(t)

	rec := ImageSyncRecord{
		OriginalImage:    "docker.io/library/nginx:1.25",
		ACRImage:         "registry.example.com/team/nginx:1.25",
		Tag:              "1.25",
		Architecture:     "amd64",
		AcrArchitectures: `["amd64","arm64"]`,
		AcrRegistryID:    1,
		OriginalInput:    "nginx:1.25",
		InputOrder:       0,
		SyncStatus:       SyncStatusPending,
		Description:      "基础镜像",
		TaskID:           "task-1",
		Priority:         1,
		Duration:         0,
		ImageSize:        0,
	}
	ok(t, db.Create(&rec).Error, "创建失败")
	if rec.ID == 0 {
		t.Fatal("主键未生成")
	}

	// 读回并校验字段
	var got ImageSyncRecord
	ok(t, db.First(&got, rec.ID).Error, "查询失败")
	eq(t, rec.OriginalImage, got.OriginalImage)
	eq(t, rec.ACRImage, got.ACRImage)
	eq(t, rec.Tag, got.Tag)
	eq(t, rec.Architecture, got.Architecture)
	eq(t, rec.AcrArchitectures, got.AcrArchitectures)
	eq(t, SyncStatusPending, got.SyncStatus)
	eq(t, "task-1", got.TaskID)
	isFalse(t, got.CreatedAt.IsZero(), "CreatedAt 应自动填充")
	isFalse(t, got.UpdatedAt.IsZero(), "UpdatedAt 应自动填充")

	// 更新状态流转 pending -> success
	ok(t, db.Model(&got).Updates(map[string]interface{}{
		"sync_status": SyncStatusSuccess,
		"duration":    95,
		"image_size":  123456,
	}).Error, "更新失败")
	var got2 ImageSyncRecord
	ok(t, db.First(&got2, rec.ID).Error, "再次查询失败")
	eq(t, SyncStatusSuccess, got2.SyncStatus)
	eq(t, int64(95), got2.Duration)
	eq(t, int64(123456), got2.ImageSize)
}

func TestImageSyncRecordUnicodeAndLongValues(t *testing.T) {
	db := newModelTestDB(t)

	long := make([]byte, 600)
	for i := range long {
		long[i] = 'a' + byte(i%26)
	}
	rec := ImageSyncRecord{
		OriginalImage: "镜像仓库/中文镜像:标签",
		Description:   "emoji 🐳 与中文描述",
		OriginalInput: string(long),
		ErrorMessage:  "错误：超时 & <>\"'",
		SyncStatus:    SyncStatusFailed,
	}
	ok(t, db.Create(&rec).Error, "创建失败")

	var got ImageSyncRecord
	ok(t, db.First(&got, rec.ID).Error, "查询失败")
	eq(t, "镜像仓库/中文镜像:标签", got.OriginalImage)
	eq(t, "emoji 🐳 与中文描述", got.Description)
	eq(t, string(long), got.OriginalInput)
	eq(t, "错误：超时 & <>\"'", got.ErrorMessage)
}

func TestImageSyncRecordZeroValueCreate(t *testing.T) {
	db := newModelTestDB(t)
	rec := ImageSyncRecord{OriginalImage: "nginx"}
	ok(t, db.Create(&rec).Error, "创建失败")

	var got ImageSyncRecord
	ok(t, db.First(&got, rec.ID).Error, "查询失败")
	eq(t, "", got.Tag)
	eq(t, "", got.Architecture)
	eq(t, 0, got.InputOrder)
	eq(t, 0, got.Priority)
	if got.StartedAt != nil || got.CompletedAt != nil {
		t.Error("零值时间指针应为 nil")
	}
}

func TestSyncTaskUniqueTaskID(t *testing.T) {
	db := newModelTestDB(t)
	first := SyncTask{TaskID: "dup-task", Status: TaskStatusPending, MaxConcurrent: 3}
	ok(t, db.Create(&first).Error, "创建失败")

	// 相同 TaskID 违反唯一索引
	dup := SyncTask{TaskID: "dup-task"}
	bad(t, db.Create(&dup).Error, "TaskID 唯一索引未生效")
}

func TestSyncTaskCRUD(t *testing.T) {
	db := newModelTestDB(t)
	task := SyncTask{
		TaskID: "task-uuid-1", ImagesJSON: `["nginx","redis"]`,
		Status: TaskStatusRunning, GitHubActionURL: "https://github.com/x",
		GitHubRunID: "991", CommitSHA: "abc", Description: "批量同步",
		MaxConcurrent: 5, TotalImages: 2, CompletedImages: 1, FailedImages: 0,
		Progress: 50.0,
	}
	ok(t, db.Create(&task).Error, "创建失败")

	var got SyncTask
	ok(t, db.Where("task_id = ?", "task-uuid-1").First(&got).Error, "查询失败")
	eq(t, TaskStatusRunning, got.Status)
	eq(t, 5, got.MaxConcurrent)
	eq(t, 2, got.TotalImages)
	eq(t, 50.0, got.Progress)

	// 进度推进
	ok(t, db.Model(&got).Update("progress", 100.0).Error, "更新失败")
	var got2 SyncTask
	ok(t, db.First(&got2, task.ID).Error, "再次查询失败")
	eq(t, 100.0, got2.Progress)
}

func TestSystemConfigUniqueKey(t *testing.T) {
	db := newModelTestDB(t)
	ok(t, db.Create(&SystemConfig{ConfigKey: "k1", ConfigValue: "v1"}).Error, "创建失败")

	bad(t, db.Create(&SystemConfig{ConfigKey: "k1"}).Error, "ConfigKey 唯一索引未生效")
}

func TestSystemConfigCRUD(t *testing.T) {
	db := newModelTestDB(t)
	cfg := SystemConfig{
		ConfigKey: "git.repo", ConfigValue: "https://gitee.com/x/y.git",
		Description: "仓库地址", IsEncrypted: false, ConfigGroup: "git", DisplayOrder: 3,
	}
	ok(t, db.Create(&cfg).Error, "创建失败")

	var got SystemConfig
	ok(t, db.Where("config_key = ?", "git.repo").First(&got).Error, "查询失败")
	eq(t, "https://gitee.com/x/y.git", got.ConfigValue)
	eq(t, "git", got.ConfigGroup)
	eq(t, 3, got.DisplayOrder)
	isFalse(t, got.IsEncrypted, "IsEncrypted 默认应为 false")
}

func TestRolePermissionManyToMany(t *testing.T) {
	db := newModelTestDB(t)

	// 用默认 seed 建权限
	ok(t, db.Create(&DefaultPermissionSeeds).Error, "权限 seed 入库失败")
	var allPerms []Permission
	ok(t, db.Find(&allPerms).Error, "查询权限失败")
	if len(allPerms) != 6 {
		t.Fatalf("权限数量不符: %d", len(allPerms))
	}

	var syncPerm Permission
	ok(t, db.Where("code = ?", PermSync).First(&syncPerm).Error, "查询 sync 权限失败")

	admin := Role{Code: RoleAdmin, Name: "管理员", IsSystem: true, Permissions: allPerms}
	user := Role{Code: RoleUser, Name: "普通用户", IsSystem: true, Permissions: []Permission{syncPerm}}
	ok(t, db.Create(&admin).Error, "创建 admin 失败")
	ok(t, db.Create(&user).Error, "创建 user 失败")

	// Preload 校验多对多关联
	var gotAdmin Role
	ok(t, db.Preload("Permissions").Where("code = ?", RoleAdmin).First(&gotAdmin).Error, "查询 admin 失败")
	eq(t, 6, len(gotAdmin.Permissions))

	var gotUser Role
	ok(t, db.Preload("Permissions").Where("code = ?", RoleUser).First(&gotUser).Error, "查询 user 失败")
	if len(gotUser.Permissions) != 1 {
		t.Fatalf("user 权限数不符: %d", len(gotUser.Permissions))
	}
	eq(t, PermSync, gotUser.Permissions[0].Code)

	// 中间表行数 = 6 + 1
	var count int64
	ok(t, db.Table("role_permissions").Count(&count).Error, "统计中间表失败")
	eq(t, int64(7), count)
}

func TestRoleUniqueCode(t *testing.T) {
	db := newModelTestDB(t)
	ok(t, db.Create(&Role{Code: "viewer", Name: "观察者"}).Error, "创建失败")
	bad(t, db.Create(&Role{Code: "viewer", Name: "重复"}).Error, "Role Code 唯一索引未生效")
}

func TestUserCRUDAndRoleAssociation(t *testing.T) {
	db := newModelTestDB(t)
	ok(t, db.Create(&DefaultPermissionSeeds).Error, "权限 seed 入库失败")

	role := Role{Code: RoleOperator, Name: "运维员", IsSystem: true}
	ok(t, db.Create(&role).Error, "创建角色失败")

	user := User{
		Username: "alice", PasswordHash: "$2a$10$hash",
		Email: "alice@example.com", RoleID: role.ID, Status: UserStatusActive,
	}
	ok(t, db.Create(&user).Error, "创建用户失败")

	// username 唯一
	bad(t, db.Create(&User{Username: "alice", PasswordHash: "x", RoleID: role.ID}).Error,
		"Username 唯一索引未生效")

	// 关联查询
	var got User
	ok(t, db.Preload("Role").Where("username = ?", "alice").First(&got).Error, "查询用户失败")
	if got.Role == nil {
		t.Fatal("Role 预加载失败")
	}
	eq(t, RoleOperator, got.Role.Code)
	eq(t, UserStatusActive, got.Status)
	if got.LastLoginAt != nil {
		t.Error("LastLoginAt 初始应为 nil")
	}

	// 更新登录时间与禁用状态
	now := fixedTime()
	ok(t, db.Model(&got).Updates(map[string]interface{}{
		"last_login_at": now,
		"status":        UserStatusDisabled,
	}).Error, "更新用户失败")
	var got2 User
	ok(t, db.Preload("Role").First(&got2, user.ID).Error, "再次查询失败")
	if got2.LastLoginAt == nil {
		t.Error("LastLoginAt 应已更新")
	}
	eq(t, UserStatusDisabled, got2.Status)
}

func TestLoginLogCRUD(t *testing.T) {
	db := newModelTestDB(t)
	log1 := LoginLog{
		UserID: 1, Username: "alice", IP: "192.168.0.1",
		UserAgent: "Mozilla/5.0", Status: LoginStatusSuccess, Message: "登录成功",
	}
	log2 := LoginLog{
		UserID: 1, Username: "alice", IP: "10.0.0.1",
		UserAgent: "curl/8.0", Status: LoginStatusFailed, Message: "密码错误",
	}
	ok(t, db.Create(&log1).Error, "创建日志1失败")
	ok(t, db.Create(&log2).Error, "创建日志2失败")

	var logs []LoginLog
	ok(t, db.Where("user_id = ?", 1).Order("id ASC").Find(&logs).Error, "查询日志失败")
	if len(logs) != 2 {
		t.Fatalf("日志数量不符: %d", len(logs))
	}
	eq(t, LoginStatusSuccess, logs[0].Status)
	eq(t, LoginStatusFailed, logs[1].Status)
}

func TestSoftDelete(t *testing.T) {
	db := newModelTestDB(t)

	t.Run("ImageSyncRecord 软删除", func(t *testing.T) {
		rec := ImageSyncRecord{OriginalImage: "nginx", SyncStatus: SyncStatusSuccess}
		ok(t, db.Create(&rec).Error, "创建失败")
		ok(t, db.Delete(&rec).Error, "删除失败")

		// 常规查询查不到
		var cnt int64
		db.Model(&ImageSyncRecord{}).Count(&cnt)
		eq(t, int64(0), cnt)

		// Unscoped 可查到
		var raw ImageSyncRecord
		ok(t, db.Unscoped().First(&raw, rec.ID).Error, "Unscoped 查询失败")
		if raw.DeletedAt.Valid != true {
			t.Error("DeletedAt 应有效")
		}
	})

	t.Run("SyncTask 软删除", func(t *testing.T) {
		task := SyncTask{TaskID: "soft-del"}
		ok(t, db.Create(&task).Error, "创建失败")
		ok(t, db.Delete(&task).Error, "删除失败")
		bad(t, db.Where("task_id = ?", "soft-del").First(&SyncTask{}).Error, "软删除后应查不到")
	})

	t.Run("User 软删除", func(t *testing.T) {
		role := Role{Code: "r1", Name: "r"}
		ok(t, db.Create(&role).Error, "创建角色失败")
		user := User{Username: "softuser", PasswordHash: "h", RoleID: role.ID}
		ok(t, db.Create(&user).Error, "创建用户失败")
		ok(t, db.Delete(&user).Error, "删除失败")
		var cnt int64
		db.Model(&User{}).Count(&cnt)
		eq(t, int64(0), cnt)
	})

	t.Run("SystemConfig 软删除后常规查询为空", func(t *testing.T) {
		ok(t, db.Create(&SystemConfig{ConfigKey: "sd-key"}).Error, "创建失败")
		var cfg SystemConfig
		ok(t, db.Where("config_key = ?", "sd-key").First(&cfg).Error, "查询失败")
		ok(t, db.Delete(&cfg).Error, "删除失败")
		var cnt int64
		db.Model(&SystemConfig{}).Count(&cnt)
		eq(t, int64(0), cnt)
	})
}

func TestAcrRegistryCRUD(t *testing.T) {
	db := newModelTestDB(t)

	reg := AcrRegistry{
		RegistryURL:   "registry.cn-hangzhou.aliyuncs.com",
		Namespace:     "team1",
		Alias:         "prod-acr",
		Username:      "user",
		Password:      "pass",
		AuthServer:    "auth",
		DockerService: "service",
		RegistryType:  RegistryTypeACR,
		IsDefault:     true,
	}
	ok(t, db.Create(&reg).Error, "创建失败")

	var got AcrRegistry
	ok(t, db.Where("alias = ?", "prod-acr").First(&got).Error, "查询失败")
	eq(t, RegistryTypeACR, got.RegistryType)
	isTrue(t, got.IsDefault, "IsDefault 应为 true")
	eq(t, "pass", got.Password)

	// 各类型都可入库
	for _, rt := range []string{RegistryTypeSWR, RegistryTypeCCR, RegistryTypeHarbor, RegistryTypeGeneric} {
		r := AcrRegistry{RegistryURL: "https://x", Namespace: "ns", Username: "u", Password: "p", RegistryType: rt}
		ok(t, db.Create(&r).Error, "类型 %s 入库失败", rt)
	}
}

func TestAcrRepositoryWithRegistry(t *testing.T) {
	db := newModelTestDB(t)

	reg := AcrRegistry{RegistryURL: "https://reg", Namespace: "ns", Username: "u", Password: "p"}
	ok(t, db.Create(&reg).Error, "创建仓库失败")

	repo := AcrRepository{AcrRegistryID: reg.ID, RepositoryName: "nginx"}
	ok(t, db.Create(&repo).Error, "创建镜像失败")

	var got AcrRepository
	ok(t, db.Preload("AcrRegistry").First(&got, repo.ID).Error, "查询失败")
	if got.AcrRegistry == nil {
		t.Fatal("AcrRegistry 预加载失败")
	}
	eq(t, reg.ID, got.AcrRegistry.ID)
	eq(t, "nginx", got.RepositoryName)

	// 软删除
	ok(t, db.Delete(&repo).Error, "删除失败")
	var cnt int64
	db.Model(&AcrRepository{}).Count(&cnt)
	eq(t, int64(0), cnt)
}

func TestBatchCreateRecords(t *testing.T) {
	db := newModelTestDB(t)
	// 批量插入保持顺序
	var recs []ImageSyncRecord
	for i := 0; i < 50; i++ {
		recs = append(recs, ImageSyncRecord{
			OriginalImage: "img" + string(rune('a'+i%26)) + ":t",
			InputOrder:    i,
			SyncStatus:    SyncStatusPending,
		})
	}
	ok(t, db.Create(&recs).Error, "批量创建失败")

	var out []ImageSyncRecord
	ok(t, db.Order("input_order ASC").Find(&out).Error, "查询失败")
	if len(out) != 50 {
		t.Fatalf("记录数量不符: %d", len(out))
	}
	for i, r := range out {
		eq(t, i, r.InputOrder)
	}
}
