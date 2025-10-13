# Docker 镜像同步平台 - 完整版构建指南

## 概述

本项目提供了一个完整的 Docker 镜像同步平台，包含前端 Vue.js 应用和后端 Go 服务。使用 `Dockerfile-all` 可以将前端和后端打包到一个镜像中，通过 nginx 提供统一的服务入口。

## 文件说明

### 核心文件
- `Dockerfile-all` - 多阶段构建文件，包含前端构建、后端构建和运行环境
- `nginx-all.conf` - nginx 配置文件，用于服务前端静态文件和代理后端 API
- `docker-compose-all.yml` - Docker Compose 配置文件，包含应用和数据库
- `.dockerignore` - Docker 构建忽略文件，优化构建性能

### 构建脚本
- `build-all.bat` - Windows 构建脚本
- `build-all.sh` - Linux/macOS 构建脚本

## 功能特性

### 🚀 多阶段构建
1. **前端构建阶段**: 使用 Node.js 18 构建 Vue.js 应用
2. **后端构建阶段**: 使用 Go 1.21 构建后端服务
3. **运行阶段**: 使用 nginx + supervisor 管理前后端服务

### 🇨🇳 国内镜像源优化
- **npm**: 使用淘宝镜像 (registry.npmmirror.com)
- **Go**: 使用七牛云代理 (goproxy.cn)
- 大幅提升国内构建速度

### 🔧 服务管理
- 使用 supervisor 管理 nginx 和后端服务
- 自动重启和日志管理
- 健康检查和监控

## 快速开始

### 前提条件
- Docker Desktop 已安装并运行
- 确保 Docker 服务正常启动

### 方法一：使用构建脚本

#### Windows
```bash
# 运行构建脚本
.\build-all.bat
```

#### Linux/macOS
```bash
# 给脚本执行权限
chmod +x build-all.sh

# 运行构建脚本
./build-all.sh
```

### 方法二：手动构建

```bash
# 1. 构建镜像
docker build -f Dockerfile-all -t docker-image-sync-platform:all .

# 2. 运行容器
docker run -d \
  -p 80:80 \
  -p 8080:8080 \
  --name docker-sync-all \
  docker-image-sync-platform:all
```

### 方法三：使用 Docker Compose

```bash
# 启动所有服务（包括数据库）
docker-compose -f docker-compose-all.yml up -d

# 查看服务状态
docker-compose -f docker-compose-all.yml ps

# 查看日志
docker-compose -f docker-compose-all.yml logs -f

# 停止服务
docker-compose -f docker-compose-all.yml down
```

## 访问地址

启动成功后，可以通过以下地址访问：

- **前端界面**: http://localhost
- **后端 API**: http://localhost:8080/api
- **健康检查**: http://localhost/health
- **nginx 状态**: http://localhost:8081/nginx_status

## 端口说明

| 端口 | 服务 | 说明 |
|------|------|------|
| 80 | nginx | 前端静态文件 + API 代理 |
| 8080 | 后端服务 | Go API 服务 |
| 8081 | nginx 状态 | 服务监控（可选） |
| 3306 | MySQL | 数据库服务 |

## 目录结构

```
/app/
├── main                 # 后端可执行文件
├── config.yaml         # 配置文件
├── sql/                # 数据库初始化脚本
└── logs/               # 日志目录

/usr/share/nginx/html/  # 前端静态文件
├── index.html
├── static/
└── ...
```

## 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| TZ | Asia/Shanghai | 时区设置 |
| GIN_MODE | release | Gin 运行模式 |
| GOPROXY | https://goproxy.cn,direct | Go 模块代理 |

## 日志管理

### 查看日志
```bash
# 查看所有日志
docker logs docker-sync-all

# 查看 nginx 日志
docker exec docker-sync-all tail -f /var/log/nginx/access.log

# 查看后端日志
docker exec docker-sync-all tail -f /var/log/supervisor/backend.out.log

# 查看 supervisor 日志
docker exec docker-sync-all tail -f /var/log/supervisor/supervisord.log
```

### 日志文件位置
- nginx 访问日志: `/var/log/nginx/access.log`
- nginx 错误日志: `/var/log/nginx/error.log`
- 后端服务日志: `/var/log/supervisor/backend.out.log`
- supervisor 日志: `/var/log/supervisor/supervisord.log`

## 健康检查

容器内置健康检查，每 30 秒检查一次服务状态：

```bash
# 查看健康状态
docker inspect --format='{{.State.Health.Status}}' docker-sync-all

# 查看健康检查历史
docker inspect --format='{{range .State.Health.Log}}{{.Output}}{{end}}' docker-sync-all
```

## 故障排除

### 常见问题

1. **Docker 服务未启动**
   ```
   ERROR: error during connect: Head "http://...": open //./pipe/dockerDesktopLinuxEngine: The system cannot find the file specified.
   ```
   **解决方案**: 启动 Docker Desktop

2. **端口被占用**
   ```
   Error starting userland proxy: listen tcp4 0.0.0.0:80: bind: address already in use
   ```
   **解决方案**: 修改端口映射或停止占用端口的服务

3. **构建失败**
   - 检查网络连接
   - 确认 Docker 有足够的磁盘空间
   - 查看构建日志定位具体错误

### 调试命令

```bash
# 进入容器
docker exec -it docker-sync-all sh

# 查看进程状态
docker exec docker-sync-all supervisorctl status

# 重启服务
docker exec docker-sync-all supervisorctl restart backend
docker exec docker-sync-all supervisorctl restart nginx

# 查看端口监听
docker exec docker-sync-all netstat -tlnp
```

## 性能优化

### 构建优化
- 使用 `.dockerignore` 减少构建上下文
- 多阶段构建减少最终镜像大小
- 使用国内镜像源提升下载速度

### 运行优化
- nginx 启用 gzip 压缩
- 静态资源缓存策略
- 连接池和缓冲区优化

## 更新和维护

### 更新应用
```bash
# 重新构建镜像
docker build -f Dockerfile-all -t docker-image-sync-platform:all .

# 停止旧容器
docker stop docker-sync-all
docker rm docker-sync-all

# 启动新容器
docker run -d -p 80:80 -p 8080:8080 --name docker-sync-all docker-image-sync-platform:all
```

### 数据备份
```bash
# 备份数据库
docker exec mysql mysqldump -u root -p docker_sync > backup.sql

# 备份应用数据
docker cp docker-sync-all:/app/data ./backup/
```

## 技术栈

- **前端**: Vue.js 3 + Vite + Element Plus
- **后端**: Go + Gin + GORM
- **数据库**: MySQL 8.0
- **Web 服务器**: nginx
- **进程管理**: supervisor
- **容器化**: Docker + Docker Compose

## 许可证

本项目采用 MIT 许可证，详见 LICENSE 文件。