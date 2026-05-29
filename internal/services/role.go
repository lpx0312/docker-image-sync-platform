package services

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"docker-image-sync-platform/internal/models"

	"gorm.io/gorm"
)

var roleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,29}$`)

// RoleService 角色与权限服务
type RoleService struct {
	db    *gorm.DB
	cache map[uint][]string
	mu    sync.RWMutex
}

// NewRoleService 创建角色服务
func NewRoleService(db *gorm.DB) *RoleService {
	return &RoleService{
		db:    db,
		cache: make(map[uint][]string),
	}
}

// ListPermissions 返回全部权限
func (s *RoleService) ListPermissions() ([]models.Permission, error) {
	var perms []models.Permission
	if err := s.db.Order("sort_order ASC, id ASC").Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("查询权限列表失败: %w", err)
	}
	return perms, nil
}

// ListRoles 返回角色列表（含权限 codes 与关联用户数）
func (s *RoleService) ListRoles() ([]map[string]interface{}, error) {
	var roles []models.Role
	if err := s.db.Order("is_system DESC, id ASC").Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("查询角色列表失败: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(roles))
	for _, role := range roles {
		item, err := s.buildRoleResponse(&role, true)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

// ListRoleOptions 返回角色下拉选项
func (s *RoleService) ListRoleOptions() ([]map[string]interface{}, error) {
	var roles []models.Role
	if err := s.db.Select("id", "code", "name", "is_system").Order("is_system DESC, id ASC").Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("查询角色选项失败: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(roles))
	for _, role := range roles {
		perms, err := s.GetRolePermissions(role.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, map[string]interface{}{
			"id":          role.ID,
			"code":        role.Code,
			"name":        role.Name,
			"is_system":   role.IsSystem,
			"permissions": perms,
		})
	}
	return result, nil
}

// GetRoleByID 获取角色详情
func (s *RoleService) GetRoleByID(id uint) (map[string]interface{}, error) {
	var role models.Role
	if err := s.db.First(&role, id).Error; err != nil {
		return nil, fmt.Errorf("角色不存在")
	}
	return s.buildRoleResponse(&role, true)
}

// CreateRole 创建自定义角色
func (s *RoleService) CreateRole(name, code, description string, permCodes []string) (*models.Role, error) {
	code = strings.TrimSpace(strings.ToLower(code))
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("角色名称不能为空")
	}
	if !roleCodePattern.MatchString(code) {
		return nil, fmt.Errorf("角色标识只能包含小写字母、数字和下划线，且以字母开头")
	}
	if len(permCodes) == 0 {
		return nil, fmt.Errorf("至少选择一项权限")
	}

	var count int64
	s.db.Model(&models.Role{}).Where("code = ?", code).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("角色标识 %s 已存在", code)
	}

	perms, err := s.findPermissionsByCodes(permCodes)
	if err != nil {
		return nil, err
	}

	role := &models.Role{
		Code:        code,
		Name:        name,
		Description: strings.TrimSpace(description),
		IsSystem:    false,
		Permissions: perms,
	}
	if err := s.db.Create(role).Error; err != nil {
		return nil, fmt.Errorf("创建角色失败: %w", err)
	}
	s.InvalidateCache(role.ID)
	return role, nil
}

// UpdateRole 更新角色
func (s *RoleService) UpdateRole(id uint, name, description string, permCodes []string) error {
	var role models.Role
	if err := s.db.First(&role, id).Error; err != nil {
		return fmt.Errorf("角色不存在")
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("角色名称不能为空")
	}
	if len(permCodes) == 0 {
		return fmt.Errorf("至少选择一项权限")
	}

	perms, err := s.findPermissionsByCodes(permCodes)
	if err != nil {
		return err
	}

	if role.Code == models.RoleAdmin {
		if !containsCode(permCodes, models.PermUsers) || !containsCode(permCodes, models.PermRoles) {
			return fmt.Errorf("管理员角色必须保留用户管理和角色管理权限")
		}
	}

	if err := s.ensureAdminCapabilityRetained(id, permCodes); err != nil {
		return err
	}

	role.Name = strings.TrimSpace(name)
	role.Description = strings.TrimSpace(description)
	if err := s.db.Model(&role).Updates(map[string]interface{}{
		"name":        role.Name,
		"description": role.Description,
	}).Error; err != nil {
		return fmt.Errorf("更新角色失败: %w", err)
	}
	if err := s.db.Model(&role).Association("Permissions").Replace(perms); err != nil {
		return fmt.Errorf("更新角色权限失败: %w", err)
	}
	s.InvalidateCache(id)
	return nil
}

// DeleteRole 删除角色
func (s *RoleService) DeleteRole(id uint) error {
	var role models.Role
	if err := s.db.First(&role, id).Error; err != nil {
		return fmt.Errorf("角色不存在")
	}
	if role.IsSystem {
		return fmt.Errorf("系统内置角色不可删除")
	}

	var userCount int64
	s.db.Model(&models.User{}).Where("role_id = ?", id).Count(&userCount)
	if userCount > 0 {
		return fmt.Errorf("该角色下仍有 %d 个用户，无法删除", userCount)
	}

	if err := s.db.Select("Permissions").Delete(&role).Error; err != nil {
		return fmt.Errorf("删除角色失败: %w", err)
	}
	s.InvalidateCache(id)
	return nil
}

// RoleExists 检查角色是否存在
func (s *RoleService) RoleExists(id uint) bool {
	var count int64
	s.db.Model(&models.Role{}).Where("id = ?", id).Count(&count)
	return count > 0
}

// GetRolePermissions 返回角色的权限 code 列表（带缓存）
func (s *RoleService) GetRolePermissions(roleID uint) ([]string, error) {
	s.mu.RLock()
	if perms, ok := s.cache[roleID]; ok {
		s.mu.RUnlock()
		return perms, nil
	}
	s.mu.RUnlock()

	var role models.Role
	if err := s.db.Preload("Permissions", func(db *gorm.DB) *gorm.DB {
		return db.Order("sort_order ASC, id ASC")
	}).First(&role, roleID).Error; err != nil {
		return nil, fmt.Errorf("角色不存在")
	}

	codes := make([]string, 0, len(role.Permissions))
	for _, p := range role.Permissions {
		codes = append(codes, p.Code)
	}
	if len(codes) == 0 {
		codes = []string{models.PermSync}
	}

	s.mu.Lock()
	s.cache[roleID] = codes
	s.mu.Unlock()
	return codes, nil
}

// HasPermission 判断角色是否拥有指定权限
func (s *RoleService) HasPermission(roleID uint, permission string) bool {
	perms, err := s.GetRolePermissions(roleID)
	if err != nil {
		return false
	}
	for _, p := range perms {
		if p == permission {
			return true
		}
	}
	return false
}

// InvalidateCache 清除角色权限缓存
func (s *RoleService) InvalidateCache(roleID uint) {
	s.mu.Lock()
	delete(s.cache, roleID)
	s.mu.Unlock()
}

func (s *RoleService) buildRoleResponse(role *models.Role, withUserCount bool) (map[string]interface{}, error) {
	perms, err := s.GetRolePermissions(role.ID)
	if err != nil {
		return nil, err
	}
	item := map[string]interface{}{
		"id":          role.ID,
		"code":        role.Code,
		"name":        role.Name,
		"description": role.Description,
		"is_system":   role.IsSystem,
		"permissions": perms,
		"created_at":  role.CreatedAt,
		"updated_at":  role.UpdatedAt,
	}
	if withUserCount {
		var userCount int64
		s.db.Model(&models.User{}).Where("role_id = ?", role.ID).Count(&userCount)
		item["user_count"] = userCount
	}
	return item, nil
}

func (s *RoleService) findPermissionsByCodes(codes []string) ([]models.Permission, error) {
	unique := dedupeStrings(codes)
	var perms []models.Permission
	if err := s.db.Where("code IN ?", unique).Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("查询权限失败: %w", err)
	}
	if len(perms) != len(unique) {
		return nil, fmt.Errorf("包含无效的权限标识")
	}
	return perms, nil
}

func (s *RoleService) ensureAdminCapabilityRetained(roleID uint, permCodes []string) error {
	if containsCode(permCodes, models.PermUsers) && containsCode(permCodes, models.PermRoles) {
		return nil
	}

	var otherAdminRoles int64
	s.db.Model(&models.Role{}).
		Joins("JOIN role_permissions rp1 ON rp1.role_id = roles.id").
		Joins("JOIN permissions p1 ON p1.id = rp1.permission_id AND p1.code = ?", models.PermUsers).
		Joins("JOIN role_permissions rp2 ON rp2.role_id = roles.id").
		Joins("JOIN permissions p2 ON p2.id = rp2.permission_id AND p2.code = ?", models.PermRoles).
		Where("roles.id <> ?", roleID).
		Count(&otherAdminRoles)
	if otherAdminRoles == 0 {
		return fmt.Errorf("系统至少需要保留一个同时拥有用户管理和角色管理权限的角色")
	}
	return nil
}

func containsCode(codes []string, target string) bool {
	for _, c := range codes {
		if c == target {
			return true
		}
	}
	return false
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
