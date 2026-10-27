package handlers

// auth_test.go 覆盖 auth.go：登录/锁定/登出/用户管理/登录日志

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hAdminPwd = "Admin123!@#"

func TestValidateStrongPassword(t *testing.T) {
	cases := []struct {
		pw   string
		want string
	}{
		{"Ab1!x", "密码长度至少8位"},
		{"abcdefg1!A", ""},   // OK
		{"abcdefg1!", "密码必须包含至少一个大写字母"},
		{"ABCDEFG1!", "密码必须包含至少一个小写字母"},
		{"Abcdefg!a", "密码必须包含至少一个数字"},
		{"Abcdefg1a", "密码必须包含至少一个特殊字符"},
	}
	for _, c := range cases {
		err := validateStrongPassword(c.pw)
		if c.want == "" {
			assert.NoError(t, err, "pw=%s", c.pw)
		} else {
			require.Error(t, err, "pw=%s", c.pw)
			assert.Contains(t, err.Error(), c.want, "pw=%s", c.pw)
		}
	}
}

// hAuthEnv 构造登录测试环境：DB + 完整 RBAC + admin 用户 + 路由
func hAuthEnv(t *testing.T) (admin *models.User, user2 *models.User, r http.Handler) {
	t.Helper()
	db := newTestDB(t)
	adminID, _, _ := hSeedFullRBAC(t, db)
	h := hNewAuthHandler(t, db)
	admin = hNewUser(t, db, "admin", hAdminPwd, adminID)
	user2 = hNewUser(t, db, "bob", "Bob1234!!", adminID+1)

	e := hNewEngine()
	auth := e.Group("/auth")
	{
		auth.POST("/login", h.Login)
	}
	// 需要身份的接口挂身份注入中间件
	me := e.Group("/me", hWithIdentity(admin.ID, admin.Username))
	{
		me.GET("", h.GetCurrentUser)
		me.PUT("/password", h.ChangePassword)
		me.POST("/logout", h.Logout)
	}
	logs := e.Group("/logs", hWithIdentity(admin.ID, admin.Username))
	{
		logs.GET("", h.GetLoginLogs)
	}
	users := e.Group("/users", hWithIdentity(admin.ID, admin.Username))
	{
		users.GET("", h.ListUsers)
		users.POST("", h.CreateUser)
		users.PUT("/:id/status", h.UpdateUserStatus)
		users.PUT("/:id/role", h.UpdateUserRole)
		users.PUT("/:id/password", h.ResetUserPassword)
		users.DELETE("/:id", h.DeleteUser)
	}
	return admin, user2, e
}

func hLoginBody(user, pwd string) string {
	return fmt.Sprintf(`{"username":%q,"password":%q,"remember_me":true}`, user, pwd)
}

func TestAuthLogin(t *testing.T) {
	admin, _, r := hAuthEnv(t)

	// 空请求体 / 畸形 JSON
	w := hPerform(t, r, "POST", "/auth/login", `{`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = hPerform(t, r, "POST", "/auth/login", `{}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 用户不存在
	w, _ = hPerformJSON(t, r, "POST", "/auth/login", hLoginBody("ghost", "what ever 1!A"), nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 正常登录成功
	w, body := hPerformJSON(t, r, "POST", "/auth/login", hLoginBody(admin.Username, hAdminPwd), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotEmpty(t, body["token"])
	userObj, ok := body["user"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, admin.Username, userObj["username"])
	assert.Contains(t, userObj, "permissions")
	assert.Contains(t, body, "expires_at")
}

func TestAuthLoginLockout429(t *testing.T) {
	admin, _, r := hAuthEnv(t)
	// 连续失败达到阈值后锁定（防爆破）
	for i := 0; i < loginMaxFailures; i++ {
		w, _ := hPerformJSON(t, r, "POST", "/auth/login", hLoginBody(admin.Username, "Wrong1!"+strings.Repeat("x", i)), nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
	w, body := hPerformJSON(t, r, "POST", "/auth/login", hLoginBody(admin.Username, hAdminPwd), nil)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, body["error"], "分钟")
}

func TestAuthLoginDisabledAndWrongPassword(t *testing.T) {
	admin, _, r := hAuthEnv(t)

	// 密码错误
	w, _ := hPerformJSON(t, r, "POST", "/auth/login", hLoginBody(admin.Username, "Wrong123!@"), nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 账号禁用
	require.NoError(t, hDB().Model(&models.User{}).Where("id = ?", admin.ID).Update("status", models.UserStatusDisabled).Error)
	w, body := hPerformJSON(t, r, "POST", "/auth/login", hLoginBody(admin.Username, hAdminPwd), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, body["error"], "禁用")
}

func TestAuthGetCurrentUser(t *testing.T) {
	admin, _, r := hAuthEnv(t)

	w, body := hPerformJSON(t, r, "GET", "/me", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, admin.Username, body["username"])

	// 用户不存在（ID 直接换成不存在的）
	e2 := hNewEngine()
	h2 := hNewAuthHandler(t, hDB())
	e2.GET("/me", hWithIdentity(99999, "nobody"), h2.GetCurrentUser)
	w, _ = hPerformJSON(t, e2, "GET", "/me", "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 角色行被删后 GetUserByID 即失败 → 404。
	// handler 的 500 分支（GetUserByID 成功而 BuildUserResponse 失败）
	// 在当前实现下无法从外部构造，属于事实上的不可达分支。
	require.NoError(t, hDB().Exec("DELETE FROM roles WHERE id = ?", admin.RoleID).Error)
	w, _ = hPerformJSON(t, e2, "GET", "/me", "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAuthChangePassword(t *testing.T) {
	admin, _, r := hAuthEnv(t)

	// 缺字段
	w, _ := hPerformJSON(t, r, "PUT", "/me/password", `{"old_password":"x"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 弱密码
	w, body := hPerformJSON(t, r, "PUT", "/me/password", `{"old_password":"o","new_password":"weak"}`, nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, body["error"], "密码")
	// 原密码错误
	w, _ = hPerformJSON(t, r, "PUT", "/me/password", `{"old_password":"Wrong0ld!","new_password":"NewPass99!"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 成功
	w, _ = hPerformJSON(t, r, "PUT", "/me/password", fmt.Sprintf(`{"old_password":%q,"new_password":"NewPass99!"}`, hAdminPwd), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	// 新密码能登录
	w, _ = hPerformJSON(t, r, "POST", "/auth/login", hLoginBody(admin.Username, "NewPass99!"), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthLogout(t *testing.T) {
	admin, _, r := hAuthEnv(t)
	// 先登录拿 token
	w, body := hPerformJSON(t, r, "POST", "/auth/login", hLoginBody(admin.Username, hAdminPwd), nil)
	require.Equal(t, http.StatusOK, w.Code)
	token := body["token"].(string)

	// 带 Bearer 登出（吊销当前 token）
	w, _ = hPerformJSON(t, r, "POST", "/me/logout", `{}`, map[string]string{"Authorization": "Bearer " + token})
	require.Equal(t, http.StatusOK, w.Code)

	// 无 Authorization 头也能登出
	w, _ = hPerformJSON(t, r, "POST", "/me/logout", `{}`, nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthGetLoginLogs(t *testing.T) {
	_, _, r := hAuthEnv(t)
	// 先制造几条日志
	_, _ = hPerformJSON(t, r, "POST", "/auth/login", hLoginBody("who", "Wrong123!"), nil)

	w, body := hPerformJSON(t, r, "GET", "/logs?page=1&page_size=10&username=who", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, float64(1), body["total"])

	// 非法分页参数回退默认值
	w, body = hPerformJSON(t, r, "GET", "/logs?page=-3&page_size=999", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := body["data"].([]interface{})
	assert.Len(t, data, 1)

	// 查询失败 → 500
	hDropTable(t, hDB(), &models.LoginLog{}, "login_logs")
	w, _ = hPerformJSON(t, r, "GET", "/logs", "", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAuthListUsers(t *testing.T) {
	_, _, r := hAuthEnv(t)

	w, body := hPerformJSON(t, r, "GET", "/users?page=1&page_size=1", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, float64(2), body["total"])
	assert.Len(t, body["data"], 1)

	// 分页参数非法回退
	w, body = hPerformJSON(t, r, "GET", "/users?page=0&page_size=1000", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, body["data"], 2)

	// 查询失败 → 500
	hDropTable(t, hDB(), &models.User{}, "users")
	w, _ = hPerformJSON(t, r, "GET", "/users", "", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAuthCreateUser(t *testing.T) {
	_, _, r := hAuthEnv(t)
	db := hDB()
	var adminRole models.Role
	require.NoError(t, db.Where("code = ?", models.RoleAdmin).First(&adminRole).Error)

	// 请求体缺失
	w, _ := hPerformJSON(t, r, "POST", "/users", `{"username":"x"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 弱密码
	w, body := hPerformJSON(t, r, "POST", "/users", `{"username":"u1","password":"weak","role_id":1}`, nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, body["error"], "密码")
	// 角色不存在
	w, _ = hPerformJSON(t, r, "POST", "/users", fmt.Sprintf(`{"username":"u1","password":%q,"role_id":9999}`, hAdminPwd), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 成功
	w, body = hPerformJSON(t, r, "POST", "/users", fmt.Sprintf(`{"username":"u1","password":%q,"email":"u1@x.c","role_id":%d}`, "User1234!", adminRole.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	// 重复用户名
	w, _ = hPerformJSON(t, r, "POST", "/users", fmt.Sprintf(`{"username":"u1","password":%q,"role_id":%d}`, "User1234!", adminRole.ID), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// BuildUserResponse 失败 → 返回降级响应（仍 200，附原始用户）
	hDropTable(t, db, &models.Role{}, "role_permissions")
	w, body = hPerformJSON(t, r, "POST", "/users", fmt.Sprintf(`{"username":"u2","password":%q,"role_id":%d}`, "User1234!", adminRole.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, hasUserKey := body["user"]
	assert.True(t, hasUserKey)
}

func TestAuthUpdateUserStatus(t *testing.T) {
	admin, bob, r := hAuthEnv(t)

	// 非法 ID
	w, _ := hPerformJSON(t, r, "PUT", "/users/abc/status", `{"status":"active"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 非法状态值
	w, _ = hPerformJSON(t, r, "PUT", fmt.Sprintf("/users/%d/status", bob.ID), `{"status":"hacked"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 用户不存在
	w, _ = hPerformJSON(t, r, "PUT", "/users/424242/status", `{"status":"disabled"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 成功禁用 bob
	w, _ = hPerformJSON(t, r, "PUT", fmt.Sprintf("/users/%d/status", bob.ID), `{"status":"disabled"}`, nil)
	require.Equal(t, http.StatusOK, w.Code)
	// 最后一个拥有完整管理权限的用户不能被禁用
	w, body := hPerformJSON(t, r, "PUT", fmt.Sprintf("/users/%d/status", admin.ID), `{"status":"disabled"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, body["error"], "最后一个")
}

func TestAuthDeleteUser(t *testing.T) {
	admin, bob, r := hAuthEnv(t)

	w, _ := hPerformJSON(t, r, "DELETE", "/users/xyz", "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 不能删自己
	w, _ = hPerformJSON(t, r, "DELETE", fmt.Sprintf("/users/%d", admin.ID), "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 成功
	w, _ = hPerformJSON(t, r, "DELETE", fmt.Sprintf("/users/%d", bob.ID), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	// 再删 → 不存在
	w, _ = hPerformJSON(t, r, "DELETE", fmt.Sprintf("/users/%d", bob.ID), "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthResetUserPassword(t *testing.T) {
	_, bob, r := hAuthEnv(t)

	w, _ := hPerformJSON(t, r, "PUT", "/users/bad/password", `{"new_password":"x"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = hPerformJSON(t, r, "PUT", fmt.Sprintf("/users/%d/password", bob.ID), `{"new_password":"short"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = hPerformJSON(t, r, "PUT", "/users/987654/password", fmt.Sprintf(`{"new_password":%q}`, "Reset1234!"), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = hPerformJSON(t, r, "PUT", fmt.Sprintf("/users/%d/password", bob.ID), fmt.Sprintf(`{"new_password":%q}`, "Reset1234!"), nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestAuthUpdateUserRole(t *testing.T) {
	admin, bob, r := hAuthEnv(t)
	db := hDB()
	var opRole models.Role
	require.NoError(t, db.Where("code = ?", models.RoleOperator).First(&opRole).Error)

	w, _ := hPerformJSON(t, r, "PUT", "/users/bad/role", `{"role_id":1}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = hPerformJSON(t, r, "PUT", fmt.Sprintf("/users/%d/role", bob.ID), `{}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 不能改自己
	w, _ = hPerformJSON(t, r, "PUT", fmt.Sprintf("/users/%d/role", admin.ID), fmt.Sprintf(`{"role_id":%d}`, opRole.ID), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 角色不存在
	w, _ = hPerformJSON(t, r, "PUT", fmt.Sprintf("/users/%d/role", bob.ID), `{"role_id":424242}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 成功
	w, _ = hPerformJSON(t, r, "PUT", fmt.Sprintf("/users/%d/role", bob.ID), fmt.Sprintf(`{"role_id":%d}`, opRole.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
}
