package handlers

// role_test.go 覆盖 role.go：权限/角色查询与 CRUD

import (
	"fmt"
	"net/http"
	"testing"

	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hRoleEnv 构造角色测试环境
func hRoleEnv(t *testing.T) (r http.Handler, roleSvc *services.RoleService) {
	t.Helper()
	db := newTestDB(t)
	hSeedFullRBAC(t, db)
	withGlobalDB(t, db)
	roleSvc = services.NewRoleService(db)
	h := NewRoleHandler(roleSvc)
	e := hNewEngine()
	e.GET("/permissions", h.ListPermissions)
	e.GET("/roles", h.ListRoles)
	e.GET("/role-options", h.ListRoleOptions)
	e.GET("/roles/:id", h.GetRole)
	e.POST("/roles", h.CreateRole)
	e.PUT("/roles/:id", h.UpdateRole)
	e.DELETE("/roles/:id", h.DeleteRole)
	return e, roleSvc
}

func hRoleIDByCode(t *testing.T, code string) uint {
	t.Helper()
	var role models.Role
	require.NoError(t, hDB().Where("code = ?", code).First(&role).Error)
	return role.ID
}

func TestRoleListPermissions(t *testing.T) {
	r, _ := hRoleEnv(t)
	w, body := hPerformJSON(t, r, "GET", "/permissions", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := body["data"].([]interface{})
	assert.Len(t, data, len(models.DefaultPermissionSeeds))

	// 查询失败 → 500
	hDropTable(t, hDB(), &models.Permission{}, "permissions")
	w, _ = hPerformJSON(t, r, "GET", "/permissions", "", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRoleListRolesAndOptions(t *testing.T) {
	r, _ := hRoleEnv(t)
	w, body := hPerformJSON(t, r, "GET", "/roles", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, body["data"], 3)

	w, body = hPerformJSON(t, r, "GET", "/role-options", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, body["data"], 3)

	hDropTable(t, hDB(), &models.Role{}, "roles")
	w, _ = hPerformJSON(t, r, "GET", "/roles", "", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	w, _ = hPerformJSON(t, r, "GET", "/role-options", "", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRoleGetRole(t *testing.T) {
	r, _ := hRoleEnv(t)
	adminID := hRoleIDByCode(t, models.RoleAdmin)

	w, _ := hPerformJSON(t, r, "GET", "/roles/not-id", "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = hPerformJSON(t, r, "GET", "/roles/424242", "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, body := hPerformJSON(t, r, "GET", fmt.Sprintf("/roles/%d", adminID), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, models.RoleAdmin, body["code"])
}

func TestRoleCreateRole(t *testing.T) {
	r, _ := hRoleEnv(t)

	// 缺字段
	w, _ := hPerformJSON(t, r, "POST", "/roles", `{"name":"x"}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 非法角色标识
	w, body := hPerformJSON(t, r, "POST", "/roles", `{"name":"角色1","code":"Bad Code","permissions":["sync"]}`, nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, body["error"], "角色标识")
	// 权限 code 不存在
	w, _ = hPerformJSON(t, r, "POST", "/roles", `{"name":"角色2","code":"viewer","permissions":["no_such_perm"]}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 成功
	w, body = hPerformJSON(t, r, "POST", "/roles", `{"name":"访客","code":"viewer","description":"只读","permissions":["sync","images"]}`, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	roleObj := body["role"].(map[string]interface{})
	assert.Equal(t, "viewer", roleObj["code"])

	// 重复 code
	w, _ = hPerformJSON(t, r, "POST", "/roles", `{"name":"访客2","code":"viewer","permissions":["sync"]}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 删除关联表后 CreateRole 的 Association 写入失败 → 400
	hDropTable(t, hDB(), &models.Role{}, "role_permissions")
	w, body = hPerformJSON(t, r, "POST", "/roles", `{"name":"访客3","code":"viewer3","permissions":["sync"]}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestRoleUpdateRole(t *testing.T) {
	r, _ := hRoleEnv(t)
	adminID := hRoleIDByCode(t, models.RoleAdmin)

	w, _ := hPerformJSON(t, r, "PUT", "/roles/bad", `{"name":"n","permissions":["sync"]}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = hPerformJSON(t, r, "PUT", fmt.Sprintf("/roles/%d", adminID), `{"name":""}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = hPerformJSON(t, r, "PUT", "/roles/424242", `{"name":"n","permissions":["sync"]}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 管理员角色必须保留 users+roles 权限
	w, body := hPerformJSON(t, r, "PUT", fmt.Sprintf("/roles/%d", adminID),
		`{"name":"管理员","permissions":["sync"]}`, nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, body["error"], "管理员角色")

	// 正常更新
	w, body = hPerformJSON(t, r, "PUT", fmt.Sprintf("/roles/%d", adminID),
		`{"name":"超级管理员","description":"desc","permissions":["sync","images","github","config","users","roles"]}`, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	roleObj := body["role"].(map[string]interface{})
	assert.Equal(t, "超级管理员", roleObj["name"])

	// 删除关联表后 UpdateRole 的 Association 写入失败 → 400
	hDropTable(t, hDB(), &models.Role{}, "role_permissions")
	w, body = hPerformJSON(t, r, "PUT", fmt.Sprintf("/roles/%d", adminID),
		`{"name":"再次改名","permissions":["sync","images","github","config","users","roles"]}`, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestRoleDeleteRole(t *testing.T) {
	r, _ := hRoleEnv(t)
	db := hDB()
	adminID := hRoleIDByCode(t, models.RoleAdmin)

	w, _ := hPerformJSON(t, r, "DELETE", "/roles/oops", "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = hPerformJSON(t, r, "DELETE", "/roles/424242", "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// 系统角色不可删
	w, _ = hPerformJSON(t, r, "DELETE", fmt.Sprintf("/roles/%d", adminID), "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 角色下有用户不可删
	viewer := &models.Role{Code: "viewer", Name: "访客"}
	require.NoError(t, db.Create(viewer).Error)
	u := &models.User{Username: "vinrole", PasswordHash: "x", RoleID: viewer.ID, Status: models.UserStatusActive}
	require.NoError(t, db.Create(u).Error)
	w, _ = hPerformJSON(t, r, "DELETE", fmt.Sprintf("/roles/%d", viewer.ID), "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	require.NoError(t, db.Delete(&models.User{}, u.ID).Error)
	w, _ = hPerformJSON(t, r, "DELETE", fmt.Sprintf("/roles/%d", viewer.ID), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
}
