# Git配置数据库化迁移设计文档

## 1. 需求概述

### 1.1 功能描述
将Git仓库配置（Gitee和GitHub的所有配置信息）从静态配置文件迁移到数据库中进行动态管理，支持在系统配置页面进行实时配置和修改，无需重启服务。

### 1.2 设计目标
- **配置动态化**：所有Git配置信息存储在数据库中，支持实时修改
- **最小化改造**：尽量减少对现有代码的修改，保持功能稳定性
- **统一管理**：在系统配置页面统一管理Git和阿里云配置
- **向下兼容**：保持现有API接口的兼容性
- **安全性**：敏感信息（密码、Token）加密存储

### 1.3 涉及配置项
**Gitee配置**：
- repo_url: 仓库URL地址
- username: 用户名
- password: 密码或访问令牌
- email: 提交邮箱

**GitHub配置**：
- repo_url: 仓库URL地址
- username: 用户名
- token: Personal Access Token

**阿里云配置**：
- registry: 镜像仓库地址
- namespace: 命名空间
- username: 用户名
- password: 密码

## 2. 数据库设计

### 2.1 system_configs表扩展

在现有的`system_configs`表中添加以下配置项：

| config_key | config_value | description | is_encrypted |
|------------|--------------|-------------|--------------|
| git_repository_type | gitee | Git仓库类型选择：gitee或github | false |
| gitee_repo_url | https://gitee.com/lpx03/docker_image_pusher.git | Gitee仓库URL地址 | false |
| gitee_username | 13297049661 | Gitee用户名 | false |
| gitee_password | [encrypted] | Gitee密码或访问令牌 | true |
| gitee_email | lpx_0312@163.com | Gitee提交邮箱 | false |
| github_repo_url | https://github.com/lpx0312/docker_image_pusher.git | GitHub仓库URL地址 | false |
| github_username | lpx0312 | GitHub用户名 | false |
| github_token | [encrypted] | GitHub Personal Access Token | true |
| aliyun_registry | registry.cn-hangzhou.aliyuncs.com | 阿里云镜像仓库地址 | false |
| aliyun_namespace | lpx03 | 阿里云命名空间 | false |
| aliyun_username | lipanx | 阿里云用户名 | false |
| aliyun_password | [encrypted] | 阿里云密码 | true |

### 2.2 表结构调整

```sql
-- 为system_configs表添加加密标识字段（如果不存在）
ALTER TABLE system_configs ADD COLUMN IF NOT EXISTS is_encrypted BOOLEAN DEFAULT FALSE COMMENT '是否为加密字段';

-- 添加配置分组字段，便于管理
ALTER TABLE system_configs ADD COLUMN IF NOT EXISTS config_group VARCHAR(50) DEFAULT 'system' COMMENT '配置分组';

-- 添加配置显示顺序字段
ALTER TABLE system_configs ADD COLUMN IF NOT EXISTS display_order INT DEFAULT 0 COMMENT '显示顺序';
```

## 3. 后端实现

### 3.1 配置服务扩展

#### 3.1.1 新增配置管理方法

```go
// internal/services/config_service.go

type ConfigService struct {
    encryptionKey string
}

// Git配置相关方法
func (s *ConfigService) GetGiteeConfig() (*GiteeConfig, error)
func (s *ConfigService) UpdateGiteeConfig(config *GiteeConfig) error
func (s *ConfigService) GetGitHubConfig() (*GitHubConfig, error)
func (s *ConfigService) UpdateGitHubConfig(config *GitHubConfig) error

// 阿里云配置相关方法
func (s *ConfigService) GetAliyunConfig() (*AliyunConfig, error)
func (s *ConfigService) UpdateAliyunConfig(config *AliyunConfig) error

// 通用配置方法
func (s *ConfigService) GetConfigValue(key string) (string, error)
func (s *ConfigService) SetConfigValue(key, value string, encrypted bool) error
func (s *ConfigService) GetConfigsByGroup(group string) (map[string]string, error)
```

#### 3.1.2 加密解密服务

```go
// internal/services/encryption_service.go

type EncryptionService struct {
    key []byte
}

func (s *EncryptionService) Encrypt(plaintext string) (string, error)
func (s *EncryptionService) Decrypt(ciphertext string) (string, error)
func (s *EncryptionService) IsEncrypted(value string) bool
```

### 3.2 配置API扩展

#### 3.2.1 新增API接口

```go
// internal/handlers/config.go

// GET /api/v1/config/git
// 获取Git配置信息（包括Gitee和GitHub）
func (h *ConfigHandler) GetGitConfig(c *gin.Context)

// PUT /api/v1/config/git/gitee
// 更新Gitee配置
func (h *ConfigHandler) UpdateGiteeConfig(c *gin.Context)

// PUT /api/v1/config/git/github
// 更新GitHub配置
func (h *ConfigHandler) UpdateGitHubConfig(c *gin.Context)

// GET /api/v1/config/aliyun
// 获取阿里云配置信息
func (h *ConfigHandler) GetAliyunConfig(c *gin.Context)

// PUT /api/v1/config/aliyun
// 更新阿里云配置
func (h *ConfigHandler) UpdateAliyunConfig(c *gin.Context)

// GET /api/v1/config/all
// 获取所有系统配置（分组返回）
func (h *ConfigHandler) GetAllConfigs(c *gin.Context)
```

#### 3.2.2 请求响应结构

```go
// 请求结构
type GiteeConfigRequest struct {
    RepoURL  string `json:"repo_url" binding:"required,url"`
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required"`
    Email    string `json:"email" binding:"required,email"`
}

type GitHubConfigRequest struct {
    RepoURL  string `json:"repo_url" binding:"required,url"`
    Username string `json:"username" binding:"required"`
    Token    string `json:"token" binding:"required"`
}

type AliyunConfigRequest struct {
    Registry  string `json:"registry" binding:"required"`
    Namespace string `json:"namespace" binding:"required"`
    Username  string `json:"username" binding:"required"`
    Password  string `json:"password" binding:"required"`
}

// 响应结构
type GitConfigResponse struct {
    RepositoryType string              `json:"repository_type"`
    Gitee         GiteeConfigResponse  `json:"gitee"`
    GitHub        GitHubConfigResponse `json:"github"`
}

type GiteeConfigResponse struct {
    RepoURL  string `json:"repo_url"`
    Username string `json:"username"`
    Email    string `json:"email"`
    // 注意：不返回密码
}

type GitHubConfigResponse struct {
    RepoURL  string `json:"repo_url"`
    Username string `json:"username"`
    // 注意：不返回Token
}
```

### 3.3 Git服务适配

#### 3.3.1 动态配置加载

```go
// internal/services/git.go

// 修改getCurrentGitConfig方法，从数据库读取配置
func (s *GitService) getCurrentGitConfig() (repoURL, username, token, email, repoType string, err error) {
    configService := NewConfigService()
    
    // 获取仓库类型
    repoType, err = configService.GetConfigValue("git_repository_type")
    if err != nil || repoType == "" {
        repoType = "gitee" // 默认值
    }
    
    switch repoType {
    case "github":
        config, err := configService.GetGitHubConfig()
        if err != nil {
            return "", "", "", "", "", err
        }
        return config.RepoURL, config.Username, config.Token, "", "github", nil
        
    case "gitee":
        fallthrough
    default:
        config, err := configService.GetGiteeConfig()
        if err != nil {
            return "", "", "", "", "", err
        }
        return config.RepoURL, config.Username, config.Password, config.Email, "gitee", nil
    }
}
```

#### 3.3.2 配置缓存机制

```go
// internal/services/config_cache.go

type ConfigCache struct {
    gitConfig   *GitConfigCache
    aliyunConfig *AliyunConfigCache
    mutex       sync.RWMutex
    lastUpdate  time.Time
    ttl         time.Duration
}

type GitConfigCache struct {
    RepositoryType string
    Gitee         *GiteeConfig
    GitHub        *GitHubConfig
}

func (c *ConfigCache) GetGitConfig() *GitConfigCache {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    
    if time.Since(c.lastUpdate) > c.ttl {
        c.refreshGitConfig()
    }
    
    return c.gitConfig
}

func (c *ConfigCache) RefreshGitConfig() error {
    // 从数据库重新加载配置
}

func (c *ConfigCache) InvalidateCache() {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.lastUpdate = time.Time{}
}
```

## 4. 前端实现

### 4.1 配置页面设计

#### 4.1.1 页面结构

```vue
<!-- web/src/views/SystemConfigView.vue -->
<template>
  <div class="system-config">
    <el-tabs v-model="activeTab" type="card">
      <!-- Git配置标签页 -->
      <el-tab-pane label="Git仓库配置" name="git">
        <git-config-panel 
          :config="gitConfig" 
          @update="handleGitConfigUpdate"
        />
      </el-tab-pane>
      
      <!-- 阿里云配置标签页 -->
      <el-tab-pane label="阿里云配置" name="aliyun">
        <aliyun-config-panel 
          :config="aliyunConfig" 
          @update="handleAliyunConfigUpdate"
        />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>
```

#### 4.1.2 Git配置组件

```vue
<!-- web/src/components/GitConfigPanel.vue -->
<template>
  <div class="git-config-panel">
    <!-- 仓库类型选择 -->
    <el-form-item label="同步仓库类型">
      <el-radio-group v-model="localConfig.repository_type" @change="handleRepoTypeChange">
        <el-radio label="gitee">Gitee</el-radio>
        <el-radio label="github">GitHub</el-radio>
      </el-radio-group>
    </el-form-item>
    
    <!-- Gitee配置 -->
    <el-collapse v-model="activeCollapse">
      <el-collapse-item title="Gitee配置" name="gitee">
        <el-form :model="localConfig.gitee" label-width="120px">
          <el-form-item label="仓库URL" required>
            <el-input v-model="localConfig.gitee.repo_url" placeholder="https://gitee.com/用户名/仓库名.git" />
          </el-form-item>
          <el-form-item label="用户名" required>
            <el-input v-model="localConfig.gitee.username" />
          </el-form-item>
          <el-form-item label="密码/令牌" required>
            <el-input v-model="localConfig.gitee.password" type="password" show-password />
          </el-form-item>
          <el-form-item label="邮箱" required>
            <el-input v-model="localConfig.gitee.email" />
          </el-form-item>
        </el-form>
      </el-collapse-item>
      
      <!-- GitHub配置 -->
      <el-collapse-item title="GitHub配置" name="github">
        <el-form :model="localConfig.github" label-width="120px">
          <el-form-item label="仓库URL" required>
            <el-input v-model="localConfig.github.repo_url" placeholder="https://github.com/用户名/仓库名.git" />
          </el-form-item>
          <el-form-item label="用户名" required>
            <el-input v-model="localConfig.github.username" />
          </el-form-item>
          <el-form-item label="访问令牌" required>
            <el-input v-model="localConfig.github.token" type="password" show-password />
          </el-form-item>
        </el-form>
      </el-collapse-item>
    </el-collapse>
    
    <!-- 操作按钮 -->
    <div class="config-actions">
      <el-button type="primary" @click="saveConfig" :loading="saving">保存配置</el-button>
      <el-button @click="testConnection" :loading="testing">测试连接</el-button>
      <el-button @click="resetConfig">重置</el-button>
    </div>
  </div>
</template>
```

### 4.2 API调用服务

```javascript
// web/src/api/config.js

export const configApi = {
  // 获取Git配置
  async getGitConfig() {
    const response = await request.get('/api/v1/config/git')
    return response.data
  },
  
  // 更新Gitee配置
  async updateGiteeConfig(config) {
    const response = await request.put('/api/v1/config/git/gitee', config)
    return response.data
  },
  
  // 更新GitHub配置
  async updateGitHubConfig(config) {
    const response = await request.put('/api/v1/config/git/github', config)
    return response.data
  },
  
  // 获取阿里云配置
  async getAliyunConfig() {
    const response = await request.get('/api/v1/config/aliyun')
    return response.data
  },
  
  // 更新阿里云配置
  async updateAliyunConfig(config) {
    const response = await request.put('/api/v1/config/aliyun', config)
    return response.data
  },
  
  // 测试Git连接
  async testGitConnection(type) {
    const response = await request.post(`/api/v1/config/git/${type}/test`)
    return response.data
  }
}
```

## 5. 实施步骤

### 5.1 第一阶段：数据库迁移和基础服务

**步骤1：数据库结构调整**
1. 执行数据库迁移脚本，添加新字段
2. 将现有config.yaml中的配置迁移到数据库
3. 创建配置初始化脚本

**步骤2：加密服务实现**
1. 实现EncryptionService
2. 添加配置加密/解密功能
3. 处理敏感信息的安全存储

**步骤3：配置服务重构**
1. 实现ConfigService
2. 添加配置缓存机制
3. 实现配置的CRUD操作

### 5.2 第二阶段：API接口开发

**步骤4：配置API实现**
1. 扩展ConfigHandler
2. 添加Git和阿里云配置相关API
3. 实现配置验证和测试接口

**步骤5：Git服务适配**
1. 修改GitService的配置加载逻辑
2. 实现动态配置切换
3. 添加配置变更通知机制

### 5.3 第三阶段：前端界面开发

**步骤6：配置页面开发**
1. 创建系统配置页面
2. 实现Git配置组件
3. 实现阿里云配置组件

**步骤7：用户体验优化**
1. 添加配置验证和提示
2. 实现配置测试功能
3. 添加配置导入导出功能

### 5.4 第四阶段：测试和部署

**步骤8：功能测试**
1. 单元测试：配置服务、加密服务
2. 集成测试：API接口、Git操作
3. 端到端测试：完整配置流程

**步骤9：部署和迁移**
1. 生产环境数据库迁移
2. 配置数据迁移脚本
3. 服务平滑升级

## 6. 数据迁移方案

### 6.1 迁移脚本

```sql
-- migration_git_config_to_db.sql

-- 插入Git配置项
INSERT INTO system_configs (config_key, config_value, description, config_group, display_order, is_encrypted, created_at, updated_at) VALUES
('git_repository_type', 'gitee', 'Git仓库类型选择：gitee或github', 'git', 1, FALSE, NOW(), NOW()),
('gitee_repo_url', 'https://gitee.com/lpx03/docker_image_pusher.git', 'Gitee仓库URL地址', 'git', 2, FALSE, NOW(), NOW()),
('gitee_username', '13297049661', 'Gitee用户名', 'git', 3, FALSE, NOW(), NOW()),
('gitee_password', '[ENCRYPTED_VALUE]', 'Gitee密码或访问令牌', 'git', 4, TRUE, NOW(), NOW()),
('gitee_email', 'lpx_0312@163.com', 'Gitee提交邮箱', 'git', 5, FALSE, NOW(), NOW()),
('github_repo_url', 'https://github.com/lpx0312/docker_image_pusher.git', 'GitHub仓库URL地址', 'git', 6, FALSE, NOW(), NOW()),
('github_username', 'lpx0312', 'GitHub用户名', 'git', 7, FALSE, NOW(), NOW()),
('github_token', '[ENCRYPTED_VALUE]', 'GitHub Personal Access Token', 'git', 8, TRUE, NOW(), NOW());

-- 插入阿里云配置项
INSERT INTO system_configs (config_key, config_value, description, config_group, display_order, is_encrypted, created_at, updated_at) VALUES
('aliyun_registry', 'registry.cn-hangzhou.aliyuncs.com', '阿里云镜像仓库地址', 'aliyun', 1, FALSE, NOW(), NOW()),
('aliyun_namespace', 'lpx03', '阿里云命名空间', 'aliyun', 2, FALSE, NOW(), NOW()),
('aliyun_username', 'lipanx', '阿里云用户名', 'aliyun', 3, FALSE, NOW(), NOW()),
('aliyun_password', '[ENCRYPTED_VALUE]', '阿里云密码', 'aliyun', 4, TRUE, NOW(), NOW());
```

### 6.2 配置迁移工具

```go
// cmd/migrate_config/main.go

func main() {
    // 读取现有config.yaml
    config, err := loadConfigFromFile("config.yaml")
    if err != nil {
        log.Fatal(err)
    }
    
    // 连接数据库
    db, err := connectDatabase()
    if err != nil {
        log.Fatal(err)
    }
    
    // 迁移Git配置
    err = migrateGitConfig(db, config)
    if err != nil {
        log.Fatal(err)
    }
    
    // 迁移阿里云配置
    err = migrateAliyunConfig(db, config)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Println("配置迁移完成")
}
```

## 7. 安全考虑

### 7.1 敏感信息加密

```go
// 使用AES-256-GCM加密敏感配置
func encryptSensitiveConfig(plaintext string) (string, error) {
    key := getEncryptionKey() // 从环境变量或密钥管理系统获取
    
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}
```

### 7.2 访问控制

```go
// 添加配置访问权限检查
func (h *ConfigHandler) checkConfigPermission(c *gin.Context, configGroup string) bool {
    // 实现基于角色的访问控制
    userRole := getUserRole(c)
    
    switch configGroup {
    case "git", "aliyun":
        return userRole == "admin" || userRole == "operator"
    default:
        return userRole == "admin"
    }
}
```

## 8. 监控和日志

### 8.1 配置变更审计

```go
// 记录配置变更日志
func logConfigChange(userID, configKey, oldValue, newValue string) {
    logger.Logger.Info("Configuration changed",
        zap.String("user_id", userID),
        zap.String("config_key", configKey),
        zap.String("old_value", maskSensitiveValue(oldValue)),
        zap.String("new_value", maskSensitiveValue(newValue)),
        zap.Time("timestamp", time.Now()),
    )
}
```

### 8.2 配置健康检查

```go
// 定期检查配置有效性
func (s *ConfigService) HealthCheck() error {
    // 检查Git配置
    if err := s.validateGitConfig(); err != nil {
        return fmt.Errorf("git config invalid: %w", err)
    }
    
    // 检查阿里云配置
    if err := s.validateAliyunConfig(); err != nil {
        return fmt.Errorf("aliyun config invalid: %w", err)
    }
    
    return nil
}
```

## 9. 回滚方案

### 9.1 配置回滚

```go
// 配置版本管理和回滚
type ConfigVersion struct {
    ID        uint      `json:"id"`
    ConfigKey string    `json:"config_key"`
    OldValue  string    `json:"old_value"`
    NewValue  string    `json:"new_value"`
    UserID    string    `json:"user_id"`
    CreatedAt time.Time `json:"created_at"`
}

func (s *ConfigService) RollbackConfig(configKey string, versionID uint) error {
    // 查找指定版本
    var version ConfigVersion
    err := s.db.Where("id = ? AND config_key = ?", versionID, configKey).First(&version).Error
    if err != nil {
        return err
    }
    
    // 回滚配置
    return s.SetConfigValue(configKey, version.OldValue, false)
}
```

### 9.2 紧急回退

```go
// 紧急情况下回退到配置文件模式
func (s *GitService) fallbackToConfigFile() error {
    logger.Logger.Warn("Falling back to config file due to database config error")
    
    // 重新加载config.yaml
    config, err := loadConfigFromFile("config.yaml")
    if err != nil {
        return err
    }
    
    // 使用文件配置
    s.useFileConfig = true
    s.fileConfig = config
    
    return nil
}
```

## 10. 性能优化

### 10.1 配置缓存策略

```go
// 多级缓存策略
type ConfigCacheManager struct {
    l1Cache *sync.Map           // 内存缓存
    l2Cache redis.Client        // Redis缓存
    db      *gorm.DB           // 数据库
    ttl     time.Duration      // 缓存TTL
}

func (c *ConfigCacheManager) GetConfig(key string) (string, error) {
    // L1缓存查找
    if value, ok := c.l1Cache.Load(key); ok {
        return value.(string), nil
    }
    
    // L2缓存查找
    value, err := c.l2Cache.Get(context.Background(), key).Result()
    if err == nil {
        c.l1Cache.Store(key, value)
        return value, nil
    }
    
    // 数据库查找
    var config models.SystemConfig
    err = c.db.Where("config_key = ?", key).First(&config).Error
    if err != nil {
        return "", err
    }
    
    // 更新缓存
    c.l1Cache.Store(key, config.ConfigValue)
    c.l2Cache.Set(context.Background(), key, config.ConfigValue, c.ttl)
    
    return config.ConfigValue, nil
}
```

## 11. 测试计划

### 11.1 单元测试

```go
// 配置服务测试
func TestConfigService_GetGiteeConfig(t *testing.T) {
    // 测试正常获取配置
    // 测试配置不存在的情况
    // 测试解密失败的情况
}

func TestConfigService_UpdateGiteeConfig(t *testing.T) {
    // 测试正常更新配置
    // 测试配置验证失败
    // 测试加密存储
}
```

### 11.2 集成测试

```go
// API接口测试
func TestConfigAPI_UpdateGitConfig(t *testing.T) {
    // 测试配置更新API
    // 测试权限验证
    // 测试配置验证
}

// Git服务集成测试
func TestGitService_WithDatabaseConfig(t *testing.T) {
    // 测试从数据库加载配置
    // 测试配置缓存
    // 测试配置热更新
}
```

## 12. 部署检查清单

### 12.1 部署前检查

- [ ] 数据库迁移脚本准备完成
- [ ] 加密密钥配置完成
- [ ] 配置迁移工具测试通过
- [ ] 回滚方案准备完成
- [ ] 监控和日志配置完成

### 12.2 部署后验证

- [ ] 配置页面功能正常
- [ ] Git操作使用数据库配置
- [ ] 配置缓存工作正常
- [ ] 敏感信息加密存储
- [ ] 配置变更日志记录正常

## 13. 总结

本设计方案实现了Git和阿里云配置的完全数据库化管理，主要特点：

1. **配置动态化**：支持实时修改配置，无需重启服务
2. **安全性增强**：敏感信息加密存储，支持访问控制
3. **用户体验优化**：统一的配置管理界面，支持配置测试
4. **系统稳定性**：多级缓存、配置验证、回滚机制
5. **最小化改造**：保持现有API兼容性，渐进式迁移

该方案能够满足配置动态管理的需求，同时保证系统的安全性和稳定性。