package services

import (
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newTestUserService 构造带 RBAC 种子的用户服务
func newTestUserService(t *testing.T) (*UserService, *gorm.DB, *RoleService) {
	t.Helper()
	db := newSvcTestDB(t)
	roleSvc := seedRBAC(t, db)
	return NewUserService(db, NewAuthService(), roleSvc), db, roleSvc
}

func adminRoleID(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	var role models.Role
	require.NoError(t, db.Where("code = ?", models.RoleAdmin).First(&role).Error)
	return role.ID
}

func userRoleID(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	var role models.Role
	require.NoError(t, db.Where("code = ?", models.RoleUser).First(&role).Error)
	return role.ID
}

func TestCreateUser(t *testing.T) {
	svc, db, _ := newTestUserService(t)

	t.Run("成功", func(t *testing.T) {
		u, err := svc.CreateUser("alice", "pw123", "a@x.com", userRoleID(t, db))
		require.NoError(t, err)
		assert.NotZero(t, u.ID)
		assert.Equal(t, "alice", u.Username)
		assert.Equal(t, models.UserStatusActive, u.Status)
		assert.True(t, svc.CheckPasswordHandler(u.PasswordHash, "pw123"))
		require.NotNil(t, u.Role, "返回应预加载角色")
		assert.Equal(t, models.RoleUser, u.Role.Code)
	})

	t.Run("用户名重复", func(t *testing.T) {
		_, err := svc.CreateUser("alice", "pw", "", userRoleID(t, db))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "已存在")
	})

	t.Run("角色不存在", func(t *testing.T) {
		_, err := svc.CreateUser("bob", "pw", "", 9999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "指定的角色不存在")
	})

	t.Run("创建失败", func(t *testing.T) {
		require.NoError(t, db.Exec(
			`CREATE TRIGGER trg_user_no_insert BEFORE INSERT ON users BEGIN SELECT RAISE(ABORT, 'no user insert'); END`).Error)
		_, err := svc.CreateUser("carol", "pw", "", userRoleID(t, db))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "创建用户失败")
	})
}

// CheckPasswordHandler 供测试校验密码哈希
func (s *UserService) CheckPasswordHandler(hash, password string) bool {
	return s.authService.CheckPassword(hash, password)
}

func TestGetUsers(t *testing.T) {
	svc, db, _ := newTestUserService(t)
	_, err := svc.CreateUser("dave", "pw", "", userRoleID(t, db))
	require.NoError(t, err)
	_, err = svc.CreateUser("erin", "pw", "", adminRoleID(t, db))
	require.NoError(t, err)

	t.Run("按用户名", func(t *testing.T) {
		u, err := svc.GetUserByUsername("erin")
		require.NoError(t, err)
		require.NotNil(t, u.Role)
		assert.Equal(t, models.RoleAdmin, u.Role.Code)

		_, err = svc.GetUserByUsername("nobody")
		require.Error(t, err)
	})

	t.Run("按ID", func(t *testing.T) {
		u, err := svc.GetUserByUsername("dave")
		require.NoError(t, err)
		u2, err := svc.GetUserByID(u.ID)
		require.NoError(t, err)
		assert.Equal(t, "dave", u2.Username)

		_, err = svc.GetUserByID(424242)
		require.Error(t, err)
	})
}

func TestListUsers(t *testing.T) {
	svc, db, _ := newTestUserService(t)
	for _, name := range []string{"u1", "u2", "u3"} {
		_, err := svc.CreateUser(name, "pw", "", userRoleID(t, db))
		require.NoError(t, err)
	}

	users, total, err := svc.ListUsers(1, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, users, 2)

	users2, _, err := svc.ListUsers(2, 2)
	require.NoError(t, err)
	assert.Len(t, users2, 1)
	assert.Equal(t, "u3", users2[0].Username)
}

func TestUpdateUserStatus(t *testing.T) {
	t.Run("禁用最后一个管理员被拦截", func(t *testing.T) {
		svc, db, _ := newTestUserService(t)
		admin, err := svc.CreateUser("root", "pw", "", adminRoleID(t, db))
		require.NoError(t, err)
		err = svc.UpdateUserStatus(admin.ID, models.UserStatusDisabled)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "最后一个")
	})

	t.Run("存在第二个管理员时可禁用", func(t *testing.T) {
		svc, db, _ := newTestUserService(t)
		a1, _ := svc.CreateUser("root", "pw", "", adminRoleID(t, db))
		_, err := svc.CreateUser("root2", "pw", "", adminRoleID(t, db))
		require.NoError(t, err)
		require.NoError(t, svc.UpdateUserStatus(a1.ID, models.UserStatusDisabled))
	})

	t.Run("普通用户可禁用", func(t *testing.T) {
		svc, db, _ := newTestUserService(t)
		_, _ = svc.CreateUser("root", "pw", "", adminRoleID(t, db))
		u, err := svc.CreateUser("plain", "pw", "", userRoleID(t, db))
		require.NoError(t, err)
		require.NoError(t, svc.UpdateUserStatus(u.ID, models.UserStatusDisabled))
	})

	t.Run("用户不存在", func(t *testing.T) {
		svc, _, _ := newTestUserService(t)
		err := svc.UpdateUserStatus(9999, models.UserStatusActive)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "用户不存在")
	})
}

func TestChangePassword(t *testing.T) {
	svc, db, _ := newTestUserService(t)
	u, err := svc.CreateUser("peter", "old-pw", "", userRoleID(t, db))
	require.NoError(t, err)

	t.Run("原密码错误", func(t *testing.T) {
		err := svc.ChangePassword(u.ID, "wrong", "new-pw")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "原密码错误")
	})

	t.Run("用户不存在", func(t *testing.T) {
		err := svc.ChangePassword(9999, "old-pw", "new-pw")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "用户不存在")
	})

	t.Run("成功", func(t *testing.T) {
		require.NoError(t, svc.ChangePassword(u.ID, "old-pw", "new-pw"))
		reloaded, err := svc.GetUserByID(u.ID)
		require.NoError(t, err)
		assert.True(t, svc.CheckPasswordHandler(reloaded.PasswordHash, "new-pw"))
	})
}

func TestResetPassword(t *testing.T) {
	svc, db, _ := newTestUserService(t)
	u, err := svc.CreateUser("rita", "pw", "", userRoleID(t, db))
	require.NoError(t, err)

	require.NoError(t, svc.ResetPassword(u.ID, "reset-pw"))
	reloaded, err := svc.GetUserByID(u.ID)
	require.NoError(t, err)
	assert.True(t, svc.CheckPasswordHandler(reloaded.PasswordHash, "reset-pw"))

	err = svc.ResetPassword(9999, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "用户不存在")
}

func TestDeleteUser(t *testing.T) {
	t.Run("最后一个管理员", func(t *testing.T) {
		svc, db, _ := newTestUserService(t)
		admin, _ := svc.CreateUser("root", "pw", "", adminRoleID(t, db))
		err := svc.DeleteUser(admin.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "最后一个")
	})

	t.Run("成功软删除与不存在", func(t *testing.T) {
		svc, db, _ := newTestUserService(t)
		_, _ = svc.CreateUser("root", "pw", "", adminRoleID(t, db))
		u, err := svc.CreateUser("temp", "pw", "", userRoleID(t, db))
		require.NoError(t, err)
		require.NoError(t, svc.DeleteUser(u.ID))

		var count int64
		db.Model(&models.User{}).Where("id = ?", u.ID).Count(&count)
		assert.Zero(t, count, "软删除后常规查询不可见")

		err = svc.DeleteUser(u.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "用户不存在")
	})
}

func TestUpdateUserRole(t *testing.T) {
	t.Run("角色不存在", func(t *testing.T) {
		svc, db, _ := newTestUserService(t)
		u, _ := svc.CreateUser("root", "pw", "", adminRoleID(t, db))
		err := svc.UpdateUserRole(u.ID, 9999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "指定的角色不存在")
	})

	t.Run("降级最后一个管理员被拦截", func(t *testing.T) {
		svc, db, _ := newTestUserService(t)
		admin, _ := svc.CreateUser("root", "pw", "", adminRoleID(t, db))
		// 目标角色无 users/roles 权限
		err := svc.UpdateUserRole(admin.ID, userRoleID(t, db))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "最后一个")
	})

	t.Run("目标角色具备管理权限时允许降级", func(t *testing.T) {
		svc, db, roleSvc := newTestUserService(t)
		admin, _ := svc.CreateUser("root", "pw", "", adminRoleID(t, db))
		custom, err := roleSvc.CreateRole("副管理员", "deputy", "", []string{"users", "roles"})
		require.NoError(t, err)
		require.NoError(t, svc.UpdateUserRole(admin.ID, custom.ID))
	})

	t.Run("普通用户换角色成功", func(t *testing.T) {
		svc, db, _ := newTestUserService(t)
		_, _ = svc.CreateUser("root", "pw", "", adminRoleID(t, db))
		u, _ := svc.CreateUser("plain", "pw", "", userRoleID(t, db))
		require.NoError(t, svc.UpdateUserRole(u.ID, adminRoleID(t, db)))
		reloaded, err := svc.GetUserByID(u.ID)
		require.NoError(t, err)
		assert.Equal(t, adminRoleID(t, db), reloaded.RoleID)
	})

	t.Run("用户不存在", func(t *testing.T) {
		svc, db, _ := newTestUserService(t)
		err := svc.UpdateUserRole(9999, userRoleID(t, db))
		require.Error(t, err)
	})
}

func TestBuildUserResponse(t *testing.T) {
	svc, db, _ := newTestUserService(t)
	u, err := svc.CreateUser("oscar", "pw", "o@x.com", adminRoleID(t, db))
	require.NoError(t, err)

	resp, err := svc.BuildUserResponse(u)
	require.NoError(t, err)
	assert.Equal(t, "oscar", resp["username"])
	assert.Equal(t, adminRoleID(t, db), resp["role_id"])
	assert.Equal(t, models.RoleAdmin, resp["role_code"])
	assert.Equal(t, "管理员", resp["role_name"])
	assert.Contains(t, resp["permissions"], models.PermUsers)

	// 未预加载 Role → 无 role_code/role_name
	bare := *u
	bare.Role = nil
	resp2, err := svc.BuildUserResponse(&bare)
	require.NoError(t, err)
	assert.NotContains(t, resp2, "role_code")

	// 角色权限查询失败
	orphan := &models.User{Username: "orphan", RoleID: 9999}
	_, err = svc.BuildUserResponse(orphan)
	require.Error(t, err)
}

func TestUpdateLastLogin(t *testing.T) {
	svc, db, _ := newTestUserService(t)
	u, _ := svc.CreateUser("lora", "pw", "", userRoleID(t, db))

	svc.UpdateLastLogin(u.ID)
	reloaded, err := svc.GetUserByID(u.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.LastLoginAt)
}

func TestLoginLogs(t *testing.T) {
	svc, _, _ := newTestUserService(t)
	svc.RecordLoginLog(1, "admin", "1.2.3.4", "UA", models.LoginStatusSuccess, "ok")
	svc.RecordLoginLog(2, "admin2", "1.2.3.5", "UA", models.LoginStatusFailed, "bad pw")
	svc.RecordLoginLog(3, "plainuser", "1.2.3.6", "UA", models.LoginStatusSuccess, "ok")

	t.Run("全部", func(t *testing.T) {
		logs, total, err := svc.GetLoginLogs(1, 10, "")
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, logs, 3)
	})

	t.Run("按用户名过滤", func(t *testing.T) {
		logs, total, err := svc.GetLoginLogs(1, 10, "admin")
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		for _, l := range logs {
			assert.Contains(t, l.Username, "admin")
		}
	})

	t.Run("分页", func(t *testing.T) {
		logs, total, err := svc.GetLoginLogs(2, 2, "")
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, logs, 1)
	})
}

func TestContainsPermission(t *testing.T) {
	assert.True(t, containsPermission([]string{"a", "b"}, "a"))
	assert.False(t, containsPermission([]string{"a"}, "b"))
	assert.False(t, containsPermission(nil, "a"))
}
