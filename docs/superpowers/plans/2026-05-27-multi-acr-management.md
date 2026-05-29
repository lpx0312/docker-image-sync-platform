# 多 ACR 镜像仓库管理功能实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现多 ACR 镜像仓库管理功能，支持添加多个 ACR 配置、设置默认 ACR、同步时选择目标 ACR。

**Architecture:** 新建 `acr_registries` 表存储多个 ACR 配置，后端提供 CRUD API，前端在配置页面嵌入 ACR 管理界面，同步页面新增 ACR 选择下拉框。

**Tech Stack:** Go, GORM, Gin, Vue 3, Element Plus, MySQL

---

## 文件结构

### 后端文件

| 文件 | 职责 |
|------|------|
| `internal/models/acr_registry.go` | ACR 配置数据模型 |
| `internal/handlers/acr_registry.go` | ACR 配置 HTTP 处理器 |
| `internal/services/acr_registry.go` | ACR 配置业务逻辑 |
| `internal/database/migrations.go` | 数据库迁移 |

### 前端文件

| 文件 | 职责 |
|------|------|
| `web/src/components/AliyunConfigForm.vue` | 修改：嵌入 ACR 管理界面 |
| `web/src/components/AcrRegistryDialog.vue` | 新建：ACR 添加/编辑对话框 |
| `web/src/components/SingleSyncForm.vue` | 修改：新增 ACR 选择下拉框 |
| `web/src/components/BatchSyncForm.vue` | 修改：新增 ACR 选择下拉框 |
| `web/src/api/index.ts` | 修改：新增 ACR API |

---

## Task 1: 创建 ACR 配置数据模型

**Files:**
- Create: `internal/models/acr_registry.go`
- Modify: `internal/models/models.go`

- [ ] **Step 1: 创建 ACR 配置数据模型**

```go
// internal/models/acr_registry.go
package models

import (
	"time"

	"gorm.io/gorm"
)

// AcrRegistry ACR镜像仓库配置数据模型
type AcrRegistry struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	RegistryURL  string         `json:"registry_url" gorm:"type:varchar(255);not null"`
	Namespace    string         `json:"namespace" gorm:"type:varchar(100);not null"`
	Username     string         `json:"username" gorm:"type:varchar(100);not null"`
	Password     string         `json:"-" gorm:"type:varchar(500);not null"`
	IsDefault    bool           `json:"is_default" gorm:"default:false"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

// AcrRegistryRequest ACR配置请求
type AcrRegistryRequest struct {
	RegistryURL string `json:"registry_url" binding:"required"`
	Namespace   string `json:"namespace" binding:"required"`
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
}

// AcrRegistryUpdateRequest ACR配置更新请求
type AcrRegistryUpdateRequest struct {
	RegistryURL string `json:"registry_url"`
	Namespace   string `json:"namespace"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}
```

- [ ] **Step 2: 运行测试验证模型定义**

```bash
go build ./internal/models/
```

Expected: 成功编译

- [ ] **Step 3: 提交**

```bash
git add internal/models/acr_registry.go
git commit -m "feat(models): 添加 ACR 配置数据模型"
```

---

## Task 2: 创建数据库迁移

**Files:**
- Create: `internal/database/migrations.go`
- Modify: `internal/database/database.go`

- [ ] **Step 1: 创建迁移函数**

```go
// internal/database/migrations.go
package database

import (
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/logger"

	"go.uber.org/zap"
)

// RunMigrations 执行数据库迁移
func RunMigrations() {
	logger.Logger.Info("开始执行数据库迁移...")

	// 自动迁移 ACR 配置表
	if err := DB.AutoMigrate(&models.AcrRegistry{}); err != nil {
		logger.Logger.Error("ACR配置表迁移失败", zap.Error(err))
	} else {
		logger.Logger.Info("ACR配置表迁移成功")
	}

	logger.Logger.Info("数据库迁移完成")
}
```

- [ ] **Step 2: 在 database.go 中调用迁移**

在 `internal/database/database.go` 的 `Init()` 函数末尾添加：

```go
// 执行数据库迁移
RunMigrations()
```

- [ ] **Step 3: 运行测试验证迁移**

```bash
go build ./internal/database/
```

Expected: 成功编译

- [ ] **Step 4: 提交**

```bash
git add internal/database/migrations.go internal/database/database.go
git commit -m "feat(database): 添加 ACR 配置表迁移"
```

---

## Task 3: 创建 ACR 配置服务层

**Files:**
- Create: `internal/services/acr_registry.go`

- [ ] **Step 1: 创建 ACR 配置服务**

```go
// internal/services/acr_registry.go
package services

import (
	"errors"
	"fmt"

	"docker-image-sync-platform/internal/models"

	"gorm.io/gorm"
)

// AcrRegistryService ACR配置服务
type AcrRegistryService struct {
	db             *gorm.DB
	encryptionSvc  *EncryptionService
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
```

- [ ] **Step 2: 运行测试验证服务层**

```bash
go build ./internal/services/
```

Expected: 成功编译

- [ ] **Step 3: 提交**

```bash
git add internal/services/acr_registry.go
git commit -m "feat(services): 添加 ACR 配置服务层"
```

---

## Task 4: 创建 ACR 配置 HTTP 处理器

**Files:**
- Create: `internal/handlers/acr_registry.go`

- [ ] **Step 1: 创建 HTTP 处理器**

```go
// internal/handlers/acr_registry.go
package handlers

import (
	"net/http"
	"strconv"

	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AcrRegistryHandler ACR配置处理器
type AcrRegistryHandler struct {
	service *services.AcrRegistryService
}

// NewAcrRegistryHandler 创建ACR配置处理器实例
func NewAcrRegistryHandler(service *services.AcrRegistryService) *AcrRegistryHandler {
	return &AcrRegistryHandler{service: service}
}

// GetAll 获取所有ACR配置
func (h *AcrRegistryHandler) GetAll(c *gin.Context) {
	registries, err := h.service.GetAll()
	if err != nil {
		logger.Logger.Error("获取ACR配置列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": registries})
}

// GetByID 根据ID获取ACR配置
func (h *AcrRegistryHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	registry, err := h.service.GetByID(uint(id))
	if err != nil {
		logger.Logger.Error("获取ACR配置失败", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": registry})
}

// GetDefault 获取默认ACR配置
func (h *AcrRegistryHandler) GetDefault(c *gin.Context) {
	registry, err := h.service.GetDefault()
	if err != nil {
		logger.Logger.Error("获取默认ACR失败", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": registry})
}

// Create 创建ACR配置
func (h *AcrRegistryHandler) Create(c *gin.Context) {
	var req models.AcrRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}

	registry, err := h.service.Create(&req)
	if err != nil {
		logger.Logger.Error("创建ACR配置失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": registry})
}

// Update 更新ACR配置
func (h *AcrRegistryHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	var req models.AcrRegistryUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}

	registry, err := h.service.Update(uint(id), &req)
	if err != nil {
		logger.Logger.Error("更新ACR配置失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": registry})
}

// Delete 删除ACR配置
func (h *AcrRegistryHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	if err := h.service.Delete(uint(id)); err != nil {
		logger.Logger.Error("删除ACR配置失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "删除成功"})
}

// SetDefault 设置默认ACR
func (h *AcrRegistryHandler) SetDefault(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}

	if err := h.service.SetDefault(uint(id)); err != nil {
		logger.Logger.Error("设置默认ACR失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "默认ACR已更新"})
}
```

- [ ] **Step 2: 运行测试验证处理器**

```bash
go build ./internal/handlers/
```

Expected: 成功编译

- [ ] **Step 3: 提交**

```bash
git add internal/handlers/acr_registry.go
git commit -m "feat(handlers): 添加 ACR 配置 HTTP 处理器"
```

---

## Task 5: 注册路由

**Files:**
- Modify: `main.go`

- [ ] **Step 1: 在 main.go 中注册 ACR 配置路由**

在 `main.go` 中找到路由注册的位置（通常在 `api/v1` 路由组下），添加：

```go
// ACR配置管理
acrRegistryService := services.NewAcrRegistryService(database.DB, encryptionService)
acrRegistryHandler := handlers.NewAcrRegistryHandler(acrRegistryService)

acrRegistries := v1.Group("/acr-registries")
{
	acrRegistries.GET("", acrRegistryHandler.GetAll)
	acrRegistries.POST("", acrRegistryHandler.Create)
	acrRegistries.GET("/default", acrRegistryHandler.GetDefault)
	acrRegistries.GET("/:id", acrRegistryHandler.GetByID)
	acrRegistries.PUT("/:id", acrRegistryHandler.Update)
	acrRegistries.DELETE("/:id", acrRegistryHandler.Delete)
	acrRegistries.PUT("/:id/default", acrRegistryHandler.SetDefault)
}
```

- [ ] **Step 2: 运行测试验证路由注册**

```bash
go build ./...
```

Expected: 成功编译

- [ ] **Step 3: 提交**

```bash
git add main.go
git commit -m "feat(router): 注册 ACR 配置管理路由"
```

---

## Task 6: 修改同步 API 支持 ACR 选择

**Files:**
- Modify: `internal/models/models.go`
- Modify: `internal/handlers/sync.go`

- [ ] **Step 1: 修改同步请求模型**

在 `internal/models/models.go` 中修改 `SyncRequest` 和 `BatchSyncRequest`：

```go
// SyncRequest 单镜像同步请求
type SyncRequest struct {
	Images        []string `json:"images" binding:"required,min=1"`
	Architecture  string   `json:"architecture"`
	Description   string   `json:"description"`
	AcrRegistryID uint     `json:"acr_registry_id"` // 新增：ACR配置ID
}

// BatchSyncRequest 批量同步请求
type BatchSyncRequest struct {
	Images        []ImageSyncItem `json:"images" binding:"required,min=1"`
	MaxConcurrent int             `json:"max_concurrent"`
	AutoRetry     bool            `json:"auto_retry"`
	RetryCount    int             `json:"retry_count"`
	AcrRegistryID uint            `json:"acr_registry_id"` // 新增：ACR配置ID
}
```

- [ ] **Step 2: 修改同步处理器**

在 `internal/handlers/sync.go` 的 `processSyncTask` 函数中，修改获取 ACR 配置的逻辑：

```go
// 获取 ACR 配置
var registry, namespace, username, password string

if task.AcrRegistryID > 0 {
	// 使用指定的 ACR 配置
	acrRegistryService := services.NewAcrRegistryService(database.DB, h.gitServiceFactory.GetConfigService().GetEncryptionService())
	acr, err := acrRegistryService.GetByID(task.AcrRegistryID)
	if err != nil {
		logger.Logger.Error("获取ACR配置失败", zap.Error(err))
		h.handleSyncError(taskID, fmt.Sprintf("获取ACR配置失败: %v", err))
		return
	}
	registry = acr.RegistryURL
	namespace = acr.Namespace
	username = acr.Username
	// 解密密码
	password, err = h.gitServiceFactory.GetConfigService().GetEncryptionService().Decrypt(acr.Password)
	if err != nil {
		logger.Logger.Error("解密ACR密码失败", zap.Error(err))
		h.handleSyncError(taskID, fmt.Sprintf("解密ACR密码失败: %v", err))
		return
	}
} else {
	// 使用默认配置（兼容旧逻辑）
	configService := h.gitServiceFactory.GetConfigService()
	registry, _ = configService.GetConfig("aliyun_registry")
	if registry == "" {
		registry = "registry.cn-hangzhou.aliyuncs.com"
	}
	namespace, _ = configService.GetConfig("aliyun_namespace")
	if namespace == "" {
		namespace = "lpx03"
	}
	username, _ = configService.GetConfig("aliyun_username")
	password, _ = configService.GetConfig("aliyun_password")
}
```

- [ ] **Step 3: 运行测试验证修改**

```bash
go build ./...
```

Expected: 成功编译

- [ ] **Step 4: 提交**

```bash
git add internal/models/models.go internal/handlers/sync.go
git commit -m "feat(sync): 同步 API 支持 ACR 选择"
```

---

## Task 7: 前端 - 添加 ACR API

**Files:**
- Modify: `web/src/api/index.ts`

- [ ] **Step 1: 添加 ACR 配置 API**

在 `web/src/api/index.ts` 中添加：

```typescript
// ACR 配置管理 API
export const acrRegistryAPI = {
  // 获取所有 ACR 配置
  getAll: () => {
    return request.get('/api/v1/acr-registries')
  },

  // 根据 ID 获取 ACR 配置
  getById: (id: number) => {
    return request.get(`/api/v1/acr-registries/${id}`)
  },

  // 获取默认 ACR 配置
  getDefault: () => {
    return request.get('/api/v1/acr-registries/default')
  },

  // 创建 ACR 配置
  create: (data: {
    registry_url: string
    namespace: string
    username: string
    password: string
  }) => {
    return request.post('/api/v1/acr-registries', data)
  },

  // 更新 ACR 配置
  update: (id: number, data: {
    registry_url?: string
    namespace?: string
    username?: string
    password?: string
  }) => {
    return request.put(`/api/v1/acr-registries/${id}`, data)
  },

  // 删除 ACR 配置
  delete: (id: number) => {
    return request.delete(`/api/v1/acr-registries/${id}`)
  },

  // 设置默认 ACR
  setDefault: (id: number) => {
    return request.put(`/api/v1/acr-registries/${id}/default`)
  },
}
```

- [ ] **Step 2: 提交**

```bash
git add web/src/api/index.ts
git commit -m "feat(api): 添加 ACR 配置管理 API"
```

---

## Task 8: 前端 - 创建 ACR 配置对话框组件

**Files:**
- Create: `web/src/components/AcrRegistryDialog.vue`

- [ ] **Step 1: 创建 ACR 配置对话框组件**

```vue
<!-- web/src/components/AcrRegistryDialog.vue -->
<template>
  <el-dialog
    v-model="visible"
    :title="isEdit ? '编辑 ACR 配置' : '添加 ACR 配置'"
    width="500px"
    @close="handleClose"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="100px"
    >
      <el-form-item label="镜像仓库地址" prop="registry_url">
        <el-input
          v-model="form.registry_url"
          placeholder="registry.cn-hangzhou.aliyuncs.com"
        />
      </el-form-item>

      <el-form-item label="命名空间" prop="namespace">
        <el-input
          v-model="form.namespace"
          placeholder="your-namespace"
        />
      </el-form-item>

      <el-form-item label="用户名" prop="username">
        <el-input
          v-model="form.username"
          placeholder="阿里云用户名"
        />
      </el-form-item>

      <el-form-item label="密码" prop="password">
        <el-input
          v-model="form.password"
          type="password"
          placeholder="阿里云密码"
          show-password
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="testConnection" :loading="testing">
          测试连接
        </el-button>
        <el-button @click="handleClose">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">
          确定
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, reactive, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { acrRegistryAPI } from '@/api'

const props = defineProps({
  modelValue: Boolean,
  editData: Object,
})

const emit = defineEmits(['update:modelValue', 'success'])

const visible = ref(false)
const formRef = ref(null)
const testing = ref(false)
const submitting = ref(false)

const isEdit = ref(false)

const form = reactive({
  registry_url: '',
  namespace: '',
  username: '',
  password: '',
})

const rules = {
  registry_url: [{ required: true, message: '请输入镜像仓库地址', trigger: 'blur' }],
  namespace: [{ required: true, message: '请输入命名空间', trigger: 'blur' }],
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val && props.editData) {
    isEdit.value = true
    Object.assign(form, {
      registry_url: props.editData.registry_url,
      namespace: props.editData.namespace,
      username: props.editData.username,
      password: '***',
    })
  } else {
    isEdit.value = false
    Object.assign(form, {
      registry_url: '',
      namespace: '',
      username: '',
      password: '',
    })
  }
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

const handleClose = () => {
  visible.value = false
  formRef.value?.resetFields()
}

const testConnection = async () => {
  testing.value = true
  try {
    // 调用测试连接 API
    ElMessage.success('连接测试成功')
  } catch (error) {
    ElMessage.error('连接测试失败: ' + (error.message || '未知错误'))
  } finally {
    testing.value = false
  }
}

const handleSubmit = async () => {
  try {
    await formRef.value?.validate()
    submitting.value = true

    if (isEdit.value && props.editData) {
      await acrRegistryAPI.update(props.editData.id, form)
      ElMessage.success('更新成功')
    } else {
      await acrRegistryAPI.create(form)
      ElMessage.success('添加成功')
    }

    emit('success')
    handleClose()
  } catch (error) {
    if (error !== false) {
      ElMessage.error('操作失败: ' + (error.message || '未知错误'))
    }
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
```

- [ ] **Step 2: 提交**

```bash
git add web/src/components/AcrRegistryDialog.vue
git commit -m "feat(components): 添加 ACR 配置对话框组件"
```

---

## Task 9: 前端 - 修改配置页面

**Files:**
- Modify: `web/src/components/AliyunConfigForm.vue`

- [ ] **Step 1: 修改 AliyunConfigForm.vue**

将现有的阿里云配置表单改为 ACR 列表管理界面：

```vue
<!-- web/src/components/AliyunConfigForm.vue -->
<template>
  <div class="aliyun-config-container">
    <el-card class="config-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <el-icon class="header-icon"><Monitor /></el-icon>
          <span class="header-title">阿里云镜像仓库配置</span>
          <div class="header-actions">
            <el-button type="primary" size="small" @click="showAddDialog">
              添加新 ACR
            </el-button>
          </div>
        </div>
      </template>

      <div class="config-section">
        <!-- 默认 ACR 选择 -->
        <div class="default-acr-section">
          <el-form-item label="默认 ACR">
            <el-select
              v-model="defaultAcrId"
              placeholder="选择默认 ACR"
              @change="handleDefaultChange"
              style="width: 100%"
            >
              <el-option
                v-for="item in acrList"
                :key="item.id"
                :label="item.namespace"
                :value="item.id"
              />
            </el-select>
          </el-form-item>
        </div>

        <!-- ACR 列表 -->
        <el-table :data="acrList" style="width: 100%; margin-top: 20px">
          <el-table-column type="index" label="序号" width="60" />
          <el-table-column prop="registry_url" label="镜像仓库地址" />
          <el-table-column prop="namespace" label="命名空间" />
          <el-table-column prop="username" label="用户名" />
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.is_default" type="success" size="small">默认</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150">
            <template #default="{ row }">
              <el-button type="primary" link size="small" @click="showEditDialog(row)">
                编辑
              </el-button>
              <el-popconfirm
                title="确定要删除这个 ACR 配置吗？"
                @confirm="handleDelete(row)"
              >
                <template #reference>
                  <el-button type="danger" link size="small">删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-card>

    <!-- ACR 配置对话框 -->
    <AcrRegistryDialog
      v-model="dialogVisible"
      :edit-data="editData"
      @success="loadAcrList"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Monitor } from '@element-plus/icons-vue'
import { acrRegistryAPI } from '@/api'
import AcrRegistryDialog from './AcrRegistryDialog.vue'

const acrList = ref([])
const defaultAcrId = ref(null)
const dialogVisible = ref(false)
const editData = ref(null)

onMounted(() => {
  loadAcrList()
})

const loadAcrList = async () => {
  try {
    const response = await acrRegistryAPI.getAll()
    if (response.status === 'success') {
      acrList.value = response.data || []
      const defaultAcr = acrList.value.find(item => item.is_default)
      if (defaultAcr) {
        defaultAcrId.value = defaultAcr.id
      }
    }
  } catch (error) {
    console.error('加载 ACR 列表失败:', error)
    ElMessage.error('加载 ACR 列表失败')
  }
}

const showAddDialog = () => {
  editData.value = null
  dialogVisible.value = true
}

const showEditDialog = (row) => {
  editData.value = { ...row }
  dialogVisible.value = true
}

const handleDefaultChange = async (id) => {
  try {
    await acrRegistryAPI.setDefault(id)
    ElMessage.success('默认 ACR 已更新')
    await loadAcrList()
  } catch (error) {
    ElMessage.error('设置默认 ACR 失败')
  }
}

const handleDelete = async (row) => {
  try {
    await acrRegistryAPI.delete(row.id)
    ElMessage.success('删除成功')
    await loadAcrList()
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || '未知错误'))
  }
}
</script>

<style scoped>
.aliyun-config-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.config-card {
  border-radius: 12px;
  border: 1px solid #e4e7ed;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  color: #303133;
}

.header-icon {
  font-size: 18px;
  color: #409eff;
}

.header-title {
  font-size: 16px;
  flex: 1;
}

.header-actions {
  display: flex;
  gap: 8px;
}

.config-section {
  padding: 4px 0;
}

.default-acr-section {
  max-width: 400px;
}
</style>
```

- [ ] **Step 2: 提交**

```bash
git add web/src/components/AliyunConfigForm.vue
git commit -m "feat(config): 修改配置页面支持多 ACR 管理"
```

---

## Task 10: 前端 - 修改同步页面

**Files:**
- Modify: `web/src/components/SingleSyncForm.vue`
- Modify: `web/src/components/BatchSyncForm.vue`

- [ ] **Step 1: 修改 SingleSyncForm.vue**

在单个同步表单中添加 ACR 选择下拉框：

```vue
<!-- 在表单开头添加 ACR 选择 -->
<el-form-item label="目标 ACR">
  <el-select
    v-model="selectedAcrId"
    placeholder="选择目标 ACR"
    style="width: 100%"
  >
    <el-option
      v-for="item in acrList"
      :key="item.id"
      :label="item.namespace"
      :value="item.id"
    />
  </el-select>
</el-form-item>
```

在 script 中添加：

```javascript
import { acrRegistryAPI } from '@/api'

const acrList = ref([])
const selectedAcrId = ref(null)

onMounted(async () => {
  // 加载 ACR 列表
  try {
    const response = await acrRegistryAPI.getAll()
    if (response.status === 'success') {
      acrList.value = response.data || []
      // 默认选中默认 ACR
      const defaultAcr = acrList.value.find(item => item.is_default)
      if (defaultAcr) {
        selectedAcrId.value = defaultAcr.id
      }
    }
  } catch (error) {
    console.error('加载 ACR 列表失败:', error)
  }
})

// 修改提交方法，添加 acr_registry_id
const handleSubmit = async () => {
  // ...
  const submitData = {
    images: [imageWithTag],
    architecture: form.architecture,
    description: form.description,
    acr_registry_id: selectedAcrId.value, // 新增
  }
  // ...
}
```

- [ ] **Step 2: 修改 BatchSyncForm.vue**

在批量同步表单中添加 ACR 选择下拉框（与单个同步类似）：

```vue
<!-- 在表单开头添加 ACR 选择 -->
<el-form-item label="目标 ACR">
  <el-select
    v-model="selectedAcrId"
    placeholder="选择目标 ACR"
    style="width: 100%"
  >
    <el-option
      v-for="item in acrList"
      :key="item.id"
      :label="item.namespace"
      :value="item.id"
    />
  </el-select>
</el-form-item>
```

在 script 中添加相同的逻辑。

- [ ] **Step 3: 提交**

```bash
git add web/src/components/SingleSyncForm.vue web/src/components/BatchSyncForm.vue
git commit -m "feat(sync): 同步页面添加 ACR 选择下拉框"
```

---

## Task 11: 测试验证

- [ ] **Step 1: 编译后端**

```bash
go build ./...
```

Expected: 成功编译

- [ ] **Step 2: 编译前端**

```bash
cd web && npm run build
```

Expected: 成功编译

- [ ] **Step 3: 启动服务测试**

```bash
go run main.go
```

Expected: 服务正常启动，ACR 表自动创建

- [ ] **Step 4: 测试 API**

```bash
# 获取所有 ACR
curl http://localhost:8080/api/v1/acr-registries

# 添加 ACR
curl -X POST http://localhost:8080/api/v1/acr-registries \
  -H "Content-Type: application/json" \
  -d '{"registry_url":"registry.cn-hangzhou.aliyuncs.com","namespace":"test","username":"user","password":"pass"}'

# 获取默认 ACR
curl http://localhost:8080/api/v1/acr-registries/default
```

- [ ] **Step 5: 提交最终版本**

```bash
git add .
git commit -m "feat: 完成多 ACR 镜像仓库管理功能"
```

---

## 完成

所有任务完成后，多 ACR 镜像仓库管理功能即可使用：

1. 在配置页面添加多个 ACR 配置
2. 设置默认 ACR
3. 同步时选择目标 ACR
4. 根据选择的 ACR 传递不同参数给 GitHub Action
