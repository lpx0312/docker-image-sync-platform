# Git仓库选择功能设计文档

## 1. 需求概述

### 1.1 功能描述
为Docker镜像同步平台添加Git仓库选择功能，允许用户在前端界面配置单个同步和批量同步任务使用的Git仓库（Gitee或GitHub）。该配置作为全局设置存储在数据库中，模拟同步功能保持不变。

### 1.2 设计目标
- **最小化改造**：尽量减少对现有代码的修改
- **功能隔离**：不影响其他功能模块的正常运行
- **配置灵活**：支持动态切换Git仓库，无需重启服务
- **向下兼容**：保持现有API接口的兼容性

### 1.3 适用范围
- 单个镜像同步（SubmitSync）
- 批量镜像同步（SubmitBatchSync）
- 不包括模拟同步功能（SubmitMockBatchSync）

## 2. 现状分析

### 2.1 当前架构
```
当前同步流程：
1. 接收同步请求 → 2. 创建任务记录 → 3. 调用processSyncTask()
4. 生成images.txt → 5. 提交到Gitee → 6. 监控GitHub Actions
7. 更新同步状态 → 8. 完成同步
```

### 2.2 现有Git服务
- **GitService**: 负责Git仓库操作（克隆、提交、推送）
- **GitHubService**: 负责GitHub Actions监控
- **配置来源**: config.yaml文件中的静态配置

### 2.3 现有配置管理
- **system_configs表**: 已存在，用于存储系统配置
- **ConfigHandler**: 提供配置查询API
- **配置模型**: SystemConfig结构体已定义

## 3. 详细设计

### 3.1 数据库设计

#### 3.1.1 新增配置项
在`system_configs`表中添加以下配置项：

| config_key | config_value | description |
|------------|--------------|-------------|
| git_repository_type | gitee | Git仓库类型选择：gitee或github |

#### 3.1.2 配置项说明
- **git_repository_type**: 
  - 取值：`gitee` 或 `github`
  - 默认值：`gitee`（保持向下兼容）
  - 作用：控制单个同步和批量同步使用的Git仓库

### 3.2 后端设计

#### 3.2.1 配置服务扩展

**新增方法**：
```go
// 获取Git仓库类型配置
func GetGitRepositoryType() string

// 更新Git仓库类型配置
func UpdateGitRepositoryType(repoType string) error
```

**配置API扩展**：
```go
// GET /api/config/git-repository
// 获取当前Git仓库配置

// PUT /api/config/git-repository
// 更新Git仓库配置
```

#### 3.2.2 Git服务适配

**方案一：服务工厂模式（推荐）**
```go
type GitServiceFactory struct {
    giteeService  *GitService
    githubService *GitService
}

func (f *GitServiceFactory) GetGitService() *GitService {
    repoType := GetGitRepositoryType()
    if repoType == "github" {
        return f.githubService
    }
    return f.giteeService
}
```

**方案二：动态配置切换**
```go
func (g *GitService) SwitchRepository(repoType string) error {
    // 动态切换Git仓库配置
}
```

#### 3.2.3 同步流程修改

**修改点**：
- `processSyncTask()` 函数开始处添加仓库类型检查
- 根据配置选择对应的Git服务实例
- 保持其他逻辑不变

**伪代码**：
```go
func processSyncTask(task *models.SyncTask) {
    // 获取Git仓库配置
    repoType := config.GetGitRepositoryType()
    
    // 选择对应的Git服务
    var gitService *GitService
    if repoType == "github" {
        gitService = gitServiceFactory.GetGitHubGitService()
    } else {
        gitService = gitServiceFactory.GetGiteeGitService()
    }
    
    // 继续原有逻辑...
}
```

### 3.3 前端设计

#### 3.3.1 配置界面设计

**位置**：系统设置页面或独立的Git配置页面

**界面元素**：
```vue
<template>
  <div class="git-config-section">
    <h3>Git仓库配置</h3>
    <el-form>
      <el-form-item label="同步仓库类型">
        <el-radio-group v-model="gitRepoType" @change="handleRepoTypeChange">
          <el-radio label="gitee">Gitee</el-radio>
          <el-radio label="github">GitHub</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="saveConfig">保存配置</el-button>
      </el-form-item>
    </el-form>
  </div>
</template>
```

#### 3.3.2 API调用
```javascript
// 获取当前配置
async getGitConfig() {
  const response = await axios.get('/api/config/git-repository')
  this.gitRepoType = response.data.repository_type
}

// 保存配置
async saveConfig() {
  await axios.put('/api/config/git-repository', {
    repository_type: this.gitRepoType
  })
  this.$message.success('配置保存成功')
}
```

## 4. 实施步骤

### 4.1 第一阶段：数据库和配置扩展
1. **扩展system_configs表**
   - 添加`git_repository_type`配置项
   - 设置默认值为`gitee`

2. **扩展配置服务**
   - 在ConfigHandler中添加Git仓库配置相关API
   - 实现配置的读取和更新功能

### 4.2 第二阶段：后端服务修改
1. **创建Git服务工厂**
   - 实现GitServiceFactory
   - 支持动态选择Git服务实例

2. **修改同步流程**
   - 在`processSyncTask()`中添加仓库类型检查
   - 根据配置选择对应的Git服务

3. **配置验证**
   - 添加配置项验证逻辑
   - 确保配置值的有效性

### 4.3 第三阶段：前端界面开发
1. **创建配置组件**
   - 设计Git仓库选择界面
   - 实现配置的读取和保存

2. **集成到系统设置**
   - 将配置组件集成到现有设置页面
   - 添加必要的权限控制

### 4.4 第四阶段：测试和验证
1. **功能测试**
   - 测试Gitee和GitHub仓库切换
   - 验证同步功能正常工作

2. **兼容性测试**
   - 确保现有功能不受影响
   - 验证模拟同步功能正常

## 5. 技术实现细节

### 5.1 配置管理

#### 5.1.1 配置读取缓存
```go
type ConfigCache struct {
    gitRepoType string
    lastUpdate  time.Time
    mutex       sync.RWMutex
}

func (c *ConfigCache) GetGitRepositoryType() string {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    
    // 缓存过期检查
    if time.Since(c.lastUpdate) > 5*time.Minute {
        c.refreshConfig()
    }
    
    return c.gitRepoType
}
```

#### 5.1.2 配置热更新
```go
func (c *ConfigCache) RefreshConfig() {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    var config models.SystemConfig
    database.DB.Where("config_key = ?", "git_repository_type").First(&config)
    
    c.gitRepoType = config.ConfigValue
    if c.gitRepoType == "" {
        c.gitRepoType = "gitee" // 默认值
    }
    
    c.lastUpdate = time.Now()
}
```

### 5.2 Git服务适配

#### 5.2.1 服务初始化
```go
type GitServiceManager struct {
    giteeService  *GitService
    githubService *GitService
    configCache   *ConfigCache
}

func NewGitServiceManager() *GitServiceManager {
    return &GitServiceManager{
        giteeService:  NewGiteeService(),
        githubService: NewGitHubGitService(),
        configCache:   NewConfigCache(),
    }
}

func (m *GitServiceManager) GetCurrentGitService() *GitService {
    repoType := m.configCache.GetGitRepositoryType()
    if repoType == "github" {
        return m.githubService
    }
    return m.giteeService
}
```

#### 5.2.2 GitHub Git服务实现
```go
func NewGitHubGitService() *GitService {
    return &GitService{
        RepoURL:     config.AppConfig.Git.GitHub.RepoURL,
        Username:    config.AppConfig.Git.GitHub.Username,
        Password:    config.AppConfig.Git.GitHub.Token, // 使用Token作为密码
        Email:       config.AppConfig.Git.GitHub.Email,
        LocalPath:   config.AppConfig.Git.LocalRepoPath + "_github",
    }
}
```

### 5.3 错误处理

#### 5.3.1 配置验证
```go
func ValidateGitRepositoryType(repoType string) error {
    validTypes := []string{"gitee", "github"}
    for _, valid := range validTypes {
        if repoType == valid {
            return nil
        }
    }
    return fmt.Errorf("invalid repository type: %s", repoType)
}
```

#### 5.3.2 服务降级
```go
func (m *GitServiceManager) GetGitServiceWithFallback() *GitService {
    repoType := m.configCache.GetGitRepositoryType()
    
    var service *GitService
    if repoType == "github" {
        service = m.githubService
    } else {
        service = m.giteeService
    }
    
    // 健康检查
    if !service.IsHealthy() {
        log.Warn("Primary git service unhealthy, falling back to gitee")
        return m.giteeService
    }
    
    return service
}
```

## 6. 风险评估与缓解

### 6.1 潜在风险
1. **配置错误导致同步失败**
   - 缓解：添加配置验证和默认值机制
   
2. **Git服务切换时的状态不一致**
   - 缓解：确保每个Git服务使用独立的本地路径
   
3. **现有功能受影响**
   - 缓解：保持向下兼容，默认使用Gitee

### 6.2 回滚方案
1. **数据库回滚**：删除新增的配置项
2. **代码回滚**：恢复到修改前的版本
3. **配置重置**：将git_repository_type设置为gitee

## 7. 测试计划

### 7.1 单元测试
- 配置服务测试
- Git服务工厂测试
- 配置验证测试

### 7.2 集成测试
- Gitee同步流程测试
- GitHub同步流程测试
- 配置切换测试

### 7.3 端到端测试
- 前端配置界面测试
- 完整同步流程测试
- 错误场景测试

## 8. 部署说明

### 8.1 数据库迁移
```sql
-- 添加Git仓库类型配置
INSERT INTO system_configs (config_key, config_value, description, created_at, updated_at) 
VALUES ('git_repository_type', 'gitee', 'Git仓库类型选择：gitee或github', NOW(), NOW());
```

### 8.2 配置文件更新
确保config.yaml中包含GitHub相关配置：
```yaml
git:
  github:
    repo_url: "https://github.com/username/repo.git"
    username: "github_username"
    token: "github_token"
    email: "github_email"
```

### 8.3 部署检查清单
- [ ] 数据库迁移脚本执行
- [ ] 配置文件更新
- [ ] GitHub仓库访问权限验证
- [ ] 前端界面功能验证
- [ ] 同步功能测试

## 9. 监控和维护

### 9.1 监控指标
- Git仓库切换频率
- 不同仓库的同步成功率
- 配置更新操作日志

### 9.2 日志记录
```go
log.Info("Git repository type changed", 
    "from", oldType, 
    "to", newType, 
    "user", userID)
```

### 9.3 性能考虑
- 配置缓存机制减少数据库查询
- Git服务实例复用避免重复初始化
- 异步配置更新避免阻塞请求

## 10. 总结

本设计方案通过最小化的代码修改，实现了Git仓库选择功能，主要特点：

1. **向下兼容**：默认使用Gitee，保持现有功能不变
2. **配置灵活**：支持动态切换，无需重启服务
3. **影响最小**：仅修改同步流程的入口点
4. **易于维护**：清晰的服务分层和配置管理

该方案能够满足用户需求，同时保证系统的稳定性和可维护性。