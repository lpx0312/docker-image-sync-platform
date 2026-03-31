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
├── internal/                 # 后端内部包
│   ├── config/              # 配置管理
│   ├── database/            # 数据库连接与迁移
│   ├── handlers/            # HTTP 处理器
│   ├── middleware/          # 中间件（CORS、日志、限流等）
│   ├── models/              # 数据模型
│   ├── services/            # 业务逻辑
│   └── logger/              # 日志工具
├── web/                     # 前端 Vue 应用
│   ├── src/
│   │   ├── api/            # API 封装
│   │   ├── components/     # 组件
│   │   ├── router/         # 路由
│   │   ├── stores/         # Pinia
│   │   └── views/          # 页面
│   └── package.json
├── deploy/                  # Docker 部署相关
├── scripts/                 # 脚本
└── Makefile                # 构建与开发命令
```

## 核心服务

### 1. Git 服务工厂
- `internal/services/git_factory.go`
- 动态创建 Gitee/GitHub 的 Git 服务实例
- 负责克隆仓库与解析镜像清单文件

### 2. GitHub 服务
- `internal/services/github.go`
- 监控 GitHub Actions 工作流
- 查询运行状态与日志

### 3. 配置服务
- `internal/services/config.go`
- 加密存储配置
- 对系统设置做数据库 CRUD

### 4. 同步处理器
- `internal/handlers/sync.go`
- 处理镜像同步请求
- 支持单条与批量同步

### 5. Git 优化服务
- `internal/services/git_optimized.go`
- 带缓存与稀疏检出的 Git 操作
- 含 GitHub 代码操作测试：`PullImagesFileForTesting()`、`UpdateImagesFileForTesting()`

## API 结构

### 基础路径：`/api/v1`

### 同步
- `POST /sync/submit` — 提交单镜像同步
- `POST /sync/batch` — 批量同步
- `GET /sync/status/:taskId` — 查询任务状态
- `GET /sync/history` — 同步历史

### 镜像管理
- `GET /images/list` — 分页列表
- `GET /images/:id` — 详情
- `DELETE /images/:id` — 删除记录
- `POST /images/:id/retry` — 失败重试

### 配置
- `GET /config/all` — 全部配置
- `GET /config/git` — Git 相关配置
- `PUT /config/git/gitee` — 更新 Gitee
- `PUT /config/git/github` — 更新 GitHub
- `POST /config/git/test` — 测试 Git 连接
- `POST /config/git-test-operations` — 测试 GitHub 拉取与提交
- `PUT /config/aliyun-db` — 更新阿里云 ACR 配置

### GitHub
- `GET /github/runs` — 工作流运行列表
- `GET /github/runs/:runId` — 运行详情
- `GET /github/rate-limit` — API 速率限制

## 配置

### 主配置文件：`config.yaml`
主要段落：
- `server`：HTTP 服务
- `database`：MySQL
- `git`：Gitee/GitHub 仓库
- `aliyun`：阿里云镜像仓库
- `log`：日志
- `sync`：同步任务（超时、并发等）

### 安全说明
- 敏感信息（密码、Token）在库中加密存储
- API 限流
- CORS 与前端访问控制
- 请求日志与错误处理中间件

## 数据库模型

### 核心表
- `image_sync_records`：单条镜像同步记录
- `sync_tasks`：批量任务
- `system_configs`：加密配置

### 关键字段
- 同步状态（pending、running、success、failed）
- 与 GitHub Actions 的关联
- 多架构（amd64、arm64）
- 重试与优先级

## 前端主要页面与组件

### 页面
- `SyncView.vue` — 镜像同步
- `ImagesView.vue` — 镜像管理与状态
- `ConfigView.vue` — 系统配置
- `GitHubView.vue` — 工作流监控

### 重要组件
- `SingleSyncForm.vue` — 单镜像同步
- `BatchSyncForm.vue` — 批量同步
- `GitConfigForm.vue` — Git 配置与 GitHub 测试入口
- `GitTestResultDialog.vue` — 测试结果展示
- `AliyunConfigForm.vue` — ACR 配置

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

### 价值
- 校验凭据与权限
- 在正式同步前发现网络与认证问题
- 观察 Git 操作耗时
- 提高对集成的信心

## 本地开发测试方法

### 开发环境
- **操作系统**：Linux（常见发行版如 Ubuntu、Debian、Fedora 等均可）
- **Shell**：bash（本仓库脚本与文档均以 bash 为准）

### 服务启动

#### 前端（固定端口 3000）
```bash
cd web
npm install   # 首次需安装依赖
npm run dev
```

#### 后端（固定端口 8080）
```bash
go run main.go
```

### 开发注意

#### 日志
- **后端日志**：`logs/app.log`
- **查看**：开发时可持续 tail 该文件排查问题

#### 重启前后端前释放端口
- **前端**：占用 **3000** 时，先结束占用进程再启动 `npm run dev`
- **后端**：占用 **8080** 时，先结束占用进程再启动 `go run main.go`

在 **bash** 下可用（需已安装 `lsof`，多数桌面/服务器 Linux 自带或可 `sudo apt install lsof`）：

```bash
# 查看占用某端口的进程（将 3000 或 8080 换成实际端口）
lsof -iTCP:3000 -sTCP:LISTEN
# 或
ss -tlnp | grep ':3000'

# 按 PID 结束进程（将 <PID> 换成上一步看到的 PID）
kill <PID>
# 仍不退出时可强制：
kill -9 <PID>
```

一行结束监听某端口的进程（谨慎使用）：

```bash
kill $(lsof -t -i:3000)   # 前端
kill $(lsof -t -i:8080)   # 后端
```

#### 代码变更与重启
- 修改前端或后端代码后，一般需**重启对应服务**（按团队习惯也可同时重启两端）
- 以实际热更新能力为准；无热更新时改代码后应重启

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

1. **环境**：端口空闲，启动前后端
2. **开发**：按需求改前端或后端
3. **重启**：无热更新时重启对应服务
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
1. **数据库**：MySQL 是否运行、账号密码与 `config.yaml` 是否一致
2. **Git**：仓库地址、Token、分支与权限
3. **同步失败**：GitHub Actions 运行状态与日志
4. **前端构建**：Node 依赖是否安装完整
5. **端口冲突**：用 `lsof`、`ss` 等查看并结束占用进程

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
