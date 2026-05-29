# ACR 镜像管理功能实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 ACR 镜像管理功能，支持查看和管理 ACR 中的镜像列表和 tag 信息。

**Architecture:** 新建 `acr_repositories` 表存储镜像名称列表，后端提供 CRUD API 和 ACR API 集成，前端新增独立页面展示两层结构（镜像名称 → tag 列表）。

**Tech Stack:** Go, GORM, Gin, Vue 3, Element Plus, MySQL, 阿里云 ACR API

---

## 文件结构

### 后端文件

| 文件 | 职责 |
|------|------|
| `internal/models/acr_repository.go` | ACR 镜像仓库数据模型 |
| `internal/services/acr_api.go` | ACR API 调用服务（Token、Tag、Manifest） |
| `internal/services/acr_repository.go` | ACR 镜像仓库业务逻辑 |
| `internal/handlers/acr_repository.go` | ACR 镜像仓库 HTTP 处理器 |
| `internal/handlers/acr_tag.go` | ACR Tag 查询 HTTP 处理器 |
| `internal/database/migrations.go` | 数据库迁移（修改） |

### 前端文件

| 文件 | 职责 |
|------|------|
| `web/src/views/ImagesManageView.vue` | 镜像管理主页面 |
| `web/src/components/AcrRepositoryList.vue` | 镜像列表组件 |
| `web/src/components/AcrTagList.vue` | Tag 列表组件 |
| `web/src/components/AddRepositoryDialog.vue` | 添加镜像对话框 |
| `web/src/components/BatchAddRepositoryDialog.vue` | 批量添加镜像对话框 |
| `web/src/api/index.js` | API 接口（修改） |
| `web/src/router/index.js` | 路由配置（修改） |

---

## Task 1: 创建 ACR 镜像仓库数据模型

**Files:**
- Create: `internal/models/acr_repository.go`
- Modify: `internal/database/migrations.go`

- [ ] **Step 1: 创建 ACR 镜像仓库数据模型**

```go
// internal/models/acr_repository.go
package models

import (
	"time"

	"gorm.io/gorm"
)

// AcrRepository ACR镜像仓库数据模型
type AcrRepository struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	AcrRegistryID  uint           `json:"acr_registry_id" gorm:"not null;index"`
	RepositoryName string         `json:"repository_name" gorm:"type:varchar(255);not null"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
	// 关联
	AcrRegistry    *AcrRegistry   `json:"acr_registry,omitempty" gorm:"foreignKey:AcrRegistryID"`
}

// AcrRepositoryRequest 添加镜像请求
type AcrRepositoryRequest struct {
	AcrRegistryID  uint   `json:"acr_registry_id" binding:"required"`
	RepositoryName string `json:"repository_name" binding:"required"`
}

// AcrRepositoryBatchRequest 批量添加镜像请求
type AcrRepositoryBatchRequest struct {
	AcrRegistryID   uint     `json:"acr_registry_id" binding:"required"`
	RepositoryNames []string `json:"repository_names" binding:"required,min=1"`
}

// AcrRepositorySyncRequest 从同步记录导入请求
type AcrRepositorySyncRequest struct {
	AcrRegistryID uint `json:"acr_registry_id" binding:"required"`
}
```

- [ ] **Step 2: 修改数据库迁移**

在 `internal/database/migrations.go` 中添加：

```go
// 自动迁移 ACR 镜像仓库表
if err := DB.AutoMigrate(&models.AcrRepository{}); err != nil {
    logger.Logger.Error("ACR镜像仓库表迁移失败", zap.Error(err))
} else {
    logger.Logger.Info("ACR镜像仓库表迁移成功")
}
```

- [ ] **Step 3: 验证编译**

```bash
go build ./internal/models/
```

- [ ] **Step 4: 提交**

```bash
git add internal/models/acr_repository.go internal/database/migrations.go
git commit -m "feat(models): 添加 ACR 镜像仓库数据模型"
```

---

## Task 2: 创建 ACR API 调用服务

**Files:**
- Create: `internal/services/acr_api.go`

- [ ] **Step 1: 创建 ACR API 服务**

```go
// internal/services/acr_api.go
package services

import (
	"encoding/json"
	"fmt"
	"time"

	"docker-image-sync-platform/internal/logger"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
)

// AcrAPIService ACR API调用服务
type AcrAPIService struct {
	client      *resty.Client
	tokenCache  map[string]*tokenCacheItem
}

type tokenCacheItem struct {
	token     string
	expiresAt time.Time
}

// ACRManifest ACR Manifest 结构
type ACRManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Manifests     []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
		Platform  struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		} `json:"platform"`
	} `json:"manifests"`
	Config struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

// TagDetail Tag详细信息
type TagDetail struct {
	Tag          string            `json:"tag"`
	Architectures []string         `json:"architectures"`
	Digests      map[string]string `json:"digests"`
	Sizes        map[string]int64  `json:"sizes"`
	PushedAt     map[string]string `json:"pushed_at"`
}

// NewAcrAPIService 创建ACR API服务实例
func NewAcrAPIService() *AcrAPIService {
	return &AcrAPIService{
		client:     resty.New().SetTimeout(30 * time.Second),
		tokenCache: make(map[string]*tokenCacheItem),
	}
}

// getAuthServer 获取认证服务器地址
func getAuthServer(registry string) string {
	// 从 registry 推断 region
	// registry.cn-hangzhou.aliyuncs.com -> dockerauth.cn-hangzhou.aliyuncs.com
	region := "cn-hangzhou"
	if len(registry) > 20 {
		prefix := registry[:20]
		if idx := len("registry."); idx < len(prefix) {
			end := len(prefix)
			for i := idx; i < len(prefix); i++ {
				if prefix[i] == '.' {
					end = i
					break
				}
			}
			region = prefix[idx:end]
		}
	}
	return fmt.Sprintf("dockerauth.%s.aliyuncs.com", region)
}

// getDockerService 获取 Docker 服务标识
func getDockerService(registry, namespace string) string {
	return fmt.Sprintf("registry.aliyuncs.com:%s:%s", "cn-hangzhou", namespace)
}

// GetToken 获取 ACR 认证 Token
func (s *AcrAPIService) GetToken(registry, username, password, namespace, repo string) (string, error) {
	cacheKey := fmt.Sprintf("%s:%s:%s", registry, namespace, repo)
	
	// 检查缓存
	if item, ok := s.tokenCache[cacheKey]; ok && time.Now().Before(item.expiresAt) {
		return item.token, nil
	}
	
	authServer := getAuthServer(registry)
	scope := fmt.Sprintf("repository:%s/%s:pull", namespace, repo)
	service := getDockerService(registry, namespace)
	
	var result struct {
		Token string `json:"token"`
	}
	
	resp, err := s.client.R().
		SetBasicAuth(username, password).
		SetFormData(map[string]string{
			"service": service,
			"scope":   scope,
		}).
		SetResult(&result).
		Post(fmt.Sprintf("https://%s/auth", authServer))
	
	if err != nil {
		return "", fmt.Errorf("获取Token失败: %w", err)
	}
	
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("获取Token失败: HTTP %d", resp.StatusCode())
	}
	
	if result.Token == "" {
		return "", fmt.Errorf("获取Token失败: 返回空Token")
	}
	
	// 缓存 Token（25分钟）
	s.tokenCache[cacheKey] = &tokenCacheItem{
		token:     result.Token,
		expiresAt: time.Now().Add(25 * time.Minute),
	}
	
	return result.Token, nil
}

// GetTags 获取镜像的 Tag 列表
func (s *AcrAPIService) GetTags(registry, username, password, namespace, repo string) ([]string, error) {
	token, err := s.GetToken(registry, username, password, namespace, repo)
	if err != nil {
		return nil, err
	}
	
	var result struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	
	resp, err := s.client.R().
		SetAuthToken(token).
		SetHeader("Accept", "application/vnd.docker.distribution.manifest.v2+json").
		SetResult(&result).
		Get(fmt.Sprintf("https://%s/v2/%s/%s/tags/list", registry, namespace, repo))
	
	if err != nil {
		return nil, fmt.Errorf("获取Tag列表失败: %w", err)
	}
	
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("获取Tag列表失败: HTTP %d", resp.StatusCode())
	}
	
	return result.Tags, nil
}

// GetTagDetail 获取 Tag 详细信息
func (s *AcrAPIService) GetTagDetail(registry, username, password, namespace, repo, tag string) (*TagDetail, error) {
	token, err := s.GetToken(registry, username, password, namespace, repo)
	if err != nil {
		return nil, err
	}
	
	acceptHeader := "application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json,application/vnd.docker.distribution.manifest.list.v2+json"
	
	resp, err := s.client.R().
		SetAuthToken(token).
		SetHeader("Accept", acceptHeader).
		Get(fmt.Sprintf("https://%s/v2/%s/%s/manifests/%s", registry, namespace, repo, tag))
	
	if err != nil {
		return nil, fmt.Errorf("获取Manifest失败: %w", err)
	}
	
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("获取Manifest失败: HTTP %d", resp.StatusCode())
	}
	
	var manifest ACRManifest
	if err := json.Unmarshal(resp.Body(), &manifest); err != nil {
		return nil, fmt.Errorf("解析Manifest失败: %w", err)
	}
	
	detail := &TagDetail{
		Tag:          tag,
		Architectures: make([]string, 0),
		Digests:      make(map[string]string),
		Sizes:        make(map[string]int64),
		PushedAt:     make(map[string]string),
	}
	
	if len(manifest.Manifests) > 0 {
		// 多架构镜像
		for _, m := range manifest.Manifests {
			if m.Platform.OS == "linux" && m.Platform.Architecture != "" {
				arch := m.Platform.Architecture
				detail.Architectures = append(detail.Architectures, arch)
				detail.Digests[arch] = m.Digest
				detail.Sizes[arch] = m.Size
			}
		}
	} else {
		// 单架构镜像
		detail.Architectures = append(detail.Architectures, "unknown")
		detail.Digests["unknown"] = resp.Header().Get("Docker-Content-Digest")
	}
	
	return detail, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./internal/services/
```

- [ ] **Step 3: 提交**

```bash
git add internal/services/acr_api.go
git commit -m "feat(services): 添加 ACR API 调用服务"
```

---

## Task 3: 创建 ACR 镜像仓库服务层

**Files:**
- Create: `internal/services/acr_repository.go`

- [ ] **Step 1: 创建 ACR 镜像仓库服务**

```go
// internal/services/acr_repository.go
package services

import (
	"errors"
	"fmt"
	"strings"

	"docker-image-sync-platform/internal/models"

	"gorm.io/gorm"
)

// AcrRepositoryService ACR镜像仓库服务
type AcrRepositoryService struct {
	db *gorm.DB
}

// NewAcrRepositoryService 创建ACR镜像仓库服务实例
func NewAcrRepositoryService(db *gorm.DB) *AcrRepositoryService {
	return &AcrRepositoryService{db: db}
}

// GetAll 获取指定ACR的所有镜像
func (s *AcrRepositoryService) GetAll(acrRegistryID uint) ([]models.AcrRepository, error) {
	var repos []models.AcrRepository
	if err := s.db.Where("acr_registry_id = ?", acrRegistryID).
		Order("repository_name ASC").
		Find(&repos).Error; err != nil {
		return nil, fmt.Errorf("获取镜像列表失败: %w", err)
	}
	return repos, nil
}

// GetByID 根据ID获取镜像
func (s *AcrRepositoryService) GetByID(id uint) (*models.AcrRepository, error) {
	var repo models.AcrRepository
	if err := s.db.First(&repo, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("镜像不存在: %d", id)
		}
		return nil, fmt.Errorf("获取镜像失败: %w", err)
	}
	return &repo, nil
}

// Create 创建镜像
func (s *AcrRepositoryService) Create(req *models.AcrRepositoryRequest) (*models.AcrRepository, error) {
	// 检查是否已存在
	var count int64
	s.db.Model(&models.AcrRepository{}).
		Where("acr_registry_id = ? AND repository_name = ?", req.AcrRegistryID, req.RepositoryName).
		Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("镜像已存在: %s", req.RepositoryName)
	}
	
	repo := &models.AcrRepository{
		AcrRegistryID:  req.AcrRegistryID,
		RepositoryName: req.RepositoryName,
	}
	
	if err := s.db.Create(repo).Error; err != nil {
		return nil, fmt.Errorf("创建镜像失败: %w", err)
	}
	
	return repo, nil
}

// BatchCreate 批量创建镜像
func (s *AcrRepositoryService) BatchCreate(req *models.AcrRepositoryBatchRequest) (int, error) {
	created := 0
	for _, name := range req.RepositoryNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		
		// 检查是否已存在
		var count int64
		s.db.Model(&models.AcrRepository{}).
			Where("acr_registry_id = ? AND repository_name = ?", req.AcrRegistryID, name).
			Count(&count)
		if count > 0 {
			continue
		}
		
		repo := &models.AcrRepository{
			AcrRegistryID:  req.AcrRegistryID,
			RepositoryName: name,
		}
		
		if err := s.db.Create(repo).Error; err != nil {
			continue
		}
		created++
	}
	
	return created, nil
}

// Delete 删除镜像
func (s *AcrRepositoryService) Delete(id uint) error {
	if err := s.db.Delete(&models.AcrRepository{}, id).Error; err != nil {
		return fmt.Errorf("删除镜像失败: %w", err)
	}
	return nil
}

// SyncFromRecords 从同步记录中提取镜像名称
func (s *AcrRepositoryService) SyncFromRecords(acrRegistryID uint) (int, error) {
	// 查询指定 ACR 的成功同步记录
	var records []models.ImageSyncRecord
	if err := s.db.Where("acr_registry_id = ? AND sync_status = ?", acrRegistryID, models.SyncStatusSuccess).
		Find(&records).Error; err != nil {
		return 0, fmt.Errorf("查询同步记录失败: %w", err)
	}
	
	created := 0
	for _, record := range records {
		// 提取镜像名称（不含 tag）
		repoName := extractRepoName(record.OriginalImage)
		if repoName == "" {
			continue
		}
		
		// 检查是否已存在
		var count int64
		s.db.Model(&models.AcrRepository{}).
			Where("acr_registry_id = ? AND repository_name = ?", acrRegistryID, repoName).
			Count(&count)
		if count > 0 {
			continue
		}
		
		repo := &models.AcrRepository{
			AcrRegistryID:  acrRegistryID,
			RepositoryName: repoName,
		}
		
		if err := s.db.Create(repo).Error; err != nil {
			continue
		}
		created++
	}
	
	return created, nil
}

// extractRepoName 从镜像地址中提取仓库名称
func extractRepoName(image string) string {
	// 移除 tag
	parts := strings.Split(image, ":")
	if len(parts) > 1 {
		image = parts[0]
	}
	
	// 移除 registry 前缀
	if strings.Contains(image, ".") {
		parts = strings.SplitN(image, "/", 2)
		if len(parts) > 1 {
			return parts[1]
		}
	}
	
	return image
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./internal/services/
```

- [ ] **Step 3: 提交**

```bash
git add internal/services/acr_repository.go
git commit -m "feat(services): 添加 ACR 镜像仓库服务层"
```

---

## Task 4: 创建 ACR 镜像仓库 HTTP 处理器

**Files:**
- Create: `internal/handlers/acr_repository.go`

- [ ] **Step 1: 创建 HTTP 处理器**

```go
// internal/handlers/acr_repository.go
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

// AcrRepositoryHandler ACR镜像仓库处理器
type AcrRepositoryHandler struct {
	service *services.AcrRepositoryService
}

// NewAcrRepositoryHandler 创建ACR镜像仓库处理器实例
func NewAcrRepositoryHandler(service *services.AcrRepositoryService) *AcrRepositoryHandler {
	return &AcrRepositoryHandler{service: service}
}

// GetAll 获取所有镜像
func (h *AcrRepositoryHandler) GetAll(c *gin.Context) {
	acrRegistryIDStr := c.Query("acr_registry_id")
	if acrRegistryIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "缺少 acr_registry_id 参数"})
		return
	}
	
	acrRegistryID, err := strconv.ParseUint(acrRegistryIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的 acr_registry_id"})
		return
	}
	
	repos, err := h.service.GetAll(uint(acrRegistryID))
	if err != nil {
		logger.Logger.Error("获取镜像列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": repos})
}

// Create 创建镜像
func (h *AcrRepositoryHandler) Create(c *gin.Context) {
	var req models.AcrRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}
	
	repo, err := h.service.Create(&req)
	if err != nil {
		logger.Logger.Error("创建镜像失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	
	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": repo})
}

// BatchCreate 批量创建镜像
func (h *AcrRepositoryHandler) BatchCreate(c *gin.Context) {
	var req models.AcrRepositoryBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}
	
	created, err := h.service.BatchCreate(&req)
	if err != nil {
		logger.Logger.Error("批量创建镜像失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("成功添加 %d 个镜像", created),
		"data":    gin.H{"created": created},
	})
}

// Delete 删除镜像
func (h *AcrRepositoryHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的ID"})
		return
	}
	
	if err := h.service.Delete(uint(id)); err != nil {
		logger.Logger.Error("删除镜像失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "删除成功"})
}

// SyncFromRecords 从同步记录导入
func (h *AcrRepositoryHandler) SyncFromRecords(c *gin.Context) {
	var req models.AcrRepositorySyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求参数错误: " + err.Error()})
		return
	}
	
	created, err := h.service.SyncFromRecords(req.AcrRegistryID)
	if err != nil {
		logger.Logger.Error("从同步记录导入失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("成功导入 %d 个镜像", created),
		"data":    gin.H{"created": created},
	})
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./internal/handlers/
```

- [ ] **Step 3: 提交**

```bash
git add internal/handlers/acr_repository.go
git commit -m "feat(handlers): 添加 ACR 镜像仓库 HTTP 处理器"
```

---

## Task 5: 创建 ACR Tag 查询 HTTP 处理器

**Files:**
- Create: `internal/handlers/acr_tag.go`

- [ ] **Step 1: 创建 HTTP 处理器**

```go
// internal/handlers/acr_tag.go
package handlers

import (
	"net/http"
	"strconv"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AcrTagHandler ACR Tag查询处理器
type AcrTagHandler struct {
	acrAPIService  *services.AcrAPIService
	acrRegService  *services.AcrRegistryService
}

// NewAcrTagHandler 创建ACR Tag查询处理器实例
func NewAcrTagHandler(acrAPIService *services.AcrAPIService, acrRegService *services.AcrRegistryService) *AcrTagHandler {
	return &AcrTagHandler{
		acrAPIService: acrAPIService,
		acrRegService: acrRegService,
	}
}

// GetTags 获取镜像的Tag列表
func (h *AcrTagHandler) GetTags(c *gin.Context) {
	acrRegistryIDStr := c.Query("acr_registry_id")
	repositoryName := c.Query("repository_name")
	
	if acrRegistryIDStr == "" || repositoryName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "缺少必要参数"})
		return
	}
	
	acrRegistryID, err := strconv.ParseUint(acrRegistryIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的 acr_registry_id"})
		return
	}
	
	// 获取 ACR 配置
	acr, err := h.acrRegService.GetByID(uint(acrRegistryID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "ACR配置不存在"})
		return
	}
	
	// 解密密码
	password, err := h.acrRegService.GetDecryptedPassword(uint(acrRegistryID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "解密密码失败"})
		return
	}
	
	// 获取 Tag 列表
	tags, err := h.acrAPIService.GetTags(acr.RegistryURL, acr.Username, password, acr.Namespace, repositoryName)
	if err != nil {
		logger.Logger.Error("获取Tag列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": tags})
}

// GetTagDetail 获取Tag详细信息
func (h *AcrTagHandler) GetTagDetail(c *gin.Context) {
	acrRegistryIDStr := c.Query("acr_registry_id")
	repositoryName := c.Query("repository_name")
	tag := c.Query("tag")
	
	if acrRegistryIDStr == "" || repositoryName == "" || tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "缺少必要参数"})
		return
	}
	
	acrRegistryID, err := strconv.ParseUint(acrRegistryIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "无效的 acr_registry_id"})
		return
	}
	
	// 获取 ACR 配置
	acr, err := h.acrRegService.GetByID(uint(acrRegistryID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "ACR配置不存在"})
		return
	}
	
	// 解密密码
	password, err := h.acrRegService.GetDecryptedPassword(uint(acrRegistryID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "解密密码失败"})
		return
	}
	
	// 获取 Tag 详细信息
	detail, err := h.acrAPIService.GetTagDetail(acr.RegistryURL, acr.Username, password, acr.Namespace, repositoryName, tag)
	if err != nil {
		logger.Logger.Error("获取Tag详细信息失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": detail})
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./internal/handlers/
```

- [ ] **Step 3: 提交**

```bash
git add internal/handlers/acr_tag.go
git commit -m "feat(handlers): 添加 ACR Tag 查询 HTTP 处理器"
```

---

## Task 6: 注册路由

**Files:**
- Modify: `main.go`

- [ ] **Step 1: 在 main.go 中注册路由**

在 `main.go` 中添加：

```go
// ACR镜像管理
acrRepositoryService := services.NewAcrRepositoryService(database.DB)
acrRepositoryHandler := handlers.NewAcrRepositoryHandler(acrRepositoryService)

acrRepositories := v1.Group("/acr-repositories")
{
	acrRepositories.GET("", acrRepositoryHandler.GetAll)
	acrRepositories.POST("", acrRepositoryHandler.Create)
	acrRepositories.POST("/batch", acrRepositoryHandler.BatchCreate)
	acrRepositories.DELETE("/:id", acrRepositoryHandler.Delete)
	acrRepositories.POST("/sync-from-records", acrRepositoryHandler.SyncFromRecords)
}

// ACR Tag查询
acrAPIService := services.NewAcrAPIService()
acrTagHandler := handlers.NewAcrTagHandler(acrAPIService, acrRegistryService)

acrTags := v1.Group("/acr-tags")
{
	acrTags.GET("", acrTagHandler.GetTags)
	acrTags.GET("/detail", acrTagHandler.GetTagDetail)
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./...
```

- [ ] **Step 3: 提交**

```bash
git add main.go
git commit -m "feat(router): 注册 ACR 镜像管理路由"
```

---

## Task 7: 前端 - 添加 API 接口

**Files:**
- Modify: `web/src/api/index.js`

- [ ] **Step 1: 添加 ACR 镜像管理 API**

在 `web/src/api/index.js` 中添加：

```javascript
// ACR 镜像管理 API
export const acrRepositoryAPI = {
  // 获取镜像列表
  getAll: (acrRegistryId) => {
    return api.get('/acr-repositories', { params: { acr_registry_id: acrRegistryId } })
  },

  // 添加镜像
  create: (data) => {
    return api.post('/acr-repositories', data)
  },

  // 批量添加镜像
  batchCreate: (data) => {
    return api.post('/acr-repositories/batch', data)
  },

  // 删除镜像
  delete: (id) => {
    return api.delete(`/acr-repositories/${id}`)
  },

  // 从同步记录导入
  syncFromRecords: (acrRegistryId) => {
    return api.post('/acr-repositories/sync-from-records', { acr_registry_id: acrRegistryId })
  },
}

// ACR Tag 查询 API
export const acrTagAPI = {
  // 获取 Tag 列表
  getTags: (acrRegistryId, repositoryName) => {
    return api.get('/acr-tags', { params: { acr_registry_id: acrRegistryId, repository_name: repositoryName } })
  },

  // 获取 Tag 详细信息
  getTagDetail: (acrRegistryId, repositoryName, tag) => {
    return api.get('/acr-tags/detail', { params: { acr_registry_id: acrRegistryId, repository_name: repositoryName, tag } })
  },
}
```

- [ ] **Step 2: 提交**

```bash
git add web/src/api/index.js
git commit -m "feat(api): 添加 ACR 镜像管理 API"
```

---

## Task 8: 前端 - 创建添加镜像对话框组件

**Files:**
- Create: `web/src/components/AddRepositoryDialog.vue`

- [ ] **Step 1: 创建添加镜像对话框组件**

```vue
<!-- web/src/components/AddRepositoryDialog.vue -->
<template>
  <el-dialog
    v-model="visible"
    title="添加镜像"
    width="500px"
    @close="handleClose"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="镜像名称" prop="repository_name">
        <el-input
          v-model="form.repository_name"
          placeholder="例如: nginx、library/mysql"
        />
      </el-form-item>
      <el-form-item>
        <el-text type="info" size="small">
          输入镜像名称，不含 tag 和 registry 地址
        </el-text>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
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
import { acrRepositoryAPI } from '@/api'

const props = defineProps({
  modelValue: Boolean,
  acrRegistryId: Number,
})

const emit = defineEmits(['update:modelValue', 'success'])

const visible = ref(false)
const formRef = ref(null)
const submitting = ref(false)

const form = reactive({
  repository_name: '',
})

const rules = {
  repository_name: [{ required: true, message: '请输入镜像名称', trigger: 'blur' }],
}

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val) {
    form.repository_name = ''
  }
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

const handleClose = () => {
  visible.value = false
  formRef.value?.resetFields()
}

const handleSubmit = async () => {
  try {
    await formRef.value?.validate()
    submitting.value = true

    await acrRepositoryAPI.create({
      acr_registry_id: props.acrRegistryId,
      repository_name: form.repository_name,
    })

    ElMessage.success('添加成功')
    emit('success')
    handleClose()
  } catch (error) {
    if (error !== false) {
      ElMessage.error('添加失败: ' + (error.message || '未知错误'))
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
git add web/src/components/AddRepositoryDialog.vue
git commit -m "feat(components): 添加镜像对话框组件"
```

---

## Task 9: 前端 - 创建批量添加镜像对话框组件

**Files:**
- Create: `web/src/components/BatchAddRepositoryDialog.vue`

- [ ] **Step 1: 创建批量添加镜像对话框组件**

```vue
<!-- web/src/components/BatchAddRepositoryDialog.vue -->
<template>
  <el-dialog
    v-model="visible"
    title="批量添加镜像"
    width="600px"
    @close="handleClose"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="镜像列表" prop="repository_names">
        <el-input
          v-model="form.repository_names"
          type="textarea"
          :rows="10"
          placeholder="每行一个镜像名称，例如:&#10;nginx&#10;redis&#10;mysql"
        />
      </el-form-item>
      <el-form-item>
        <el-text type="info" size="small">
          每行输入一个镜像名称，不含 tag 和 registry 地址
        </el-text>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
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
import { acrRepositoryAPI } from '@/api'

const props = defineProps({
  modelValue: Boolean,
  acrRegistryId: Number,
})

const emit = defineEmits(['update:modelValue', 'success'])

const visible = ref(false)
const formRef = ref(null)
const submitting = ref(false)

const form = reactive({
  repository_names: '',
})

const rules = {
  repository_names: [{ required: true, message: '请输入镜像列表', trigger: 'blur' }],
}

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val) {
    form.repository_names = ''
  }
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

const handleClose = () => {
  visible.value = false
  formRef.value?.resetFields()
}

const handleSubmit = async () => {
  try {
    await formRef.value?.validate()
    submitting.value = true

    const names = form.repository_names
      .split('\n')
      .map(n => n.trim())
      .filter(n => n !== '')

    if (names.length === 0) {
      ElMessage.warning('请输入至少一个镜像名称')
      return
    }

    const result = await acrRepositoryAPI.batchCreate({
      acr_registry_id: props.acrRegistryId,
      repository_names: names,
    })

    ElMessage.success(result.data?.message || '添加成功')
    emit('success')
    handleClose()
  } catch (error) {
    if (error !== false) {
      ElMessage.error('添加失败: ' + (error.message || '未知错误'))
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
git add web/src/components/BatchAddRepositoryDialog.vue
git commit -m "feat(components): 批量添加镜像对话框组件"
```

---

## Task 10: 前端 - 创建 Tag 列表组件

**Files:**
- Create: `web/src/components/AcrTagList.vue`

- [ ] **Step 1: 创建 Tag 列表组件**

```vue
<!-- web/src/components/AcrTagList.vue -->
<template>
  <el-dialog
    v-model="visible"
    :title="`${repositoryName} - Tag 列表`"
    width="900px"
    @close="handleClose"
  >
    <!-- 搜索区域 -->
    <div class="search-section">
      <el-row :gutter="16">
        <el-col :span="8">
          <el-input
            v-model="searchTag"
            placeholder="搜索 Tag 名称"
            clearable
            @input="handleSearch"
          />
        </el-col>
        <el-col :span="8">
          <el-input
            v-model="searchDigest"
            placeholder="搜索 SHA256"
            clearable
            @input="handleSearch"
          />
        </el-col>
        <el-col :span="8">
          <el-select
            v-model="searchArch"
            placeholder="筛选架构"
            clearable
            @change="handleSearch"
          >
            <el-option label="amd64" value="amd64" />
            <el-option label="arm64" value="arm64" />
          </el-select>
        </el-col>
      </el-row>
    </div>

    <!-- Tag 列表 -->
    <el-table
      :data="filteredTags"
      v-loading="loading"
      empty-text="暂无 Tag 数据"
      style="margin-top: 16px;"
    >
      <el-table-column prop="tag" label="Tag" width="150" />
      <el-table-column label="架构" width="120">
        <template #default="{ row }">
          <div class="arch-tags">
            <el-tag
              v-for="arch in row.architectures"
              :key="arch"
              size="small"
              :type="getArchTagType(arch)"
            >
              {{ arch }}
            </el-tag>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="Digest" min-width="200">
        <template #default="{ row }">
          <div v-for="arch in row.architectures" :key="arch" class="digest-item">
            <el-text type="info" size="small">{{ arch }}:</el-text>
            <el-text size="small" class="digest-value">{{ row.digests[arch] || '-' }}</el-text>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="大小" width="120">
        <template #default="{ row }">
          <div v-for="arch in row.architectures" :key="arch" class="size-item">
            <el-text type="info" size="small">{{ arch }}:</el-text>
            <el-text size="small">{{ formatSize(row.sizes[arch]) }}</el-text>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="80" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="handleCopy(row)">
            复制
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <template #footer>
      <el-button @click="handleClose">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { acrTagAPI } from '@/api'
import { copyToClipboard } from '@/utils/clipboard'

const props = defineProps({
  modelValue: Boolean,
  acrRegistryId: Number,
  repositoryName: String,
})

const emit = defineEmits(['update:modelValue'])

const visible = ref(false)
const loading = ref(false)
const tags = ref([])
const searchTag = ref('')
const searchDigest = ref('')
const searchArch = ref('')

watch(() => props.modelValue, (val) => {
  visible.value = val
  if (val && props.acrRegistryId && props.repositoryName) {
    loadTags()
  }
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

const filteredTags = computed(() => {
  return tags.value.filter(tag => {
    if (searchTag.value && !tag.tag.includes(searchTag.value)) {
      return false
    }
    if (searchArch.value && !tag.architectures.includes(searchArch.value)) {
      return false
    }
    if (searchDigest.value) {
      const hasDigest = Object.values(tag.digests || {}).some(d => 
        d && d.includes(searchDigest.value)
      )
      if (!hasDigest) return false
    }
    return true
  })
})

const loadTags = async () => {
  loading.value = true
  try {
    const response = await acrTagAPI.getTags(props.acrRegistryId, props.repositoryName)
    if (response && response.status === 'success') {
      const tagNames = response.data || []
      // 获取每个 tag 的详细信息
      const details = []
      for (const tagName of tagNames) {
        try {
          const detailResp = await acrTagAPI.getTagDetail(
            props.acrRegistryId,
            props.repositoryName,
            tagName
          )
          if (detailResp && detailResp.status === 'success') {
            details.push(detailResp.data)
          }
        } catch (e) {
          console.error(`获取 ${tagName} 详情失败:`, e)
          details.push({
            tag: tagName,
            architectures: [],
            digests: {},
            sizes: {},
          })
        }
      }
      tags.value = details
    }
  } catch (error) {
    console.error('加载Tag列表失败:', error)
    ElMessage.error('加载Tag列表失败')
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  // 搜索是响应式的，无需额外处理
}

const handleCopy = (tag) => {
  const text = `${props.repositoryName}:${tag.tag}`
  copyToClipboard(text)
  ElMessage.success('已复制到剪贴板')
}

const handleClose = () => {
  visible.value = false
  tags.value = []
  searchTag.value = ''
  searchDigest.value = ''
  searchArch.value = ''
}

const getArchTagType = (arch) => {
  if (arch === 'amd64') return 'primary'
  if (arch === 'arm64') return 'success'
  return 'info'
}

const formatSize = (bytes) => {
  if (!bytes) return '-'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB'
}
</script>

<style scoped>
.search-section {
  margin-bottom: 16px;
}

.arch-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.digest-item,
.size-item {
  display: flex;
  gap: 4px;
  margin-bottom: 2px;
}

.digest-value {
  word-break: break-all;
}
</style>
```

- [ ] **Step 2: 提交**

```bash
git add web/src/components/AcrTagList.vue
git commit -m "feat(components): Tag 列表组件"
```

---

## Task 11: 前端 - 创建镜像管理主页面

**Files:**
- Create: `web/src/views/ImagesManageView.vue`

- [ ] **Step 1: 创建镜像管理主页面**

```vue
<!-- web/src/views/ImagesManageView.vue -->
<template>
  <div class="images-manage-view">
    <el-card class="list-card">
      <template #header>
        <div class="card-header">
          <span>镜像管理</span>
          <div class="header-actions">
            <el-select
              v-model="selectedAcrId"
              placeholder="选择 ACR"
              style="width: 300px; margin-right: 16px;"
              @change="handleAcrChange"
            >
              <el-option
                v-for="item in acrList"
                :key="item.id"
                :label="item.namespace"
                :value="item.id"
              />
            </el-select>
            <el-button
              type="primary"
              :icon="Refresh"
              @click="loadRepositories"
              :loading="loading"
              size="small"
            >
              刷新
            </el-button>
          </div>
        </div>
      </template>

      <!-- 操作按钮 -->
      <div class="action-section">
        <el-button type="primary" size="small" @click="showAddDialog">
          添加镜像
        </el-button>
        <el-button type="success" size="small" @click="showBatchAddDialog">
          批量添加
        </el-button>
        <el-button type="warning" size="small" @click="handleSyncFromRecords" :loading="syncing">
          从同步记录导入
        </el-button>
      </div>

      <!-- 镜像列表 -->
      <el-table
        :data="repositories"
        v-loading="loading"
        empty-text="暂无镜像数据"
        style="margin-top: 16px;"
      >
        <el-table-column type="index" label="序号" width="60" />
        <el-table-column prop="repository_name" label="镜像名称" min-width="200">
          <template #default="{ row }">
            <el-button type="primary" link @click="showTagList(row)">
              {{ row.repository_name }}
            </el-button>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column prop="updated_at" label="更新时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.updated_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-popconfirm
              title="确定要删除这个镜像吗？"
              @confirm="handleDelete(row)"
            >
              <template #reference>
                <el-button type="danger" link size="small">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 添加镜像对话框 -->
    <AddRepositoryDialog
      v-model="addDialogVisible"
      :acr-registry-id="selectedAcrId"
      @success="loadRepositories"
    />

    <!-- 批量添加对话框 -->
    <BatchAddRepositoryDialog
      v-model="batchAddDialogVisible"
      :acr-registry-id="selectedAcrId"
      @success="loadRepositories"
    />

    <!-- Tag 列表对话框 -->
    <AcrTagList
      v-model="tagListVisible"
      :acr-registry-id="selectedAcrId"
      :repository-name="selectedRepoName"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { acrRegistryAPI, acrRepositoryAPI } from '@/api'
import { formatTime } from '@/utils/format'
import AddRepositoryDialog from '@/components/AddRepositoryDialog.vue'
import BatchAddRepositoryDialog from '@/components/BatchAddRepositoryDialog.vue'
import AcrTagList from '@/components/AcrTagList.vue'

const acrList = ref([])
const selectedAcrId = ref(null)
const repositories = ref([])
const loading = ref(false)
const syncing = ref(false)

const addDialogVisible = ref(false)
const batchAddDialogVisible = ref(false)
const tagListVisible = ref(false)
const selectedRepoName = ref('')

onMounted(() => {
  loadAcrList()
})

const loadAcrList = async () => {
  try {
    const response = await acrRegistryAPI.getAll()
    if (response && response.status === 'success') {
      acrList.value = response.data || []
      // 默认选中默认 ACR
      const defaultAcr = acrList.value.find(item => item.is_default)
      if (defaultAcr) {
        selectedAcrId.value = defaultAcr.id
        loadRepositories()
      }
    }
  } catch (error) {
    console.error('加载 ACR 列表失败:', error)
  }
}

const handleAcrChange = () => {
  if (selectedAcrId.value) {
    loadRepositories()
  }
}

const loadRepositories = async () => {
  if (!selectedAcrId.value) return
  
  loading.value = true
  try {
    const response = await acrRepositoryAPI.getAll(selectedAcrId.value)
    if (response && response.status === 'success') {
      repositories.value = response.data || []
    }
  } catch (error) {
    console.error('加载镜像列表失败:', error)
    ElMessage.error('加载镜像列表失败')
  } finally {
    loading.value = false
  }
}

const showAddDialog = () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择 ACR')
    return
  }
  addDialogVisible.value = true
}

const showBatchAddDialog = () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择 ACR')
    return
  }
  batchAddDialogVisible.value = true
}

const showTagList = (row) => {
  selectedRepoName.value = row.repository_name
  tagListVisible.value = true
}

const handleSyncFromRecords = async () => {
  if (!selectedAcrId.value) {
    ElMessage.warning('请先选择 ACR')
    return
  }
  
  syncing.value = true
  try {
    const response = await acrRepositoryAPI.syncFromRecords(selectedAcrId.value)
    if (response && response.status === 'success') {
      ElMessage.success(response.message || '导入成功')
      loadRepositories()
    }
  } catch (error) {
    console.error('导入失败:', error)
    ElMessage.error('导入失败')
  } finally {
    syncing.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await acrRepositoryAPI.delete(row.id)
    ElMessage.success('删除成功')
    loadRepositories()
  } catch (error) {
    ElMessage.error('删除失败')
  }
}
</script>

<style scoped>
.images-manage-view {
  max-width: var(--max-width);
  margin: 0 auto;
}

.list-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  align-items: center;
}

.action-section {
  display: flex;
  gap: 8px;
}
</style>
```

- [ ] **Step 2: 提交**

```bash
git add web/src/views/ImagesManageView.vue
git commit -m "feat(views): 镜像管理主页面"
```

---

## Task 12: 前端 - 配置路由

**Files:**
- Modify: `web/src/router/index.js`

- [ ] **Step 1: 添加路由配置**

在 `web/src/router/index.js` 中添加：

```javascript
{
  path: '/images',
  name: 'ImagesManage',
  component: () => import('@/views/ImagesManageView.vue'),
  meta: { title: '镜像管理', requiredPermission: 'sync' }
}
```

- [ ] **Step 2: 提交**

```bash
git add web/src/router/index.js
git commit -m "feat(router): 添加镜像管理路由"
```

---

## Task 13: 测试验证

- [ ] **Step 1: 编译后端**

```bash
go build ./...
```

- [ ] **Step 2: 编译前端**

```bash
cd web && npm run build
```

- [ ] **Step 3: 启动服务测试**

```bash
.claude/scripts/dev-server.sh restart
```

- [ ] **Step 4: 测试 API**

```bash
# 获取 ACR 列表
curl http://localhost:8080/api/v1/acr-registries

# 添加镜像
curl -X POST http://localhost:8080/api/v1/acr-repositories \
  -H "Content-Type: application/json" \
  -d '{"acr_registry_id":1,"repository_name":"nginx"}'

# 获取 Tag 列表
curl "http://localhost:8080/api/v1/acr-tags?acr_registry_id=1&repository_name=nginx"
```

- [ ] **Step 5: 提交最终版本**

```bash
git add .
git commit -m "feat: 完成 ACR 镜像管理功能"
```

---

## 完成

所有任务完成后，ACR 镜像管理功能即可使用：

1. 在镜像管理页面选择 ACR
2. 添加镜像名称（手动、批量、从同步记录导入）
3. 点击镜像名称查看 Tag 列表
4. 查看 Tag 的架构、digest、大小等详细信息
