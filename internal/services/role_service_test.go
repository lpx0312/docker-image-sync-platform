package services

import (
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedRBAC 写入权限与角色种子（admin 拥有 users+roles，user 仅 sync）
func seedRBAC(t *testing.T, db *gorm.DB) *RoleService {
	t.Helper()
	perms := make([]models.Permission, 0)
	permByCode := map[string]*models.Permission{}
	for _, p := range models.DefaultPermissionSeeds {
		perm := p
		require.NoError(t, db.Create(&perm).Error)
		perms = append(perms, perm)
		permByCode[perm.Code] = &perm
	}
	for _, r := range models.DefaultRoleSeeds {
		role := models.Role{Code: r.Code, Name: r.Name, Description: r.Description, IsSystem: r.IsSystem}
		require.NoError(t, db.Create(&role).Error)
		rolePerms := make([]models.Permission, 0, len(r.Permissions))
		for _, code := range r.Permissions {
			rolePerms = append(rolePerms, *permByCode[code])
		}
		require.NoError(t, db.Model(&role).Association("Permissions").Append(&rolePerms))
	}
	return NewRoleService(db)
}

func TestListPermissions(t *testing.T) {
	db := newSvcTestDB(t)
	svc := seedRBAC(t, db)

	perms, err := svc.ListPermissions()
	require.NoError(t, err)
	require.Len(t, perms, len(models.DefaultPermissionSeeds))
	// sort_order 升序
	for i := 1; i < len(perms); i++ {
		assert.GreaterOrEqual(t, perms[i].SortOrder, perms[i-1].SortOrder)
	}

	// 错误分支
	svc2 := NewRoleService(svcClosedDB(t))
	_, err = svc2.ListPermissions()
	require.Error(t, err)
}

func TestListRolesAndOptions(t *testing.T) {
	db := newSvcTestDB(t)
	svc := seedRBAC(t, db)

	roles, err := svc.ListRoles()
	require.NoError(t, err)
	require.Len(t, roles, 3)
	// is_system DESC → 三个系统角色；字段含权限与用户数
	assert.Equal(t, "admin", roles[0]["code"])
	assert.NotEmpty(t, roles[0]["permissions"])
	assert.Equal(t, int64(0), roles[0]["user_count"])

	opts, err := svc.ListRoleOptions()
	require.NoError(t, err)
	require.Len(t, opts, 3)
	assert.Contains(t, opts[0], "id")
	assert.Contains(t, opts[0]["permissions"], models.PermUsers)

	// 错误分支
	svc2 := NewRoleService(svcClosedDB(t))
	_, err = svc2.ListRoles()
	require.Error(t, err)
	_, err = svc2.ListRoleOptions()
	require.Error(t, err)
}

func TestGetRoleByID(t *testing.T) {
	db := newSvcTestDB(t)
	svc := seedRBAC(t, db)

	var admin models.Role
	require.NoError(t, db.Where("code = ?", models.RoleAdmin).First(&admin).Error)

	item, err := svc.GetRoleByID(admin.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin", item["code"])
	assert.Equal(t, true, item["is_system"])

	_, err = svc.GetRoleByID(9999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "角色不存在")
}

func TestCreateRole(t *testing.T) {
	db := newSvcTestDB(t)
	svc := seedRBAC(t, db)

	t.Run("成功", func(t *testing.T) {
		role, err := svc.CreateRole(" 观察者 ", "Viewer_1", "只读", []string{"sync", "images"})
		require.NoError(t, err)
		assert.Equal(t, "viewer_1", role.Code, "code 应小写规范化")
		assert.Equal(t, "观察者", role.Name)
		assert.False(t, role.IsSystem)

		perms, err := svc.GetRolePermissions(role.ID)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{models.PermSync, models.PermImages}, perms)
	})

	t.Run("名称为空", func(t *testing.T) {
		_, err := svc.CreateRole("", "code1", "", []string{"sync"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "角色名称不能为空")
	})

	t.Run("非法code", func(t *testing.T) {
		// 注：code 会先做小写规范化，"UPPER"/"x"（单字母）均合法
		for _, code := range []string{"", "1abc", "has-dash", "way_too_long_code_name_exceeding_30_chars"} {
			_, err := svc.CreateRole("n", code, "", []string{"sync"})
			require.Error(t, err, "code %q 应不合法", code)
		}
	})

	t.Run("未选择权限", func(t *testing.T) {
		_, err := svc.CreateRole("n", "codex", "", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "至少选择一项权限")
	})

	t.Run("code重复", func(t *testing.T) {
		_, err := svc.CreateRole("n", "admin", "", []string{"sync"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "已存在")
	})

	t.Run("无效权限标识", func(t *testing.T) {
		_, err := svc.CreateRole("n", "codewithbad", "", []string{"sync", "no_such_perm"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "无效的权限标识")
	})
}

func TestUpdateRole(t *testing.T) {
	db := newSvcTestDB(t)
	svc := seedRBAC(t, db)

	var admin, userRole models.Role
	require.NoError(t, db.Where("code = ?", models.RoleAdmin).First(&admin).Error)
	require.NoError(t, db.Where("code = ?", models.RoleUser).First(&userRole).Error)

	t.Run("角色不存在", func(t *testing.T) {
		err := svc.UpdateRole(9999, "n", "d", []string{"sync"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "角色不存在")
	})

	t.Run("名称为空与无权限", func(t *testing.T) {
		require.Error(t, svc.UpdateRole(userRole.ID, "  ", "d", []string{"sync"}))
		require.Error(t, svc.UpdateRole(userRole.ID, "n", "d", nil))
	})

	t.Run("无效权限", func(t *testing.T) {
		err := svc.UpdateRole(userRole.ID, "n", "d", []string{"nope"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "无效的权限标识")
	})

	t.Run("admin角色必须保留用户与角色管理权限", func(t *testing.T) {
		err := svc.UpdateRole(admin.ID, "管理员", "d", []string{"sync", "images"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "必须保留用户管理和角色管理权限")
	})

	t.Run("剥夺最后一个admin-like角色权限被拦截", func(t *testing.T) {
		// user 角色当前仅 sync，先赋 users+roles 成为 admin-like，再尝试剥夺
		permUsers := findPermID(t, db, models.PermUsers)
		permRoles := findPermID(t, db, models.PermRoles)
		require.NoError(t, db.Exec(
			"INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?), (?, ?)",
			userRole.ID, permUsers, userRole.ID, permRoles).Error)

		err := svc.UpdateRole(userRole.ID, "普通用户", "d", []string{"sync"})
		// admin 角色仍具备 users+roles → otherAdminRoles>0 → 应允许更新成功
		require.NoError(t, err)

		// 再剥夺 admin 自身之外的最后一个：硬删 admin（Role 无软删字段）后仅剩 user 一个 admin-like
		require.NoError(t, db.Delete(&admin).Error)
		require.NoError(t, db.Exec(
			"INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?), (?, ?)",
			userRole.ID, permUsers, userRole.ID, permRoles).Error)
		err = svc.UpdateRole(userRole.ID, "普通用户", "d", []string{"sync"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "至少需要保留一个同时拥有用户管理和角色管理权限的角色")
	})

	t.Run("成功更新", func(t *testing.T) {
		// 前置子测试硬删了 admin（Role 无软删字段），先重建一个具备 users+roles 的角色以免触发保护
		restore, err := svc.CreateRole("管理员副本", "admin2", "", []string{"users", "roles"})
		require.NoError(t, err)
		defer func() { _ = svc.DeleteRole(restore.ID) }()

		err = svc.UpdateRole(userRole.ID, "普通用户2", "desc2", []string{"sync", "images"})
		require.NoError(t, err)

		var reloaded models.Role
		require.NoError(t, db.First(&reloaded, userRole.ID).Error)
		assert.Equal(t, "普通用户2", reloaded.Name)
		assert.Equal(t, "desc2", reloaded.Description)
		perms, err := svc.GetRolePermissions(userRole.ID)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{models.PermSync, models.PermImages}, perms)
	})

	t.Run("更新失败回滚", func(t *testing.T) {
		require.NoError(t, db.Exec(
			`CREATE TRIGGER trg_role_no_update BEFORE UPDATE ON roles BEGIN SELECT RAISE(ABORT, 'no role update'); END`).Error)
		// 保留 users+roles 使权限保留检查通过，落到 DB 更新时被触发器拦截
		err := svc.UpdateRole(userRole.ID, "x", "d", []string{"users", "roles"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "更新角色失败")
	})
}

func findPermID(t *testing.T, db *gorm.DB, code string) uint {
	t.Helper()
	var p models.Permission
	require.NoError(t, db.Where("code = ?", code).First(&p).Error)
	return p.ID
}

func TestDeleteRole(t *testing.T) {
	db := newSvcTestDB(t)
	svc := seedRBAC(t, db)

	var admin, userRole models.Role
	require.NoError(t, db.Where("code = ?", models.RoleAdmin).First(&admin).Error)
	require.NoError(t, db.Where("code = ?", models.RoleUser).First(&userRole).Error)

	t.Run("角色不存在", func(t *testing.T) {
		err := svc.DeleteRole(9999)
		require.Error(t, err)
	})

	t.Run("系统角色不可删除", func(t *testing.T) {
		err := svc.DeleteRole(admin.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "系统内置角色不可删除")
	})

	t.Run("角色下有用户", func(t *testing.T) {
		custom, err := svc.CreateRole("临时有用户", "tmp_with_user", "", []string{"sync"})
		require.NoError(t, err)
		require.NoError(t, db.Create(&models.User{
			Username: "u1", PasswordHash: "h", RoleID: custom.ID, Status: models.UserStatusActive,
		}).Error)
		err = svc.DeleteRole(custom.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "仍有 1 个用户")
		require.NoError(t, db.Where("username = ?", "u1").Delete(&models.User{}).Error)
		require.NoError(t, svc.DeleteRole(custom.ID))
	})

	t.Run("成功删除自定义角色", func(t *testing.T) {
		custom, err := svc.CreateRole("临时", "tmp_role", "", []string{"sync"})
		require.NoError(t, err)
		require.NoError(t, svc.DeleteRole(custom.ID))
		assert.False(t, svc.RoleExists(custom.ID))
	})

	t.Run("删除失败", func(t *testing.T) {
		custom, err := svc.CreateRole("临时2", "tmp_role2", "", []string{"sync"})
		require.NoError(t, err)
		require.NoError(t, db.Exec(
			`CREATE TRIGGER trg_role_no_delete BEFORE DELETE ON roles BEGIN SELECT RAISE(ABORT, 'no role delete'); END`).Error)
		err = svc.DeleteRole(custom.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "删除角色失败")
	})
}

func TestRoleExists(t *testing.T) {
	db := newSvcTestDB(t)
	svc := seedRBAC(t, db)
	var admin models.Role
	require.NoError(t, db.Where("code = ?", models.RoleAdmin).First(&admin).Error)

	assert.True(t, svc.RoleExists(admin.ID))
	assert.False(t, svc.RoleExists(4242))
}

func TestGetRolePermissionsAndCache(t *testing.T) {
	db := newSvcTestDB(t)
	svc := seedRBAC(t, db)

	var admin models.Role
	require.NoError(t, db.Where("code = ?", models.RoleAdmin).First(&admin).Error)

	perms, err := svc.GetRolePermissions(admin.ID)
	require.NoError(t, err)
	assert.Contains(t, perms, models.PermUsers)
	assert.Contains(t, perms, models.PermRoles)

	// 不存在的角色
	_, err = svc.GetRolePermissions(9999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "角色不存在")

	// 无权限角色默认 sync
	bare := models.Role{Code: "bare", Name: "裸角色"}
	require.NoError(t, db.Create(&bare).Error)
	perms, err = svc.GetRolePermissions(bare.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{models.PermSync}, perms)

	// 缓存：修改数据库后命中缓存，InvalidateCache 后重新加载
	permImages := findPermID(t, db, models.PermImages)
	require.NoError(t, db.Exec(
		"INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", bare.ID, permImages).Error)
	cached, err := svc.GetRolePermissions(bare.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{models.PermSync}, cached, "应命中缓存")

	svc.InvalidateCache(bare.ID)
	fresh, err := svc.GetRolePermissions(bare.ID)
	require.NoError(t, err)
	// 重新加载后角色已拥有 images 权限，非空权限列表不再注入默认 sync
	assert.Equal(t, []string{models.PermImages}, fresh)
}

func TestHasPermission(t *testing.T) {
	db := newSvcTestDB(t)
	svc := seedRBAC(t, db)

	var userRole models.Role
	require.NoError(t, db.Where("code = ?", models.RoleUser).First(&userRole).Error)

	assert.True(t, svc.HasPermission(userRole.ID, models.PermSync))
	assert.False(t, svc.HasPermission(userRole.ID, models.PermUsers))
	assert.False(t, svc.HasPermission(9999, models.PermSync))
}

func TestContainsCodeAndDedupeStrings(t *testing.T) {
	assert.True(t, containsCode([]string{"a", "b"}, "b"))
	assert.False(t, containsCode([]string{"a"}, "b"))
	assert.False(t, containsCode(nil, "b"))

	assert.Equal(t, []string{"a", "b"}, dedupeStrings([]string{"a", " a ", "b", "", "b"}))
	assert.Empty(t, dedupeStrings(nil))
}
