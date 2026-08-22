# Docker镜像同步平台

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![Vue Version](https://img.shields.io/badge/Vue-3.0+-green.svg)](https://vuejs.org)

一个功能完整的Docker镜像自动化同步平台，专为解决国内访问Docker Hub等海外镜像仓库困难而设计。通过Web界面轻松管理镜像同步任务，自动将镜像同步到绑定的目标仓库——支持阿里云 ACR、华为云 SWR、腾讯云 CCR、Harbor 与通用 Registry（OCI/Docker v2）五种类型多实例混管，提供稳定可靠的镜像访问服务。

## ✨ 核心特性

### 🚀 自动化同步
- **一键同步**: 简单输入镜像名称即可启动同步流程
- **智能流程**: 后端 → workflow_dispatch → Actions → 目标镜像仓库全自动化（目标仓库地址/命名空间/凭据全部由后端随任务下发，Action 仓库不保存任何目标侧配置）
- **多源支持**: DockerHub、gcr.io、k8s.io、ghcr.io、quay.io等主流镜像仓库
- **架构兼容**: 支持AMD64、ARM64等多种CPU架构

### 🗂️ 多仓库管理
- **五种类型**: 阿里云 ACR / 华为云 SWR / 腾讯云 CCR / Harbor / 通用 Registry（OCI v2），同类型可配多个实例
- **别名标识**: 仓库以别名（ALIAS）区分与引用，避免各厂商命名空间同名冲突
- **连接测试**: 每个仓库配置支持一键测试（登录凭证 + SWR/CCR 管理面 AK/SK），配置错误即时暴露
- **仓库导入**: 支持从远程仓库拉取镜像列表导入台账（ACR/generic 走 `_catalog`，SWR 走管理面 API，CCR 走腾讯云 API）
- **配额感知**: ACR 个人版按 300 仓库配额参与目标路由，其余类型不限

### 📊 实时监控
- **状态跟踪**: 实时显示同步进度和状态
- **GitHub Actions集成**: 直接监控Actions工作流执行情况
- **失败重试**: 自动检测失败任务并支持一键重试
- **详细日志**: 完整的同步过程日志记录

### 💾 数据管理
- **持久化存储**: MySQL数据库存储所有同步记录
- **高级搜索**: 支持按状态、时间、镜像名等多维度筛选
- **批量操作**: 支持批量删除、重试等操作
- **数据导出**: 支持同步记录导出功能

### 🔐 认证与权限
- **用户认证**: JWT Token 认证，支持 Remember Me
- **角色控制**: 管理员 / 运维员 / 普通用户三级角色
- **权限管理**: 基于角色的访问控制（RBAC），细粒度权限分配
- **用户管理**: 管理员可创建用户、分配角色、重置密码
- **审计日志**: 完整的登录日志记录

### 🎯 用户体验
- **现代化界面**: 基于Vue 3 + Element Plus的响应式设计
- **操作简便**: 直观的用户界面，零学习成本
- **移动适配**: 完美支持移动端访问

## 🛠️ 技术架构

### 后端技术栈
| 技术 | 版本 | 用途 |
|------|------|------|
| **Go** | 1.21+ | 主要编程语言，高性能并发处理 |
| **Gin** | v1.9+ | 轻量级Web框架，提供RESTful API |
| **GORM** | v1.25+ | ORM框架，简化数据库操作 |
| **MySQL** | 8.0+ | 关系型数据库，存储同步记录 |
| **Viper** | v1.16+ | 配置管理，支持多种配置格式 |
| **Zap** | v1.24+ | 高性能日志框架 |
| **go-git** | v5.7+ | Git操作库，自动化代码提交 |

### 前端技术栈
| 技术 | 版本 | 用途 |
|------|------|------|
| **Vue 3** | 3.3+ | 现代化前端框架，组合式API |
| **Element Plus** | 2.3+ | 企业级UI组件库 |
| **JavaScript** | ES2020+ | 前端开发语言 |
| **Vite** | 4.4+ | 快速构建工具 |
| **Pinia** | 2.1+ | 状态管理，替代Vuex |
| **Vue Router** | 4.2+ | 单页面应用路由 |
| **Axios** | 1.6+ | HTTP 客户端 |

### 部署技术栈
| 技术 | 版本 | 用途 |
|------|------|------|
| **Docker** | 20.10+ | 容器化部署 |
| **Docker Compose** | 2.0+ | 多容器编排 |
| **Nginx** | 1.21+ | 反向代理和静态文件服务 |

## 📚 文档导航

### 📖 用户文档
- **[快速开始](#-快速开始)** - 快速部署和使用指南
- **[使用指南](#-使用指南)** - 详细的功能使用说明
- **[Swagger API文档](http://localhost:8080/api/v1/docs.html)** - 交互式在线接口文档
- **[Swagger使用说明](docs/SWAGGER使用说明.md)** - Swagger文档使用指南
- **[端到端测试指南](docs/e2e-test-guide.md)** - E2E 测试操作手册

### 🛠️ 运维文档
- **[All-in-One 部署](deploy/docker-all/README.md)** - 一体化 Docker 部署指南
- **[脚本工具集](scripts/README.md)** - 开发运维脚本说明
- **[监控告警](#监控和告警)** - 系统监控和告警配置

### 🔧 开发文档
- **[项目结构](#项目结构)** - 代码组织和架构说明
- **[技术架构](#-技术架构)** - 技术选型和架构设计
- **[开发环境](#-方式二本地开发环境)** - 本地开发环境搭建

## 🚀 快速开始

### 📋 前置要求

在开始之前，请确保您的系统已安装以下软件：

- **Docker** 20.10+ 和 **Docker Compose** 2.0+
- **Git** 2.30+
- **Node.js** 16+ 和 **npm** 8+ (开发环境)
- **Go** 1.21+ (开发环境)
- **MySQL** 8.0+ (如不使用Docker)

### 🎯 部署选项

本项目提供两种 Docker 部署方式：

| 部署方式 | 目录 | 特点 | 适用场景 |
|---------|------|------|----------|
| **分离式部署** | `deploy/docker-signal/` | 前后端分离，独立容器 | 开发环境、微服务架构 |
| **完整版部署** | `deploy/docker-all/` | 前后端一体，单容器 | 生产环境、简化部署 |

### 🐳 方式一：Docker一键部署（推荐）

这是最简单的部署方式，适合生产环境和快速体验。

1. **克隆项目**
```bash
git clone https://github.com/lpx0312/docker_image_pusher.git
cd docker-image-sync-platform
```

2. **配置环境变量**
```bash
# 复制环境变量模板
cp .env.example .env

# 编辑配置文件（必须配置）
nano .env
```

**重要配置项说明：**
```bash
# Git仓库配置（必填）
GITEE_USERNAME=your-gitee-username
GITEE_PASSWORD=your-gitee-password
GITHUB_USERNAME=your-github-username
GITHUB_TOKEN=your-github-token

# 阿里云ACR配置（默认兜底配置；多仓库实例请在 Web 端「系统配置 → 镜像仓库配置」中添加）
ALIYUN_REGISTRY=registry.cn-hangzhou.aliyuncs.com
ALIYUN_NAMESPACE=your-namespace
ALIYUN_USERNAME=your-aliyun-username
ALIYUN_PASSWORD=your-aliyun-password

# 数据库配置（可选，使用默认值）
MYSQL_ROOT_PASSWORD=your-secure-password
MYSQL_DATABASE=docker_sync
```

3. **一键启动**

**方式A：分离式部署（推荐）**
```bash
# Linux/macOS
cd deploy/docker-signal
chmod +x deploy.sh
./deploy.sh
```

**方式B：完整版部署（前端+后端一体）**
```bash
# 使用 Makefile（推荐）
make deploy-all-build
make deploy-all-up

# 或手动执行
cd deploy/docker-all
chmod +x build-all.sh
./build-all.sh
docker-compose -f docker-compose-all.yml up -d
```

4. **验证部署**
```bash
# 检查服务状态
docker-compose ps

# 查看日志
docker-compose logs -f
```

5. **访问应用**
- 🌐 **Web界面**: http://localhost
- 🔌 **API接口**: http://localhost:8080/api/v1
- 📊 **健康检查**: http://localhost:8080/api/v1/health
- 📖 **API文档**: http://localhost:8080/api/v1/docs.html

### 💻 方式二：本地开发环境

适合开发者进行功能开发和调试。

1. **安装依赖**
```bash
# 使用Makefile（推荐）
make deps

# 或手动安装
go mod tidy
cd web && npm install
```

2. **准备数据库**
```bash
# 启动MySQL（使用Docker）
docker run -d \
  --name mysql-dev \
  -e MYSQL_ROOT_PASSWORD=123456 \
  -e MYSQL_DATABASE=docker_sync \
  -p 3306:3306 \
  mysql:8.0

# 或使用现有MySQL实例
mysql -u root -p -e "CREATE DATABASE docker_sync;"
```

3. **配置文件**
复制并编辑配置文件：

```bash
cp config.yaml.example config.yaml
```

**配置文件示例：**
```yaml
# 服务器配置
server:
  host: "0.0.0.0"
  port: 8080
  mode: "debug"  # debug, release, test

# Git仓库配置
git:
  gitee:
    username: "your-gitee-username"
    password: "your-gitee-password"
    email: "your-email@example.com"
    repo_url: "https://gitee.com/your-username/docker_image_pusher.git"
  github:
    username: "your-github-username"
    token: "ghp_your-github-token"
    repo_url: "https://github.com/your-username/docker_image_pusher.git"

# 阿里云ACR配置（默认兜底；多类型仓库实例在 Web 端「镜像仓库配置」中管理）
aliyun:
  registry: "registry.cn-hangzhou.aliyuncs.com"
  namespace: "your-namespace"
  username: "your-aliyun-username"
  password: "your-aliyun-password"

# 数据库配置
database:
  host: "localhost"
  port: 3306
  username: "root"
  password: "123456"
  database: "docker_sync"
  charset: "utf8mb4"
  parse_time: true
  loc: "Local"

# 日志配置
log:
  level: "debug"
  file: "logs/app.log"
  max_size: 100
  max_backups: 3
  max_age: 28

# 认证配置
auth:
  jwt_secret: "your-jwt-secret-key"
  token_expiry: "24h"
  remember_me_expiry: "168h"
  auto_logout_minutes: 30
  default_admin_username: "admin"
  default_admin_password: "your-admin-password"
```

4. **启动开发环境**
```bash
# 方式1：使用脚本（推荐）
# Linux/macOS
chmod +x dev.sh
./dev.sh

# Windows PowerShell
.\dev.bat

# 方式2：使用Makefile
make dev

# 方式3：手动启动
# 终端1：启动后端
go run main.go

# 终端2：启动前端
cd web && npm run dev
```

5. **访问开发环境**
- 🌐 **前端开发服务器**: http://localhost:3000
- 🔌 **后端API服务器**: http://localhost:8080
- 📊 **API文档**: http://localhost:8080/api/v1/docs.html
- 🔑 **默认管理员**: admin / 密码见 `config.yaml` 中 `auth.default_admin_password`

## 📖 使用指南

### 🐳 镜像同步操作

#### 单个镜像同步
1. 访问Web界面，点击"同步镜像"
2. 输入源镜像地址，例如：
   - `nginx:latest`
   - `k8s.gcr.io/kube-state-metrics/kube-state-metrics:v2.0.0`
   - `ghcr.io/prometheus/prometheus:latest`
3. 选择目标架构（可选）：`linux/amd64`、`linux/arm64`
4. 设置目标标签（可选，默认使用源标签）
5. 添加描述信息（可选）
6. 点击"提交同步"按钮

#### 批量镜像同步
1. 点击"批量同步"标签页
2. 在文本框中输入多个镜像，每行一个：
```
nginx:latest
redis:7-alpine
mysql:8.0
```
3. 设置并发数量（建议1-3个）
4. 点击"批量提交"

### 📊 状态监控

#### 同步记录查看
- **镜像列表页面**：查看所有同步记录
- **状态筛选**：支持按状态筛选（待同步、同步中、成功、失败）
- **搜索功能**：支持按镜像名称搜索
- **详细信息**：点击记录查看详细同步信息

#### 实时状态更新
- 同步状态自动刷新（每30秒）
- GitHub Actions执行状态实时显示
- 失败任务支持一键重试

### 🔧 GitHub Actions集成

#### 工作流监控
- 查看GitHub Actions工作流运行状态
- 监控API使用限制和配额
- 查看账号 Actions 分钟数用量（已用/套餐包含/剩余估算）及仓库耗时排行（默认收起，展开后显示前 10，其余可继续展开）
- 查看详细的运行日志和错误信息

#### 自动化流程
1. **代码提交**：系统通过 Git API 更新 Action 仓库的`images.txt`文件
2. **任务分发**：系统调用 workflow_dispatch 触发 GitHub Actions，目标仓库地址、命名空间、用户名、密码作为 inputs 一并下发
3. **镜像同步**：GitHub Actions 按 dispatch 下发的目标执行镜像同步（仅推送到该目标，Action 仓库仅保留源仓库拉取凭据）
4. **状态回调**：同步完成后更新数据库状态

#### 在 GitHub 网页端查看
除本平台的 GitHub Actions 页面外，也可以直接在 GitHub 网页端查看运行状态与用量：

- **平台自身流水线**（镜像构建 docker-all / CLI 发布 cli-release）：`https://github.com/lpx0312/docker-image-sync-platform/actions`
- **镜像同步流水线**（Action 仓库）：`https://github.com/lpx0312/acr-images-sync-action/actions`
- **单次运行详情**：`https://github.com/<owner>/<repo>/actions/runs/<run_id>`——平台「运行详情」弹层中的 GitHub 链接即此地址，可查看每一步日志与耗时；运行列表页支持按 workflow/分支/状态筛选
- **账号用量与账单**：`https://github.com/settings/billing`（Settings → Billing & plans，Actions 分钟数明细按月按仓库列出；平台用量卡片的数据口径与此一致）
- **CLI 方式**：`gh run list --repo <owner>/<repo>` 查看运行列表，`gh run watch <run-id>` 实时跟踪，`gh api users/<user>/settings/billing/usage` 查询用量明细

## 运维管理

### 常用命令

```bash
# 查看服务状态
make help

# 启动开发环境
make dev

# 构建项目
make build

# 运行测试
make test

# 查看Docker日志
make docker-logs

# 停止服务
make docker-stop
```

### 健康检查

```bash


# 检查API健康状态
curl http://localhost:8080/api/v1/health
```

### 监控和告警

```bash
# 启动监控服务
./scripts/monitor.sh

# 后台运行监控
nohup ./scripts/monitor.sh > monitor.log 2>&1 &
```

### 数据备份

```bash
# 完整备份
./scripts/backup.sh

# 仅备份数据库
./scripts/backup.sh -d

# 仅备份配置文件
./scripts/backup.sh -c
```

### 日志管理

- 应用日志：`logs/app.log`
- 监控日志：`logs/monitor.log`
- Docker日志：`docker-compose logs`

### 故障排查

1. **服务无法启动**
   - 检查配置文件是否正确
   - 检查数据库连接
   - 查看日志文件

2. **同步失败**
   - 检查Git仓库配置
   - 检查GitHub Token权限
   - 查看GitHub Actions日志

3. **前端无法访问**
   - 检查Nginx配置
   - 检查前端构建是否成功
   - 检查端口是否被占用

## 项目结构

```
docker-image-sync-platform/
├── main.go                 # 主程序入口
├── config.yaml             # 配置文件
├── Makefile                # 构建脚本
├── internal/               # 内部包
│   ├── config/             # 配置管理
│   ├── database/           # 数据库操作
│   ├── handlers/           # HTTP处理器（sync、image、config、auth）
│   ├── logger/             # 日志管理
│   ├── middleware/         # 中间件（CORS、日志、限流、认证、权限）
│   ├── models/             # 数据模型（含用户、角色、权限）
│   ├── services/           # 业务服务（含认证、用户管理）
│   └── utils/              # 工具函数（ACR、Git URL解析等）
├── docs/                   # 文档目录
│   ├── swagger.json        # OpenAPI 2.0 规范文件
│   ├── swagger-ui.html     # Swagger UI 界面
│   ├── SWAGGER使用说明.md   # API文档使用指南
│   └── e2e-test-guide.md   # 端到端测试指南
├── deploy/                 # Docker部署配置
│   ├── docker-signal/      # 前后端分离部署
│   └── docker-all/         # 前后端一体部署
├── scripts/                # 运维脚本
├── logs/                   # 应用日志（运行时生成）
└── web/                    # 前端项目
    ├── src/
    │   ├── api/            # API接口层
    │   ├── components/     # 公共组件
    │   ├── router/         # 路由（含登录守卫与权限控制）
    │   ├── stores/         # 状态管理（Pinia，含认证状态）
    │   ├── styles/         # 设计令牌
    │   ├── utils/          # 工具函数
    │   └── views/          # 页面视图（含登录、用户管理）
    └── package.json
```

## 🤝 贡献指南

我们欢迎所有形式的贡献！无论是报告bug、提出新功能建议，还是提交代码改进。

### 如何贡献

1. **Fork** 本仓库
2. 创建您的特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交您的更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开一个 **Pull Request**

### 开发规范

- 遵循现有的代码风格
- 添加适当的测试用例
- 更新相关文档
- 确保所有测试通过

### 报告问题

如果您发现了bug或有功能建议，请：

1. 检查 [Issues](https://github.com/lpx0312/docker_image_pusher/issues) 中是否已有相关问题
2. 如果没有，请创建新的Issue，并提供：
   - 详细的问题描述
   - 重现步骤
   - 期望的行为
   - 系统环境信息

## 📋 常见问题 (FAQ)

### Q: 支持哪些镜像仓库？
A: 支持所有公开的Docker镜像仓库，包括：
- Docker Hub (docker.io)
- Google Container Registry (gcr.io)
- Kubernetes Registry (k8s.gcr.io)
- GitHub Container Registry (ghcr.io)
- Quay.io
- 其他符合OCI标准的镜像仓库

### Q: 支持同步到哪些目标仓库？
A: 阿里云 ACR、华为云 SWR、腾讯云 CCR（个人版）、Harbor、通用 Registry（OCI/Docker v2）五种类型，可在「镜像仓库配置」中添加多个实例并用别名区分；同步任务按亲和性/默认仓库自动路由，也可手动指定目标。

### Q: 同步失败怎么办？
A: 常见原因包括：
- 网络连接问题
- 认证配置错误（Git Token、阿里云 ACR 凭据）
- 镜像不存在或无权限访问
- GitHub Actions配额限制
请查看 `logs/app.log` 获取详细错误信息。

### Q: 如何配置自定义域名？
A: 修改 `deploy/docker-signal/nginx.conf` 或 `deploy/docker-all/nginx-all.conf` 中的 Nginx 配置。

### Q: 支持私有镜像仓库吗？
A: 目标仓库支持私有——Harbor 与通用 Registry 类型可对接自建私有仓库（Harbor 需公网有效 TLS 证书）；源镜像侧 Docker Hub / ghcr.io 的私有拉取凭证可在同步执行仓库的 vars/secrets 中配置。

### Q: 如何批量导入镜像列表？
A: 镜像管理页支持「从仓库导入」（远程拉取已有仓库列表）、「批量添加」（逐个远程校验存在性）、「从同步记录导入」三种方式建立台账；CLI 侧可用 `dsync repo import --acr <别名>`（从仓库导入）与 `dsync repo sync-records --acr <别名>`；同步侧可使用批量同步功能或 `dsync batch -f images.txt`。

## 🔄 更新日志

### v2.1.1 (2026-08)
- 🔄 Action 流水线目标配置全量改为后端 dispatch 下发：删除 Action 仓库目标侧 vars/secrets（原 HUAWEI_*）及 inputs 默认值
- 🔄 移除固定 SWR 附加推送（双写备份），同步仅推送到任务指定的目标仓库
- 🔄 Action 仓库保留/新增源镜像拉取凭据（Docker Hub / ghcr.io / quay.io，可选）

### v2.1.0 (2026-08)
- ✨ 多类型目标仓库：华为云 SWR（双凭证：登录凭证 + 管理面 AK/SK）、腾讯云 CCR、Harbor、通用 Registry
- ✨ 镜像仓库别名（ALIAS）体系：展示/选择/CLI 引用统一按别名，老数据自动迁移
- ✨ 仓库连接测试、从仓库导入镜像列表
- ✨ 同步流水线目标参数通用化；可选 SWR 附加推送（自动双写备份）
- ✨ 镜像管理（台账/Tag/架构/清理）全链路适配多类型仓库

### v2.0.0 (2024-01-15)
- ✨ 新增批量同步功能
- ✨ 新增GitHub Actions集成
- ✨ 新增实时状态监控
- 🐛 修复同步状态更新问题
- 📚 完善文档和部署指南

### v1.5.0 (2023-12-20)
- ✨ 新增多架构支持
- ✨ 新增API接口文档
- 🔧 优化同步性能
- 🐛 修复前端显示问题

### v1.0.0 (2023-11-01)
- 🎉 首个正式版本发布
- ✨ 基础镜像同步功能
- ✨ Web管理界面
- ✨ Docker一键部署

## 🌟 致谢

感谢以下开源项目和贡献者：

- [Gin](https://github.com/gin-gonic/gin) - 高性能的Go Web框架
- [Vue.js](https://github.com/vuejs/vue) - 渐进式JavaScript框架
- [Element Plus](https://github.com/element-plus/element-plus) - Vue 3组件库
- [GORM](https://github.com/go-gorm/gorm) - Go语言ORM库
- [Docker](https://www.docker.com/) - 容器化平台

特别感谢所有提交Issue和Pull Request的贡献者！

## 📞 联系我们

- **项目维护者**: [lpx0312](https://github.com/lpx0312)
- **邮箱**: support@example.com
- **QQ群**: 123456789
- **微信群**: 扫描下方二维码

## 🔗 相关链接

- **在线演示**: https://demo.docker-sync.com
- **Docker Hub**: https://hub.docker.com/r/lpx0312/docker-sync
- **阿里云镜像**: https://registry.cn-hangzhou.aliyuncs.com/lpx0312/docker-sync

## 仓库地址

- **GitHub**: https://github.com/lpx0312/docker_image_pusher
- **Gitee**: https://gitee.com/lpx03/docker_image_pusher

## 📄 许可证

本项目基于 [MIT License](LICENSE) 开源协议发布。

---

<div align="center">

**如果这个项目对您有帮助，请给我们一个 ⭐ Star！**

Made with ❤️ by [lpx0312](https://github.com/lpx0312)

</div>