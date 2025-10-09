# Docker镜像同步平台

一个用于同步Docker镜像到阿里云ACR的Web平台，支持通过GitHub Actions自动化同步流程。

## 功能特性

- 🐳 支持多种Docker镜像源（DockerHub, gcr.io, k8s.io, ghcr.io等）
- 🚀 自动化同步流程（Gitee → GitHub → Actions → 阿里云ACR）
- 📊 实时监控同步状态
- 📝 同步记录管理
- 🎯 支持多架构镜像
- 💾 MySQL数据库存储

## 技术栈

### 后端
- Go 1.19+
- Gin Web框架
- GORM (MySQL)
- Viper配置管理
- Zap日志
- go-git Git操作

### 前端
- Vue 3
- Element Plus
- TypeScript
- Vite
- Pinia状态管理

## 快速开始

### 方式一：Docker部署（推荐）

1. **克隆项目**
```bash
git clone <repository-url>
cd docker-image-sync-platform
```

2. **配置环境变量**
```bash
cp .env.example .env
# 编辑 .env 文件，填入您的配置信息
```

3. **一键部署**
```bash
# Linux/macOS
./deploy.sh

# Windows
deploy.bat
```

4. **访问应用**
- 前端：http://localhost
- API：http://localhost:8080/api/v1

### 方式二：开发环境

1. **安装依赖**
```bash
# 使用Makefile
make deps

# 或手动安装
go mod tidy
cd web && npm install
```

2. **配置文件**
复制 `config.yaml` 并填入您的配置信息：

```yaml
git:
  gitee:
    username: "your-gitee-username"
    password: "your-gitee-password"
    email: "your-email@example.com"
  github:
    username: "your-github-username"
    token: "your-github-token"

aliyun:
  registry: "registry.cn-hangzhou.aliyuncs.com"
  namespace: "your-namespace"
  username: "your-aliyun-username"
  password: "your-aliyun-password"

database:
  host: "localhost"
  username: "root"
  password: "your-db-password"
  database: "docker_sync"
```

3. **启动开发环境**
```bash
# Linux/macOS
./dev.sh

# Windows
dev.bat

# 或使用Makefile
make dev
```

## 使用说明

### 镜像同步

1. 在同步页面输入源镜像地址（如：`nginx:latest`）
2. 设置目标标签（可选，默认使用源标签）
3. 添加描述信息（可选）
4. 点击"提交同步"按钮

### 查看状态

- 在镜像列表页面查看所有同步记录
- 支持按状态筛选（待同步、同步中、成功、失败）
- 点击记录可查看详细信息

### GitHub Actions监控

- 查看GitHub Actions工作流运行状态
- 监控API使用限制
- 查看详细的运行日志

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
# 检查所有服务状态
./scripts/health-check.sh

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
├── config.yaml            # 配置文件
├── docker-compose.yml     # Docker编排文件
├── Dockerfile             # Docker镜像构建文件
├── Makefile              # 构建脚本
├── deploy.sh             # 部署脚本
├── internal/             # 内部包
│   ├── config/          # 配置管理
│   ├── database/        # 数据库操作
│   ├── handlers/        # HTTP处理器
│   ├── logger/          # 日志管理
│   ├── middleware/      # 中间件
│   ├── models/          # 数据模型
│   └── services/        # 业务服务
├── scripts/             # 运维脚本
│   ├── health-check.sh  # 健康检查
│   ├── monitor.sh       # 监控脚本
│   └── backup.sh        # 备份脚本
├── sql/                 # SQL脚本
└── web/                 # 前端项目
    ├── src/
    │   ├── api/        # API接口
    │   ├── router/     # 路由
    │   ├── stores/     # 状态管理
    │   └── views/      # 页面
    └── package.json
```

## 仓库地址

- GitHub: https://github.com/lpx0312/docker_image_pusher
- Gitee: https://gitee.com/lpx03/docker_image_pusher

## 许可证

MIT License