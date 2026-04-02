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
}

// NewUserService 创建用户服务
func NewUserService(db *gorm.DB, authService *AuthService) *UserService {
	return &UserService{db: db, authService: authService}
}

// CreateUser 创建用户
func (s *UserService) CreateUser(username, password, email, role string) (*models.User, error) {
	var count int64
	s.db.Model(&models.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("用户名 %s 已存在", username)
	}

	hash, err := s.authService.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username:     username,
		PasswordHash: hash,
		Email:        email,
		Role:         role,
		Status:       models.UserStatusActive,
	}

	if err := s.db.Create(user).Error; err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	return user, nil
}

// GetUserByUsername 根据用户名查询
func (s *UserService) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByID 根据 ID 查询
func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
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
	if err := s.db.Order("id ASC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("查询用户列表失败: %w", err)
	}

	return users, total, nil
}

// UpdateUserStatus 更新用户状态
func (s *UserService) UpdateUserStatus(id uint, status string) error {
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
	result := s.db.Delete(&models.User{}, id)
	if result.Error != nil {
		return fmt.Errorf("删除用户失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
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
