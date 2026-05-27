package services

import (
	"errors"
	"fmt"

	"docker-image-sync-platform/internal/models"

	"gorm.io/gorm"
)

// AcrRegistryService ACR配置服务
type AcrRegistryService struct {
	db            *gorm.DB
	encryptionSvc *EncryptionService
}

// NewAcrRegistryService 创建ACR配置服务实例
func NewAcrRegistryService(db *gorm.DB, encryptionSvc *EncryptionService) *AcrRegistryService {
	return &AcrRegistryService{
		db:            db,
		encryptionSvc: encryptionSvc,
	}
}

// GetAll 获取所有ACR配置
func (s *AcrRegistryService) GetAll() ([]models.AcrRegistry, error) {
	var registries []models.AcrRegistry
	if err := s.db.Find(&registries).Error; err != nil {
		return nil, fmt.Errorf("获取ACR配置列表失败: %w", err)
	}
	return registries, nil
}

// GetByID 根据ID获取ACR配置
func (s *AcrRegistryService) GetByID(id uint) (*models.AcrRegistry, error) {
	var registry models.AcrRegistry
	if err := s.db.First(&registry, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("ACR配置不存在: %d", id)
		}
		return nil, fmt.Errorf("获取ACR配置失败: %w", err)
	}
	return &registry, nil
}

// GetDefault 获取默认ACR配置
func (s *AcrRegistryService) GetDefault() (*models.AcrRegistry, error) {
	var registry models.AcrRegistry
	if err := s.db.Where("is_default = ?", true).First(&registry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("未设置默认ACR")
		}
		return nil, fmt.Errorf("获取默认ACR失败: %w", err)
	}
	return &registry, nil
}

// Create 创建ACR配置
func (s *AcrRegistryService) Create(req *models.AcrRegistryRequest) (*models.AcrRegistry, error) {
	// 加密密码
	encryptedPassword, err := s.encryptionSvc.Encrypt(req.Password)
	if err != nil {
		return nil, fmt.Errorf("加密密码失败: %w", err)
	}

	registry := &models.AcrRegistry{
		RegistryURL: req.RegistryURL,
		Namespace:   req.Namespace,
		Username:    req.Username,
		Password:    encryptedPassword,
	}

	// 如果是第一个ACR，自动设为默认
	var count int64
	s.db.Model(&models.AcrRegistry{}).Count(&count)
	if count == 0 {
		registry.IsDefault = true
	}

	if err := s.db.Create(registry).Error; err != nil {
		return nil, fmt.Errorf("创建ACR配置失败: %w", err)
	}

	return registry, nil
}

// Update 更新ACR配置
func (s *AcrRegistryService) Update(id uint, req *models.AcrRegistryUpdateRequest) (*models.AcrRegistry, error) {
	registry, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if req.RegistryURL != "" {
		updates["registry_url"] = req.RegistryURL
	}
	if req.Namespace != "" {
		updates["namespace"] = req.Namespace
	}
	if req.Username != "" {
		updates["username"] = req.Username
	}
	if req.Password != "" && req.Password != "***" {
		encryptedPassword, err := s.encryptionSvc.Encrypt(req.Password)
		if err != nil {
			return nil, fmt.Errorf("加密密码失败: %w", err)
		}
		updates["password"] = encryptedPassword
	}

	if err := s.db.Model(registry).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("更新ACR配置失败: %w", err)
	}

	return s.GetByID(id)
}

// Delete 删除ACR配置
func (s *AcrRegistryService) Delete(id uint) error {
	registry, err := s.GetByID(id)
	if err != nil {
		return err
	}

	// 如果是默认ACR，检查是否有其他ACR
	if registry.IsDefault {
		var count int64
		s.db.Model(&models.AcrRegistry{}).Count(&count)
		if count <= 1 {
			return fmt.Errorf("不能删除唯一的ACR配置")
		}

		// 将另一个ACR设为默认
		var another models.AcrRegistry
		if err := s.db.Where("id != ?", id).First(&another).Error; err == nil {
			s.db.Model(&another).Update("is_default", true)
		}
	}

	if err := s.db.Delete(registry).Error; err != nil {
		return fmt.Errorf("删除ACR配置失败: %w", err)
	}

	return nil
}

// SetDefault 设置默认ACR
func (s *AcrRegistryService) SetDefault(id uint) error {
	// 验证ACR存在
	if _, err := s.GetByID(id); err != nil {
		return err
	}

	// 使用事务确保原子性
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 取消所有默认
		if err := tx.Model(&models.AcrRegistry{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return fmt.Errorf("取消默认ACR失败: %w", err)
		}

		// 设置新的默认
		if err := tx.Model(&models.AcrRegistry{}).Where("id = ?", id).Update("is_default", true).Error; err != nil {
			return fmt.Errorf("设置默认ACR失败: %w", err)
		}

		return nil
	})
}
