package handlers

import (
	"net/http"
	"strconv"

	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
)

// RoleHandler 角色管理 Handler
type RoleHandler struct {
	roleService *services.RoleService
}

// NewRoleHandler 创建角色 Handler
func NewRoleHandler(roleService *services.RoleService) *RoleHandler {
	return &RoleHandler{roleService: roleService}
}

// ListPermissions 获取全部权限
func (h *RoleHandler) ListPermissions(c *gin.Context) {
	perms, err := h.roleService.ListPermissions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": perms})
}

// ListRoles 获取角色列表
func (h *RoleHandler) ListRoles(c *gin.Context) {
	roles, err := h.roleService.ListRoles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": roles})
}

// ListRoleOptions 获取角色下拉选项
func (h *RoleHandler) ListRoleOptions(c *gin.Context) {
	roles, err := h.roleService.ListRoleOptions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": roles})
}

// GetRole 获取角色详情
func (h *RoleHandler) GetRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色ID"})
		return
	}

	role, err := h.roleService.GetRoleByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, role)
}

type createRoleRequest struct {
	Name        string   `json:"name" binding:"required"`
	Code        string   `json:"code" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions" binding:"required,min=1"`
}

// CreateRole 创建角色
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var req createRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写完整的角色信息"})
		return
	}

	role, err := h.roleService.CreateRole(req.Name, req.Code, req.Description, req.Permissions)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.roleService.GetRoleByID(role.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "角色创建成功", "role": role})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色创建成功", "role": item})
}

type updateRoleEntityRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions" binding:"required,min=1"`
}

// UpdateRole 更新角色
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色ID"})
		return
	}

	var req updateRoleEntityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写完整的角色信息"})
		return
	}

	if err := h.roleService.UpdateRole(uint(id), req.Name, req.Description, req.Permissions); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.roleService.GetRoleByID(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "角色已更新"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色已更新", "role": item})
}

// DeleteRole 删除角色
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色ID"})
		return
	}

	if err := h.roleService.DeleteRole(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色已删除"})
}
