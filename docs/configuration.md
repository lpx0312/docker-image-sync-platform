# 配置说明文档

## 概述

Docker镜像同步平台支持多种配置方式，配置优先级如下：

```
环境变量 > .env文件 > config.yaml > 默认值
```

## 配置文件

### 1. 环境变量配置 (.env)

环境变量配置具有最高优先级，适用于生产环境和容器化部署。

**创建配置文件：**
```bash
cp .env.example .env
```

**主要配置项：**

#### 数据库配置
```bash
# MySQL配置
MYSQL_ROOT_PASSWORD=root123456      # MySQL root密码
MYSQL_DATABASE=docker_sync          # 数据库名称
MYSQL_USER=docker_sync              # 应用数据库用户
MYSQL_PASSWORD=sync123456           # 应用数据库密码
MYSQL_PORT=3306                     # 数据库端口
```

#### 应用配置
```bash
# 运行模式
GIN_MODE=release                    # debug/release/test
LOG_LEVEL=info                      # debug/info/warn/error
APP_ENV=production                  # development/staging/production

# 网络配置
APP_PORT=8080                       # 后端服务端口
FRONTEND_PORT=80                    # 前端服务端口
SERVER_HOST=0.0.0.0                # 监听地址
```

#### Git仓库配置
```bash
# Gitee配置
GITEE_REPO_URL=https://gitee.com/username/repo.git
GITEE_USERNAME=your-username
GITEE_PASSWORD=your-password-or-token
GITEE_EMAIL=your-email@example.com

# GitHub配置
GITHUB_REPO_URL=https://github.com/username/repo.git
GITHUB_USERNAME=your-username
GITHUB_TOKEN=REDACTED_GITHUB_TOKEN
```

#### 阿里云ACR配置
```bash
# 阿里云容器镜像服务
ALIYUN_REGISTRY=registry.cn-hangzhou.aliyuncs.com
ALIYUN_NAMESPACE=your-namespace
ALIYUN_USERNAME=your-aliyun-username
ALIYUN_PASSWORD=your-aliyun-password
```

### 2. YAML配置文件 (config.yaml)

YAML配置文件提供了更详细的配置选项，适用于开发环境。

#### 服务器配置
```yaml
server:
  port: 8080              # 服务端口
  mode: debug             # 运行模式：debug/release/test
  host: "0.0.0.0"         # 监听地址
```

#### 数据库配置
```yaml
database:
  host: "localhost"       # 数据库地址
  port: 3306             # 数据库端口
  username: "root"       # 数据库用户名
  password: "password"   # 数据库密码
  database: "docker_sync" # 数据库名称
  charset: "utf8mb4"     # 字符集
  parse_time: true       # 时间解析
  loc: "Local"           # 时区设置
  
  # 连接池配置
  max_idle_conns: 10     # 最大空闲连接数
  max_open_conns: 100    # 最大打开连接数
  conn_max_lifetime: 3600 # 连接最大生存时间（秒）
```

#### Git仓库配置
```yaml
git:
  gitee:
    repo_url: "https://gitee.com/username/repo.git"
    username: "your-username"
    password: "your-password"
    email: "your-email@example.com"
  
  github:
    repo_url: "https://github.com/username/repo"
    username: "your-username"
    token: "github_pat_xxxxx"
  
  # 本地仓库路径配置已移至各自的git平台配置下
# gitee.local_path 和 github.local_path
```

#### 阿里云配置
```yaml
aliyun:
  registry: "registry.cn-hangzhou.aliyuncs.com"
  namespace: "your-namespace"
  username: "your-username"
  password: "your-password"
```

#### 日志配置
```yaml
log:
  level: "info"              # 日志级别
  file_path: "./logs/app.log" # 日志文件路径
  max_size: 100              # 单文件最大大小(MB)
  max_backups: 3             # 保留文件数量
  max_age: 28                # 保留天数
  compress: true             # 是否压缩
```

#### 同步任务配置
```yaml
sync:
  timeout_minutes: 30        # 任务超时时间（分钟）
  max_concurrent_jobs: 3     # 最大并发任务数
  max_retry_count: 3         # 最大重试次数
  retry_interval_minutes: 5  # 重试间隔（分钟）
```

#### GitHub Actions配置
```yaml
github_actions:
  workflow_file: "docker.yaml"           # 工作流文件名
  api_timeout_seconds: 30                # API超时时间
  status_check_interval_seconds: 60      # 状态检查间隔
```

#### 安全配置
```yaml
security:
  rate_limit:
    requests_per_minute: 100  # 每分钟最大请求数
    burst: 10                 # 突发请求数
  
  cors:
    allowed_origins: ["*"]    # 允许的源域名
    allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
    allowed_headers: ["*"]    # 允许的请求头
```

## 配置最佳实践

### 1. 安全配置

#### 密码和令牌管理
- **生产环境**：使用环境变量或密钥管理系统
- **开发环境**：使用 `.env` 文件，确保不提交到版本控制
- **强密码策略**：使用包含大小写字母、数字和特殊字符的复杂密码

#### Git访问令牌
```bash
# Gitee访问令牌获取
1. 登录Gitee -> 设置 -> 私人令牌
2. 创建令牌，选择所需权限
3. 复制令牌到配置文件

# GitHub Personal Access Token获取
1. 登录GitHub -> Settings -> Developer settings -> Personal access tokens
2. 创建新令牌，选择 repo 和 workflow 权限
3. 复制令牌到配置文件
```

### 2. 性能优化

#### 数据库连接池
```yaml
database:
  max_idle_conns: 10     # 根据并发需求调整
  max_open_conns: 100    # 不超过数据库最大连接数
  conn_max_lifetime: 3600 # 避免长时间连接
```

#### 同步任务配置
```yaml
sync:
  max_concurrent_jobs: 3  # 根据服务器性能调整
  timeout_minutes: 30     # 根据网络环境调整
```

### 3. 日志管理

#### 日志级别选择
- **开发环境**：`debug` - 详细调试信息
- **测试环境**：`info` - 一般业务信息
- **生产环境**：`warn` 或 `error` - 减少日志量

#### 日志轮转配置
```yaml
log:
  max_size: 100      # 根据磁盘空间调整
  max_backups: 3     # 保留足够的历史日志
  max_age: 28        # 根据合规要求调整
  compress: true     # 节省磁盘空间
```

### 4. 网络配置

#### 端口配置
```bash
# 确保端口未被占用
netstat -tlnp | grep :8080

# 防火墙配置（Linux）
sudo ufw allow 8080
sudo ufw allow 80
```

#### 跨域配置
```yaml
security:
  cors:
    allowed_origins: ["https://yourdomain.com"]  # 生产环境指定具体域名
    allowed_methods: ["GET", "POST", "PUT", "DELETE"]
```

## 环境特定配置

### 开发环境
```yaml
server:
  mode: debug
  port: 8080

log:
  level: debug
  
sync:
  max_concurrent_jobs: 1
```

### 生产环境
```yaml
server:
  mode: release
  port: 8080

log:
  level: info
  
sync:
  max_concurrent_jobs: 5
  
security:
  rate_limit:
    requests_per_minute: 1000
```

## 故障排除

### 常见配置问题

#### 1. 数据库连接失败
```bash
# 检查数据库服务状态
systemctl status mysql

# 检查网络连通性
telnet database_host 3306

# 检查用户权限
mysql -u username -p -h host
```

#### 2. Git仓库访问失败
```bash
# 测试Git连接
git clone https://gitee.com/username/repo.git

# 检查访问令牌权限
curl -H "Authorization: token YOUR_TOKEN" https://api.github.com/user
```

#### 3. 阿里云ACR访问失败
```bash
# 测试Docker登录
docker login registry.cn-hangzhou.aliyuncs.com

# 检查命名空间
docker images | grep registry.cn-hangzhou.aliyuncs.com/namespace
```

### 配置验证

#### 启动前检查清单
- [ ] 数据库连接配置正确
- [ ] Git仓库访问权限正常
- [ ] 阿里云ACR认证信息有效
- [ ] 端口未被占用
- [ ] 日志目录可写
- [ ] 临时目录可写

#### 配置测试命令
```bash
# 测试配置文件语法
go run main.go --config-test

# 验证数据库连接
go run main.go --db-test

# 验证Git仓库访问
go run main.go --git-test
```

## 配置模板

### Docker Compose环境变量
```yaml
# docker-compose.yml
version: '3.8'
services:
  app:
    environment:
      - MYSQL_HOST=mysql
      - MYSQL_PORT=3306
      - MYSQL_USER=${MYSQL_USER}
      - MYSQL_PASSWORD=${MYSQL_PASSWORD}
      - GIN_MODE=release
```

### Kubernetes ConfigMap
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  config.yaml: |
    server:
      port: 8080
      mode: release
    database:
      host: mysql-service
      port: 3306
```

### 系统服务配置
```ini
# /etc/systemd/system/docker-sync.service
[Unit]
Description=Docker Image Sync Platform
After=network.target

[Service]
Type=simple
User=app
WorkingDirectory=/opt/docker-sync
ExecStart=/opt/docker-sync/main
EnvironmentFile=/opt/docker-sync/.env
Restart=always

[Install]
WantedBy=multi-user.target
```

## 配置更新

### 热重载配置
某些配置项支持热重载，无需重启服务：
- 日志级别
- 同步任务参数
- 安全策略

### 配置变更流程
1. 备份当前配置
2. 修改配置文件
3. 验证配置语法
4. 重启服务（如需要）
5. 验证服务状态

### 配置版本管理
```bash
# 配置文件版本控制
git add config.yaml.example
git commit -m "Update configuration template"

# 环境特定配置
cp config.yaml config.yaml.backup
```