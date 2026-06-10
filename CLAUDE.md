# CLAUDE.md

本文件为在本仓库中协作开发时向 Claude Code（claude.ai/code）提供的指引。

## 项目概览

本项目为 Docker 镜像同步平台：后端 Go、前端 Vue.js，通过 GitHub Actions 工作流，将 Docker Hub、GCR 等来源的镜像自动同步到阿里云容器镜像服务（ACR）。

## 架构概览

### 后端（Go）
- **框架**：Gin（REST API）
- **数据库**：MySQL，ORM 为 GORM
- **配置**：Viper
- **日志**：Zap 结构化日志
- **Git**：go-git，带缓存优化
- **镜像仓库**：go-containerregistry

### 前端（Vue.js）
- **框架**：Vue 3（Composition API）
- **UI**：Element Plus
- **状态**：Pinia
- **路由**：Vue Router
- **构建**：Vite
- **HTTP**：Axios

### 目录结构
```
docker-image-sync-platform/
├── main.go                    # 应用入口
├── config.yaml               # 配置文件
├── config.yaml.example       # 配置文件模板（不含敏感信息）
├── Makefile                  # 构建与开发命令
├── internal/                 # 后端内部包
│   ├── config/              # 配置管理
│   ├── database/            # 数据库连接与迁移
│   ├── handlers/            # HTTP 处理器（sync、image、config、auth）
│   ├── middleware/          # 中间件（CORS、日志、限流、认证、权限等）
│   ├── models/              # 数据模型（含用户、角色、权限）
│   ├── services/            # 业务逻辑（含认证、用户管理）
│   ├── logger/              # 日志工具
│   └── utils/               # 工具函数（ACR、Git URL 解析、镜像仓库等）
├── docs/                     # API 文档
│   ├── swagger.json         # OpenAPI 2.0 规范文件
│   ├── swagger-ui.html      # Swagger UI 界面
│   ├── SWAGGER使用说明.md    # 文档使用指南
│   ├── e2e-test-guide.md    # 端到端测试指南
│   └── README.md            # 文档中心索引
├── web/                     # 前端 Vue 应用
│   ├── src/
│   │   ├── api/            # API 封装
│   │   ├── components/     # 组件
│   │   ├── router/         # 路由（含登录守卫与权限控制）
│   │   ├── stores/         # Pinia（含 auth 状态）
│   │   ├── styles/         # 设计令牌（design tokens）
│   │   ├── utils/          # 工具函数（剪贴板、格式化、状态映射）
│   │   └── views/          # 页面（含登录、用户管理）
│   └── package.json
├── deploy/                  # Docker 部署相关
│   ├── docker-signal/      # 前后端分离部署
│   └── docker-all/         # 前后端一体部署
├── scripts/                 # 脚本
└── logs/                    # 应用日志（运行时生成）
```

## 核心服务

### 1. 认证与用户服务
- `internal/services/auth.go` — JWT 认证、登录/登出、Token 管理
- `internal/services/user.go` — 用户 CRUD、角色管理、密码重置
- `internal/handlers/auth.go` — 认证相关 HTTP 处理器
- `internal/middleware/auth.go` — JWT 认证中间件、角色/权限校验

### 2. Git 服务工厂
- `internal/services/git_factory.go`
- 动态创建 Gitee/GitHub 的 Git 服务实例
- 负责克隆仓库与解析镜像清单文件

### 3. GitHub 服务
- `internal/services/github.go`
- 监控 GitHub Actions 工作流
- 查询运行状态与日志

### 4. 配置服务
- `internal/services/config.go`
- 加密存储配置
- 对系统设置做数据库 CRUD

### 5. 同步处理器
- `internal/handlers/sync.go`
- 处理镜像同步请求
- 支持单条与批量同步

### 6. Git 优化服务
- `internal/services/git_optimized.go`
- 带缓存与稀疏检出的 Git 操作
- 含 GitHub 代码操作测试：`PullImagesFileForTesting()`、`UpdateImagesFileForTesting()`

### 7. Git API 与接口抽象
- `internal/services/git_api.go` — 基于 HTTP API 的 Git 操作实现
- `internal/services/git_interfaces.go` — Git 服务接口定义

### 8. 工具函数
- `internal/utils/acr.go` — ACR 镜像地址解析与构建
- `internal/utils/git_config.go` — Git 配置辅助
- `internal/utils/git_url.go` — Git URL 解析
- `internal/utils/registry.go` — 镜像仓库地址处理

## API 结构

### 基础路径：`/api/v1`

### Swagger API 文档
- **Swagger UI**：`http://localhost:8080/api/v1/docs.html`
- **OpenAPI JSON**：`http://localhost:8080/api/v1/swagger.json`
- **静态资源目录**：`http://localhost:8080/api/v1/docs/`
- **使用说明**：`docs/SWAGGER使用说明.md`

### 认证 (Auth) — 公开接口
- `POST /auth/login` — 用户登录（返回 JWT Token）

### 认证 (Auth) — 需登录
- `GET /auth/me` — 获取当前用户信息（含 permissions、role_id、role_name）
- `PUT /auth/password` — 修改密码
- `POST /auth/logout` — 登出
- `GET /auth/roles/options` — 角色下拉选项（创建用户用）

### 用户管理 (Auth) — 需 `users` 权限
- `GET /auth/login-logs` — 登录日志
- `GET /auth/users` — 用户列表
- `POST /auth/users` — 创建用户（body 使用 `role_id`）
- `PUT /auth/users/:id/status` — 更新用户状态（启用/禁用）
- `PUT /auth/users/:id/role` — 更新用户角色（body 使用 `role_id`）
- `DELETE /auth/users/:id` — 删除用户
- `PUT /auth/users/:id/password` — 重置用户密码

### 角色管理 (Auth) — 需 `roles` 权限
- `GET /auth/permissions` — 全部权限列表
- `GET /auth/roles` — 角色列表（含权限与用户数）
- `POST /auth/roles` — 创建自定义角色
- `GET /auth/roles/:id` — 角色详情
- `PUT /auth/roles/:id` — 更新角色名称/描述/权限
- `DELETE /auth/roles/:id` — 删除角色（内置角色不可删）

### 同步 (Sync) — 需登录
- `POST /sync/submit` — 提交单镜像同步（同步限流）
- `POST /sync/batch` — 批量同步（同步限流）
- `POST /sync/batch/mock` — 模拟批量同步（测试用，同步限流）
- `GET /sync/status/:taskId` — 查询单个任务状态
- `GET /sync/status/:taskId` — 查询任务状态（含 workflow_runs 聚合）
- `GET /sync/history` — 同步历史

### 镜像管理 (Images) — 需登录
- `GET /images/list` — 分页列表（支持搜索、状态与架构筛选）
- `GET /images/stats` — 各状态数量统计
- `GET /images/:id` — 详情
- `DELETE /images/:id` — 删除记录（软删除）
- `POST /images/:id/retry` — 失败重试
- `POST /images/:id/check` — 检查单个镜像是否存在于 ACR
- `POST /images/batch-check` — 批量检查镜像是否存在于 ACR

### 配置 (Config) — 需登录 + `config` 权限
- `GET /config/status` — 配置状态（不含敏感信息）
- `GET /config/all` — 全部配置
- `GET /config/debug/:key` — 调试：获取指定配置项（仅非 release 模式）
- `GET /config/git-repository` — 获取当前 Git 仓库类型
- `PUT /config/git-repository` — 切换 Git 仓库类型（gitee/github）
- `GET /config/git` — Git 详细配置
- `PUT /config/git/gitee` — 更新 Gitee
- `PUT /config/git/github` — 更新 GitHub
- `POST /config/git/test` — 测试 Git 连接
- `POST /config/git-test-operations` — 测试 GitHub 拉取与提交（三步完整测试）
- `GET /config/git-optimization` — 获取 Git 优化配置
- `PUT /config/git-optimization` — 更新 Git 优化配置
- `GET /config/git-performance` — Git 性能指标
- `GET /config/git-network-test` — Git 网络质量测试
- `GET /acr-registries` — ACR 实例列表
- `POST /acr-registries` — 创建 ACR
- `POST /acr-registries/test` — 测试 ACR 连接

### GitHub — 需登录 + `github` 权限
- `GET /github/runs` — 工作流运行列表
- `GET /github/runs/:runId` — 运行详情
- `GET /github/rate-limit` — API 速率限制

### 健康检查 — 公开
- `GET /health` — 系统健康检查

## 配置

### 主配置文件：`config.yaml`
主要段落：
- `server`：HTTP 服务
- `database`：MySQL
- `git`：Gitee/GitHub 仓库及优化参数（操作模式、稀疏检出、缓存等）
- `aliyun`：可选，仅空库时 bootstrap 首条 `acr_registries`
- `log`：日志
- `sync`：同步任务（超时、并发、重试等）
- `auth`：JWT 认证（密钥、Token 有效期、自动登出、默认管理员账号）
- `github_actions`：工作流文件名和状态检查间隔
- `security`：限流与 CORS

### 安全说明
- JWT Token 认证，支持 Remember Me 长效 Token
- 基于角色的访问控制（RBAC）：管理员 / 运维员 / 普通用户
- 敏感信息（密码、Token）在库中加密存储
- API 限流
- CORS 与前端访问控制
- 请求日志与错误处理中间件
- 登录日志审计

## 数据库模型

### 核心表（schema 见 `scripts/init.sql`）
- `sync_batches`：同步批次头表
- `sync_records`：镜像同步明细
- `sync_workflow_runs`：批次×ACR 的 GitHub Actions 记录
- `acr_registries` / `acr_repositories`：ACR 配置与仓库台账
- `system_configs`：Git 等加密配置
- `users`：用户账号（用户名、密码哈希、邮箱、role_id、状态）
- `roles`：角色定义（code、name、is_system）
- `permissions`：权限注册表（code、name）
- `role_permissions`：角色与权限多对多关联
- `login_logs`：登录日志审计（用户、IP、User-Agent、状态）

### 关键字段
- 同步状态（pending、syncing、success、failed）
- 与 GitHub Actions 的关联
- 多架构（amd64、arm64）
- 重试与优先级

### 角色与权限（数据库驱动 RBAC）

- 权限在 `permissions` 表注册，启动时 seed；角色在 `roles` 表管理，支持自定义 CRUD
- 内置角色（不可删除）：**admin**、**operator**、**user**
- 默认权限映射：admin → 全部；operator → sync/images/github/config；user → sync（不含镜像管理）
- 权限标识：`PermSync`、`PermImages`、`PermGitHub`、`PermConfig`、`PermUsers`、`PermRoles`
- 用户通过 `users.role_id` 关联角色，登录/`/auth/me` 返回 `permissions` 数组
- 新增功能模块时：在 seed 注册 permission → 路由加 `PermissionRequired` → 前端 router/App.vue 加菜单 → 管理员在「角色管理」分配权限

### 新增菜单标准流程

1. 后端：在 `DefaultPermissionSeeds` 注册新 permission code；路由加 `PermissionRequired`
2. 前端：在 `web/src/constants/permissions.js`、router、App.vue 各加一处
3. 运营：管理员在「角色管理」界面给相关角色勾选新权限

## 前端主要页面与组件

### 页面
- `LoginView.vue` — 登录页面
- `SyncView.vue` — 镜像同步
- `ImagesManageView.vue` — 镜像台账管理
- `ConfigView.vue` — Git 与 ACR 配置
- `GitHubView.vue` — 工作流监控
- `UserManageView.vue` — 用户管理（需 users 权限）
- `RoleManageView.vue` — 角色管理（需 roles 权限）

### 重要组件
- `SingleSyncForm.vue` — 单镜像同步
- `BatchSyncForm.vue` — 批量同步
- `GitConfigForm.vue` — Git 配置与 GitHub 测试入口
- `GitTestResultDialog.vue` — 测试结果展示
- `AcrRegistryConfigForm.vue` — ACR 实例管理
- `ChangePasswordDialog.vue` — 修改密码对话框

### 状态管理
- `stores/auth.js` — 认证状态（登录、登出、Token、用户信息、权限）
- `stores/sync.js` — 同步状态
- `stores/image.js` — 镜像状态

### 路由守卫
- 未登录自动跳转登录页
- 基于角色权限的路由守卫（`requiredPermission`）
- 路由：`/login`、`/sync`、`/github`、`/config`、`/users`、`/roles`

## GitHub 代码操作测试功能

### 说明
在正式用于镜像同步前，可通过该功能校验 GitHub 配置是否可用。

### 能力
- **三步流程**：从仓库拉取 `images.txt` → 写入测试内容并提交 → 推送并校验
- **耗时**：记录 pull、commit、push 各步耗时
- **错误信息**：便于排查
- **界面**：Vue 弹窗分步展示结果

### 接口
- **URL**：`POST /api/v1/config/git-test-operations`
- **请求体示例**：
  ```json
  {
    "repo_url": "https://github.com/username/repository.git",
    "username": "github-username",
    "token": "github-personal-access-token",
    "email": "user@example.com",
    "branch": "main"
  }
  ```
- **响应示例**：
  ```json
  {
    "success": true,
    "message": "GitHub代码操作测试部分失败",
    "data": {
      "pull_success": false,
      "pull_time": 21070,
      "commit_success": false,
      "commit_time": 0,
      "push_success": false,
      "push_time": 0,
      "test_images_txt": false,
      "total_time": 21071,
      "commit_sha": "",
      "error_message": "Detailed error information..."
    }
  }
  ```

### 前端
- **GitConfigForm.vue**：GitHub 配置区含「测试代码拉取和提交」按钮
- **GitTestResultDialog.vue**：整体状态、各步耗时、错误与排查提示、成功时的 commit SHA 与 GitHub 链接、表格统计等

### 实现要点
- **后端**：测试目录如 `/tmp/git-test-operations`
- **Git**：真实 clone、commit、push
- **错误处理**：尽量给出可读说明
- **安全**：隔离临时环境，不影响生产数据

### 使用步骤
1. 打开 Web 端「配置」页
2. 填写 GitHub 仓库 URL、用户名、Token、邮箱、分支等
3. 在 GitHub 配置区点击「测试代码拉取和提交」
4. 在弹窗中查看各步结果
5. 全部通过后再用于实际同步

## 本地开发测试方法

### 开发环境
- **操作系统**：Ubuntu 24.04 LTS（x86_64）— 内核 6.17
- **Shell**：bash
- **Go**：1.26.1 linux/amd64（通过 snap 安装）
- **Node.js**：v24.14.1
- **npm**：11.11.0
- **Docker**：29.3.1（需 `sudo` 执行或将当前用户加入 docker 组）
- **Docker Compose**：v5.1.1
- **MySQL**：通过 Docker 容器运行（`docker-sync-mysql-dev`），映射端口 3306

### 服务启动

#### 准备 MySQL（如本机未安装 MySQL）
```bash
# 使用 Docker 启动 MySQL 8.0（首次需拉取镜像）
sudo docker run -d \
  --name docker-sync-mysql-dev \
  -p 3306:3306 \
  -e MYSQL_ROOT_PASSWORD=Ab123456 \
  -e MYSQL_DATABASE=docker_sync \
  mysql:8.0 --default-authentication-plugin=mysql_native_password

# 确认端口已监听
ss -ltnp | grep ':3306'
```

#### 后端（固定端口 8080）
```bash
go run main.go
```

#### 前端（固定端口 3000）
```bash
cd web
npm install   # 首次需安装依赖
npm run dev
```

### 访问地址
- **前端**：http://localhost:3000
- **后端 API**：http://localhost:8080/api/v1
- **Swagger UI**：http://localhost:8080/api/v1/docs.html
- **Swagger JSON**：http://localhost:8080/api/v1/swagger.json
- **健康检查**：http://localhost:8080/api/v1/health

### 开发注意

#### 日志
- **后端日志**：`logs/app.log`
- **查看**：`tail -f logs/app.log`

#### 重启前后端前释放端口
- **前端**：占用 **3000** 时，先结束占用进程再启动 `npm run dev`
- **后端**：占用 **8080** 时，先结束占用进程再启动 `go run main.go`

```bash
# 查看占用某端口的进程
ss -tlnp | grep ':3000'
# 或使用 lsof（需安装：sudo apt install lsof）
lsof -iTCP:3000 -sTCP:LISTEN

# 按 PID 结束进程
kill <PID>
# 仍不退出时可强制：
kill -9 <PID>

# 一行结束监听某端口的进程（谨慎使用）
kill $(lsof -t -i:3000)   # 前端
kill $(lsof -t -i:8080)   # 后端
```

#### MySQL 管理（Docker 方式）
```bash
# 查看容器状态
sudo docker ps --filter name=mysql

# 查看 MySQL 日志
sudo docker logs docker-sync-mysql-dev --tail 50

# 进入 MySQL 命令行
sudo docker exec -it docker-sync-mysql-dev mysql -uroot -pAb123456

# 停止 / 启动 / 重启
sudo docker stop docker-sync-mysql-dev
sudo docker start docker-sync-mysql-dev
sudo docker restart docker-sync-mysql-dev
```

#### 代码变更与重启
- 修改前端或后端代码后，需**重启对应服务**
- 前端 Vite 有热更新能力，大多数修改无需手动重启
- 后端 Go 无热更新，修改代码后必须重新 `go run main.go`

### 自动化测试

#### Chrome DevTools
可用 Chrome DevTools MCP 做页面与接口相关自动化：
- 点击、输入、导航等
- 配置类表单的完整性校验
- 前后端联调验证
- 界面是否按预期渲染

通过 Chrome DevTools MCP 生成的文件请放在项目根目录下的 `ChromeDevTools-Files/`（若不存在请先创建）。

#### 功能位置
若不确定某功能在界面何处，可直接询问维护者。

#### 常见测试场景
1. **配置**：Git 保存、校验、切换
2. **同步**：单条与批量
3. **状态**：进度与结果
4. **异常**：错误提示是否合理

### 开发工作流建议

1. **环境**：确认 MySQL 容器运行中、端口空闲，启动前后端
2. **开发**：按需求改前端或后端
3. **重启**：后端改代码后重启 `go run main.go`；前端 Vite 通常自动热更新
4. **验证**：必要时用 Chrome DevTools MCP 等工具自测
5. **确认**：功能与日志无异常

## 部署

### Docker 方式
1. **前后端分离**：`deploy/docker-signal/`
   - 前后端各一容器，适合开发与微服务拆分

2. **一体化**：`deploy/docker-all/`
   - 单容器内前后端，便于简化生产部署

### 环境变量
- 数据库连接
- Git 仓库凭据
- 阿里云镜像配置
- 服务与日志相关项

## 故障排查

### 常见问题
1. **数据库**：MySQL 容器是否运行（`sudo docker ps`）、账号密码与 `config.yaml` 是否一致
2. **Git**：仓库地址、Token、分支与权限
3. **同步失败**：GitHub Actions 运行状态与日志
4. **前端构建**：Node 依赖是否安装完整（`cd web && npm install`）
5. **端口冲突**：用 `ss -tlnp` 查看并结束占用进程
6. **Docker 权限**：命令需加 `sudo`，或将用户加入 docker 组（`sudo usermod -aG docker $USER`）

### 健康检查与日志
- 健康检查接口：`/api/v1/health`
- 服务状态：`make health-check`
- 日志：`logs/app.log` 或 `make docker-logs`

## 安全说明

- 同步相关接口限流
- 敏感配置加密存储
- CORS 配置
- 请求日志与监控
- 输入校验与清理

## 使用中文回答和询问
