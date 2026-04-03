package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"unicode"

	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
)

func validateStrongPassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("密码长度至少8位")
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}
	if !hasUpper {
		return fmt.Errorf("密码必须包含至少一个大写字母")
	}
	if !hasLower {
		return fmt.Errorf("密码必须包含至少一个小写字母")
	}
	if !hasDigit {
		return fmt.Errorf("密码必须包含至少一个数字")
	}
	if !hasSpecial {
		return fmt.Errorf("密码必须包含至少一个特殊字符")
	}
	return nil
}

// AuthHandler 认证相关 Handler
type AuthHandler struct {
	authService *services.AuthService
	userService *services.UserService
}

// NewAuthHandler 创建认证 Handler
func NewAuthHandler(authService *services.AuthService, userService *services.UserService) *AuthHandler {
	return &AuthHandler{authService: authService, userService: userService}
}

type loginRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	RememberMe bool   `json:"remember_me"`
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名和密码不能为空"})
		return
	}

	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")

	user, err := h.userService.GetUserByUsername(req.Username)
	if err != nil {
		h.userService.RecordLoginLog(0, req.Username, ip, ua, models.LoginStatusFailed, "用户不存在")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	if user.Status == models.UserStatusDisabled {
		h.userService.RecordLoginLog(user.ID, req.Username, ip, ua, models.LoginStatusFailed, "账号已被禁用")
		c.JSON(http.StatusForbidden, gin.H{"error": "账号已被禁用，请联系管理员"})
		return
	}

	if !h.authService.CheckPassword(user.PasswordHash, req.Password) {
		h.userService.RecordLoginLog(user.ID, req.Username, ip, ua, models.LoginStatusFailed, "密码错误")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	token, expiresAt, err := h.authService.GenerateToken(user.ID, user.Username, user.Role, req.RememberMe)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成Token失败"})
		return
	}

	h.userService.UpdateLastLogin(user.ID)
	h.userService.RecordLoginLog(user.ID, req.Username, ip, ua, models.LoginStatusSuccess, "登录成功")

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":          user.ID,
			"username":    user.Username,
			"role":        user.Role,
			"email":       user.Email,
			"permissions": models.GetRolePermissions(user.Role),
		},
		"expires_at": expiresAt,
	})
}

// GetCurrentUser 获取当前用户信息
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID, _ := c.Get("userID")
	user, err := h.userService.GetUserByID(userID.(uint))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            user.ID,
		"username":      user.Username,
		"role":          user.Role,
		"email":         user.Email,
		"status":        user.Status,
		"permissions":   models.GetRolePermissions(user.Role),
		"last_login_at": user.LastLoginAt,
		"created_at":    user.CreatedAt,
	})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword 修改密码
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入原密码和新密码"})
		return
	}

	if err := validateStrongPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("userID")
	if err := h.userService.ChangePassword(userID.(uint), req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功，请重新登录"})
}

// Logout 登出
func (h *AuthHandler) Logout(c *gin.Context) {
	username, _ := c.Get("username")
	userID, _ := c.Get("userID")
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")

	h.userService.RecordLoginLog(userID.(uint), username.(string), ip, ua, models.LoginStatusSuccess, "主动登出")
	c.JSON(http.StatusOK, gin.H{"message": "已登出"})
}

// GetLoginLogs 查看登录日志
func (h *AuthHandler) GetLoginLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	username := c.Query("username")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	logs, total, err := h.userService.GetLoginLogs(page, pageSize, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询登录日志失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"total": total, "data": logs})
}

// ListUsers 用户列表
func (h *AuthHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	users, total, err := h.userService.ListUsers(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"total": total, "data": users})
}

type createUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email"`
	Role     string `json:"role" binding:"required,oneof=admin operator user"`
}

// CreateUser 创建用户
func (h *AuthHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写完整的用户信息（用户名、密码、角色）"})
		return
	}

	if err := validateStrongPassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userService.CreateUser(req.Username, req.Password, req.Email, req.Role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "用户创建成功", "user": user})
}

type updateStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

// UpdateUserStatus 更新用户状态
func (h *AuthHandler) UpdateUserStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}

	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "状态值无效"})
		return
	}

	if err := h.userService.UpdateUserStatus(uint(id), req.Status); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "用户状态已更新"})
}

// DeleteUser 删除用户
func (h *AuthHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}

	currentUserID, _ := c.Get("userID")
	if currentUserID.(uint) == uint(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能删除自己的账号"})
		return
	}

	if err := h.userService.DeleteUser(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "用户已删除"})
}

type resetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

// ResetUserPassword 管理员重置用户密码
func (h *AuthHandler) ResetUserPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}

	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入新密码"})
		return
	}

	if err := validateStrongPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.userService.ResetPassword(uint(id), req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码已重置"})
}

// GetRoles 获取所有可用角色及其权限
func (h *AuthHandler) GetRoles(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"roles": models.GetAllRoles()})
}

type updateRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// UpdateUserRole 修改用户角色
func (h *AuthHandler) UpdateUserRole(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}

	var req updateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "角色值无效"})
		return
	}

	if !models.IsValidRole(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的角色类型"})
		return
	}

	currentUserID, _ := c.Get("userID")
	if currentUserID.(uint) == uint(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能修改自己的角色"})
		return
	}

	if err := h.userService.UpdateUserRole(uint(id), req.Role); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "用户角色已更新"})
}
