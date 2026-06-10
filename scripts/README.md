## init.sql

全库 DDL（含同步三表、RBAC、ACR）。新环境部署时先执行本文件，再启动应用。

# Scripts - 开发运维工具集

本目录包含了 Docker 镜像同步平台的开发环境启动、健康检查、数据备份和系统监控等实用脚本。

## 📋 目录结构

```
scripts/
├── README.md              # 本说明文档
├── backup.sh              # 数据备份脚本
├── dev.sh                 # 开发环境启动脚本
├── health-check.sh        # 服务健康检查脚本
└── monitor.sh             # 系统监控脚本
```

## 🚀 脚本使用说明

### 1. dev.sh - 开发环境启动脚本

一键启动完整的开发环境，包括数据库、后端和前端服务。

#### 功能特性
- ✅ 环境依赖检查（Go、Node.js、Docker）
- ✅ MySQL数据库自动启动（Docker容器或本地服务）
- ✅ 智能MySQL就绪检查
- ✅ Go后端服务启动
- ✅ Vue前端开发服务器启动
- ✅ 优雅的进程管理

#### 使用方法
```bash
# 进入项目根目录执行
./scripts/dev.sh
```

#### 环境要求
- Go 1.21+
- Node.js 18+
- Docker (可选，用于启动MySQL)
- MySQL 8.0+ (如果使用本地数据库)

---

### 2. health-check.sh - 服务健康检查脚本

全面检查Docker镜像同步平台各个组件的运行状态。

#### 检查项目
- 后端API服务 (http://localhost:8080)
- 镜像统计API接口
- 同步历史API接口
- 前端服务 (http://localhost:3000)
- 数据库连接状态
- MySQL Docker容器状态

#### 使用方法
```bash
./scripts/health-check.sh
```

#### 输出示例
```
Docker镜像同步平台 - 健康检查
================================
检查 后端健康检查... ✓ 正常 (HTTP 200)
检查 镜像统计API... ✓ 正常
检查 同步历史API... ✓ 正常
检查 前端服务... ✓ 正常 (HTTP 200)
检查数据库连接... ✓ 正常
检查MySQL容器... ✓ 运行中

健康检查完成
```

---

### 3. backup.sh - 数据备份脚本

自动化备份系统数据和配置文件，支持灵活的备份选项。

#### 备份内容
- **数据库备份**: MySQL完整数据备份
- **配置文件备份**: config.yaml 等配置文件
- **日志文件备份**: 应用日志文件
- **临时数据备份**: temp目录的临时文件

#### 使用方法
```bash
# 查看帮助
./scripts/backup.sh -h

# 完整备份
./scripts/backup.sh

# 仅备份数据库
./scripts/backup.sh -d

# 仅备份配置文件
./scripts/backup.sh -c

# 仅备份日志文件
./scripts/backup.sh -l

# 仅备份临时数据
./scripts/backup.sh -g
```

#### 备份文件位置
- 备份文件存储在 `backups/` 目录
- 文件命名格式: `docker_sync_backup_YYYYMMDD_HHMMSS_*.tar.gz`
- 自动清理7天前的旧备份

---

### 4. monitor.sh - 系统监控脚本

持续监控系统运行状态，提供实时告警和性能统计。

#### 监控功能
- 服务健康状态检查
- 业务数据统计（镜像同步数量、成功率等）
- 系统资源监控
- 异常告警通知
- 日志记录和分析

#### 使用方法
```bash
# 单次检查
./scripts/monitor.sh --once

# 后台持续监控
nohup ./scripts/monitor.sh &

# 查看监控日志
tail -f logs/monitor.log
```

#### 监控配置
- 检查间隔：60秒
- 告警阈值：连续5次失败
- 日志文件：`logs/monitor.log`

---

## ⚙️ 环境要求

### 基础要求
- **操作系统**: Windows/Linux/macOS
- **Shell环境**: Bash 或兼容的Shell环境
- **网络连接**: 可访问网络以下载依赖

### 工具依赖
- **curl**: HTTP请求工具（必需）
- **Docker**: 容器管理工具（用于数据库）
- **Go**: Go开发环境（后端运行）
- **Node.js**: Node.js开发环境（前端运行）

### 可选工具
- **jq**: JSON处理工具（优化显示效果）
- **mysqladmin**: MySQL管理工具（增强数据库检查）
- **telnet**: 网络连接测试工具

## 🔧 配置说明

### MySQL配置
脚本中使用的MySQL配置与 `config.yaml` 保持一致：
- 主机: 127.0.0.1
- 端口: 3306
- 用户名: root
- 密码: 123456
- 数据库: docker_sync

### Docker配置
- 容器名称: `docker-sync-mysql-dev`
- 镜像版本: `mysql:8.0`

### 服务端口
- 后端API: http://localhost:8080
- 前端服务: http://localhost:3000

## 🛠️ 故障排除

### 常见问题

#### 1. MySQL连接失败
```bash
# 检查MySQL服务状态
netstat -ano | findstr :3306

# 检查Docker容器
docker ps | grep mysql

# 检查配置文件
cat config.yaml | grep -A 10 database
```

#### 2. 端口占用冲突
```bash
# 查看端口占用
netstat -ano | findstr :8080  # 后端端口
netstat -ano | findstr :3000  # 前端端口
netstat -ano | findstr :3306  # 数据库端口

# 停止占用进程
tasklist | findstr :8080
taskkill /PID 进程ID /F
```

#### 3. 依赖安装失败
```bash
# Go环境
go version

# Node.js环境
npm --version

# Docker环境
docker --version
```

#### 4. 脚本权限问题
```bash
# Linux/macOS
chmod +x scripts/*.sh

# Windows (Git Bash)
chmod +x scripts/*.sh

# 或直接运行
bash scripts/dev.sh
```

### 调试技巧

#### 启用详细输出
```bash
bash -x scripts/dev.sh
```

#### 查看日志
```bash
# 应用日志
tail -f logs/app.log

# 监控日志
tail -f logs/monitor.log

# Docker容器日志
docker logs docker-sync-mysql-dev
```

## 📊 脚本状态矩阵

| 脚本名称 | 主要功能 | 适用环境 | 依赖工具 | 测试状态 |
|---------|----------|----------|----------|----------|
| dev.sh | 环境启动 | 开发环境 | Docker/Go/Node.js | ✅ |
| health-check.sh | 健康检查 | 所有环境 | curl | ✅ |
| backup.sh | 数据备份 | 所有环境 | tar | ✅ |
| monitor.sh | 系统监控 | 所有环境 | curl | ✅ |

## 🔄 更新日志

### v1.1.0 (2024-12-21)
- ✅ 优化dev.sh MySQL启动逻辑
- ✅ 修复backup.sh配置文件检查
- ✅ 增强monitor.sh兼容性
- ✅ 改进health-check.sh错误处理
- ✅ 添加Windows环境适配

### v1.0.0 (2024-12-19)
- ✅ 初始版本发布
- ✅ 基础功能实现

## 📞 技术支持

如果遇到脚本相关问题，请：

1. 查看本文档的故障排除部分
2. 检查脚本日志输出
3. 验证环境配置是否正确
4. 提供详细的错误信息和复现步骤

---

**注意**: 本脚本集专门为Docker镜像同步平台设计，使用前请确保了解各脚本的功能和限制。