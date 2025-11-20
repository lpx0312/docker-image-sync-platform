# Docker镜像同步平台 - All-in-One 部署

## 📋 概述

这个目录包含了Docker镜像同步平台的All-in-One部署配置，将前端和后端打包到一个Docker容器中，简化部署流程。

## 🚀 快速开始

### 1. 构建镜像

```bash
# 给构建脚本执行权限
chmod +x build-all.sh

# 构建镜像
./build-all.sh
```

### 2. 配置环境变量

```bash
# 复制环境变量模板
cp env-example .env

# 编辑配置文件
vim .env
```

### 3. 启动服务

选择以下任一方式：

#### 方式一：不包含MySQL（连接外部数据库）
```bash
docker-compose -f docker-compose-all.yml up -d
```

#### 方式二：包含MySQL（推荐用于测试）
```bash
docker-compose -f docker-compose-all-mysql.yml up -d
```

#### 方式三：直接运行镜像
```bash
docker run -d \
  -p 80:80 \
  -p 8080:8080 \
  --name docker-sync-all \
  docker-image-sync-platform:all-latest
```

## ⚙️ 配置说明

### Git仓库配置

#### 必填配置
- `GIT_REPOSITORY_TYPE`: Git仓库类型，可选值：`gitee` 或 `github`

#### Gitee配置（当使用Gitee时）
- `GITEE_REPO_URL`: Gitee仓库URL
- `GITEE_USERNAME`: Gitee用户名
- `GITEE_PASSWORD`: Gitee密码（可选，推荐使用Token）
- `GITEE_TOKEN`: Gitee访问令牌（推荐）
- `GITEE_EMAIL`: Gitee邮箱地址
- `GITEE_BRANCH`: 仓库分支（默认：main）

#### GitHub配置（当使用GitHub时）
- `GITHUB_REPO_URL`: GitHub仓库URL
- `GITHUB_USERNAME`: GitHub用户名
- `GITHUB_TOKEN`: GitHub Personal Access Token
- `GITHUB_EMAIL`: GitHub邮箱地址
- `GITHUB_BRANCH`: 仓库分支（默认：main）

### 数据库配置

#### 内置MySQL（使用docker-compose-all-mysql.yml）
- `MYSQL_ROOT_PASSWORD`: MySQL root密码
- `MYSQL_DATABASE`: 数据库名称
- `MYSQL_USER`: 数据库用户
- `MYSQL_PASSWORD`: 数据库密码

#### 外部MySQL（使用docker-compose-all.yml）
- `DB_HOST`: 数据库主机地址
- `DB_PORT`: 数据库端口
- `DB_USERNAME`: 数据库用户名
- `DB_PASSWORD`: 数据库密码
- `DB_DATABASE`: 数据库名称

### 阿里云配置
- `ALIYUN_REGISTRY`: 阿里云镜像仓库地址
- `ALIYUN_NAMESPACE`: 阿里云命名空间
- `ALIYUN_USERNAME`: 阿里云用户名
- `ALIYUN_PASSWORD`: 阿里云密码

## 🔧 更新说明

### v2.0.0 更新内容

1. **移除本地Git路径配置**
   - 删除了 `GITEE_LOCAL_PATH` 和 `GITHUB_LOCAL_PATH` 配置项
   - 现在完全使用API模式，无需本地Git仓库

2. **新增Git仓库类型配置**
   - 新增 `GIT_REPOSITORY_TYPE` 环境变量
   - 支持在Gitee和GitHub之间切换
   - 默认值：`github`

3. **配置文件优化**
   - 重新组织了环境变量配置结构
   - 增加了详细的配置说明和使用指南
   - 更新了部署脚本的提示信息

4. **API模式完全支持**
   - 所有Git操作通过API完成
   - 支持稀疏检出和网络质量检测
   - 无需在容器内维护本地Git仓库

## 🌐 访问地址

- **前端界面**: http://localhost
- **后端API**: http://localhost:8080/api
- **健康检查**: http://localhost/health
- **API文档**: http://localhost:8080/api/v1/config/all

## 📁 文件说明

- `Dockerfile-all`: 多阶段构建Docker文件
- `docker-compose-all.yml`: 不包含MySQL的部署配置
- `docker-compose-all-mysql.yml`: 包含MySQL的部署配置
- `env-example`: 环境变量配置模板
- `nginx-all.conf`: Nginx配置文件
- `supervisord.conf`: 进程管理配置
- `build-all.sh`: 镜像构建脚本
- `README.md`: 本文档

## 🛠️ 故障排除

### 常见问题

1. **Git连接失败**
   - 检查Token是否正确且有效
   - 确认仓库URL和分支名称正确
   - 验证网络连接

2. **数据库连接失败**
   - 检查数据库服务是否正常运行
   - 验证数据库配置信息
   - 确认网络连通性

3. **端口冲突**
   - 修改docker-compose文件中的端口映射
   - 停止占用端口的其他服务

### 日志查看

```bash
# 查看容器日志
docker logs docker-sync-all

# 实时查看日志
docker logs -f docker-sync-all

# 查看特定服务日志（使用docker-compose）
docker-compose logs -f app-all
```

## 📞 支持

如遇问题，请检查：
1. 环境变量配置是否正确
2. Git Token是否有足够权限
3. 网络连接是否正常
4. Docker服务是否正常运行