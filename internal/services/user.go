package services

import (
	"fmt"
	"time"

	"docker-image-sync-platform/internal/models"

	"gorm.io/gorm"
)

// UserService 用户服务
type UserService struct {
	db          *gorm.DB
	authService *AuthService
	roleService *RoleService
}

// NewUserService 创建用户服务
func NewUserService(db *gorm.DB, authService *AuthService, roleService *RoleService) *UserService {
	return &UserService{db: db, authService: authService, roleService: roleService}
}

// CreateUser 创建用户
func (s *UserService) CreateUser(username, password, email string, roleID uint) (*models.User, error) {
	var count int64
	s.db.Model(&models.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("用户名 %s 已存在", username)
	}

	if !s.roleService.RoleExists(roleID) {
		return nil, fmt.Errorf("指定的角色不存在")
	}

	hash, err := s.authService.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username:     username,
		PasswordHash: hash,
		Email:        email,
		RoleID:       roleID,
		Status:       models.UserStatusActive,
	}

	if err := s.db.Create(user).Error; err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	return s.GetUserByID(user.ID)
}

// GetUserByUsername 根据用户名查询
func (s *UserService) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	if err := s.db.Preload("Role").Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByID 根据 ID 查询
func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	if err := s.db.Preload("Role").First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// ListUsers 分页查询用户列表
func (s *UserService) ListUsers(page, pageSize int) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	s.db.Model(&models.User{}).Count(&total)

	offset := (page - 1) * pageSize
	if err := s.db.Preload("Role").Order("id ASC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("查询用户列表失败: %w", err)
	}

	return users, total, nil
}

// UpdateUserStatus 更新用户状态
func (s *UserService) UpdateUserStatus(id uint, status string) error {
	if status == models.UserStatusDisabled {
		if err := s.ensureNotLastAdmin(id); err != nil {
			return err
		}
	}

	result := s.db.Model(&models.User{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return fmt.Errorf("更新用户状态失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

// ChangePassword 修改密码
func (s *UserService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	user, err := s.GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}

	if !s.authService.CheckPassword(user.PasswordHash, oldPassword) {
		return fmt.Errorf("原密码错误")
	}

	hash, err := s.authService.HashPassword(newPassword)
	if err != nil {
		return err
	}

	return s.db.Model(&models.User{}).Where("id = ?", userID).Update("password_hash", hash).Error
}

// ResetPassword 管理员重置密码
func (s *UserService) ResetPassword(userID uint, newPassword string) error {
	hash, err := s.authService.HashPassword(newPassword)
	if err != nil {
		return err
	}

	result := s.db.Model(&models.User{}).Where("id = ?", userID).Update("password_hash", hash)
	if result.Error != nil {
		return fmt.Errorf("重置密码失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

// DeleteUser 删除用户（软删除）
func (s *UserService) DeleteUser(id uint) error {
	if err := s.ensureNotLastAdmin(id); err != nil {
		return err
	}

	result := s.db.Delete(&models.User{}, id)
	if result.Error != nil {
		return fmt.Errorf("删除用户失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

// UpdateUserRole 修改用户角色
func (s *UserService) UpdateUserRole(id uint, roleID uint) error {
	if !s.roleService.RoleExists(roleID) {
		return fmt.Errorf("指定的角色不存在")
	}

	if err := s.ensureNotLastAdmin(id); err != nil {
		newRolePerms, permErr := s.roleService.GetRolePermissions(roleID)
		if permErr != nil {
			return permErr
		}
		hasUsers := false
		hasRoles := false
		for _, p := range newRolePerms {
			if p == models.PermUsers {
				hasUsers = true
			}
			if p == models.PermRoles {
				hasRoles = true
			}
		}
		if !hasUsers || !hasRoles {
			return err
		}
	}

	result := s.db.Model(&models.User{}).Where("id = ?", id).Update("role_id", roleID)
	if result.Error != nil {
		return fmt.Errorf("更新用户角色失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

// BuildUserResponse 构建用户 API 响应
func (s *UserService) BuildUserResponse(user *models.User) (map[string]interface{}, error) {
	perms, err := s.roleService.GetRolePermissions(user.RoleID)
	if err != nil {
		return nil, err
	}

	resp := map[string]interface{}{
		"id":            user.ID,
		"username":      user.Username,
		"email":         user.Email,
		"role_id":       user.RoleID,
		"status":        user.Status,
		"permissions":   perms,
		"last_login_at": user.LastLoginAt,
		"created_at":    user.CreatedAt,
	}
	if user.Role != nil {
		resp["role_code"] = user.Role.Code
		resp["role_name"] = user.Role.Name
	}
	return resp, nil
}

// UpdateLastLogin 更新最后登录时间
func (s *UserService) UpdateLastLogin(userID uint) {
	now := time.Now()
	s.db.Model(&models.User{}).Where("id = ?", userID).Update("last_login_at", &now)
}

// RecordLoginLog 记录登录日志
func (s *UserService) RecordLoginLog(userID uint, username, ip, userAgent, status, message string) {
	log := &models.LoginLog{
		UserID:    userID,
		Username:  username,
		IP:        ip,
		UserAgent: userAgent,
		Status:    status,
		Message:   message,
	}
	s.db.Create(log)
}

// GetLoginLogs 查询登录日志
func (s *UserService) GetLoginLogs(page, pageSize int, username string) ([]models.LoginLog, int64, error) {
	var logs []models.LoginLog
	var total int64

	query := s.db.Model(&models.LoginLog{})
	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("查询登录日志失败: %w", err)
	}

	return logs, total, nil
}

func (s *UserService) ensureNotLastAdmin(userID uint) error {
	user, err := s.GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}

	perms, err := s.roleService.GetRolePermissions(user.RoleID)
	if err != nil {
		return err
	}
	if !containsPermission(perms, models.PermUsers) || !containsPermission(perms, models.PermRoles) {
		return nil
	}

	var adminLikeUsers int64
	s.db.Model(&models.User{}).
		Joins("JOIN roles ON roles.id = users.role_id").
		Joins("JOIN role_permissions rp1 ON rp1.role_id = roles.id").
		Joins("JOIN permissions p1 ON p1.id = rp1.permission_id AND p1.code = ?", models.PermUsers).
		Joins("JOIN role_permissions rp2 ON rp2.role_id = roles.id").
		Joins("JOIN permissions p2 ON p2.id = rp2.permission_id AND p2.code = ?", models.PermRoles).
		Where("users.status = ? AND users.deleted_at IS NULL", models.UserStatusActive).
		Where("users.id <> ?", userID).
		Count(&adminLikeUsers)
	if adminLikeUsers == 0 {
		return fmt.Errorf("不能移除系统中最后一个拥有完整管理权限的活跃用户")
	}
	return nil
}

func containsPermission(perms []string, target string) bool {
	for _, p := range perms {
		if p == target {
			return true
		}
	}
	return false
}
