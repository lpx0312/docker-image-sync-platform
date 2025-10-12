# 故障排除指南

## 概述

本文档提供了Docker镜像同步平台常见问题的诊断和解决方案，帮助用户快速定位和修复问题。

## 快速诊断工具

### 1. 健康检查脚本

```bash
#!/bin/bash
# health-check.sh

echo "=== Docker镜像同步平台健康检查 ==="
echo "检查时间: $(date)"
echo

# 检查服务状态
echo "1. 服务状态检查"
if command -v systemctl &> /dev/null; then
    echo "   系统服务状态: $(systemctl is-active docker-sync)"
elif command -v docker-compose &> /dev/null; then
    echo "   Docker Compose状态:"
    docker-compose ps
fi

# 检查端口
echo "2. 端口检查"
if netstat -tlnp | grep -q ":8080"; then
    echo "   ✓ 后端端口8080正常监听"
else
    echo "   ✗ 后端端口8080未监听"
fi

if netstat -tlnp | grep -q ":80"; then
    echo "   ✓ 前端端口80正常监听"
else
    echo "   ✗ 前端端口80未监听"
fi

# 检查API健康状态
echo "3. API健康检查"
if curl -s -f http://localhost:8080/api/v1/health > /dev/null; then
    echo "   ✓ API健康检查通过"
else
    echo "   ✗ API健康检查失败"
fi

# 检查数据库连接
echo "4. 数据库连接检查"
if mysql -u docker_sync -p"$MYSQL_PASSWORD" -e "SELECT 1" docker_sync &> /dev/null; then
    echo "   ✓ 数据库连接正常"
else
    echo "   ✗ 数据库连接失败"
fi

# 检查磁盘空间
echo "5. 磁盘空间检查"
DISK_USAGE=$(df /opt/docker-sync 2>/dev/null | awk 'NR==2 {print $5}' | sed 's/%//')
if [ "$DISK_USAGE" -lt 80 ]; then
    echo "   ✓ 磁盘空间充足 (${DISK_USAGE}%)"
else
    echo "   ⚠ 磁盘空间不足 (${DISK_USAGE}%)"
fi

# 检查内存使用
echo "6. 内存使用检查"
MEM_USAGE=$(free | awk 'NR==2{printf "%.0f", $3*100/$2}')
if [ "$MEM_USAGE" -lt 80 ]; then
    echo "   ✓ 内存使用正常 (${MEM_USAGE}%)"
else
    echo "   ⚠ 内存使用过高 (${MEM_USAGE}%)"
fi

echo
echo "=== 检查完成 ==="
```

### 2. 日志分析脚本

```bash
#!/bin/bash
# log-analyzer.sh

LOG_FILE="/opt/docker-sync/logs/app.log"
ERROR_COUNT=$(grep -c "ERROR" "$LOG_FILE" 2>/dev/null || echo "0")
WARN_COUNT=$(grep -c "WARN" "$LOG_FILE" 2>/dev/null || echo "0")

echo "=== 日志分析报告 ==="
echo "错误数量: $ERROR_COUNT"
echo "警告数量: $WARN_COUNT"
echo

if [ "$ERROR_COUNT" -gt 0 ]; then
    echo "最近的错误信息:"
    grep "ERROR" "$LOG_FILE" | tail -5
fi

if [ "$WARN_COUNT" -gt 0 ]; then
    echo "最近的警告信息:"
    grep "WARN" "$LOG_FILE" | tail -5
fi
```

## 常见问题分类

### 1. 服务启动问题

#### 问题：服务无法启动

**症状：**
- 执行 `systemctl start docker-sync` 失败
- Docker容器启动失败
- 进程立即退出

**诊断步骤：**

```bash
# 1. 查看服务状态
systemctl status docker-sync

# 2. 查看详细日志
journalctl -u docker-sync -f

# 3. 检查配置文件
./docker-sync --config-test

# 4. 检查端口占用
netstat -tlnp | grep :8080
```

**常见原因和解决方案：**

| 原因 | 解决方案 |
|------|----------|
| 端口被占用 | `sudo lsof -i :8080` 查找占用进程并终止 |
| 配置文件错误 | 检查 `config.yaml` 语法和参数 |
| 权限不足 | 确保用户有读写权限 |
| 依赖服务未启动 | 启动MySQL等依赖服务 |

#### 问题：Docker容器启动失败

**诊断命令：**

```bash
# 查看容器状态
docker-compose ps

# 查看容器日志
docker-compose logs app

# 查看详细错误
docker-compose up --no-deps app
```

**解决方案：**

```bash
# 重建容器
docker-compose down
docker-compose up --build -d

# 清理无用镜像
docker system prune -f
```

### 2. 数据库连接问题

#### 问题：数据库连接失败

**症状：**
- 应用启动时报数据库连接错误
- API请求返回数据库错误
- 日志中出现连接超时

**诊断步骤：**

```bash
# 1. 测试数据库连接
mysql -h localhost -u docker_sync -p docker_sync

# 2. 检查MySQL服务状态
systemctl status mysql

# 3. 查看MySQL错误日志
tail -f /var/log/mysql/error.log

# 4. 检查网络连接
telnet localhost 3306
```

**常见错误和解决方案：**

| 错误信息 | 原因 | 解决方案 |
|----------|------|----------|
| `Access denied for user` | 用户名或密码错误 | 检查 `.env` 文件中的数据库配置 |
| `Can't connect to MySQL server` | MySQL服务未启动 | `systemctl start mysql` |
| `Unknown database` | 数据库不存在 | 执行初始化SQL脚本 |
| `Too many connections` | 连接数超限 | 调整MySQL配置或应用连接池 |

**数据库初始化脚本：**

```sql
-- init-db.sql
CREATE DATABASE IF NOT EXISTS docker_sync CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE USER IF NOT EXISTS 'docker_sync'@'localhost' IDENTIFIED BY 'your_password';
CREATE USER IF NOT EXISTS 'docker_sync'@'%' IDENTIFIED BY 'your_password';

GRANT ALL PRIVILEGES ON docker_sync.* TO 'docker_sync'@'localhost';
GRANT ALL PRIVILEGES ON docker_sync.* TO 'docker_sync'@'%';

FLUSH PRIVILEGES;

USE docker_sync;

-- 创建必要的表
CREATE TABLE IF NOT EXISTS images (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    tag VARCHAR(100) NOT NULL,
    source_registry VARCHAR(255) NOT NULL,
    target_registry VARCHAR(255) NOT NULL,
    status ENUM('pending', 'syncing', 'success', 'failed') DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY unique_image (name, tag, target_registry)
);

CREATE TABLE IF NOT EXISTS sync_logs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    image_id INT,
    operation VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE CASCADE
);
```

### 3. Git操作问题

#### 问题：Git仓库操作失败

**症状：**
- 无法克隆仓库
- 推送失败
- 认证错误

**诊断步骤：**

```bash
# 1. 测试Git连接
git clone https://gitee.com/username/repo.git

# 2. 检查SSH密钥
ssh -T git@gitee.com
ssh -T git@github.com

# 3. 测试HTTPS认证
git config --global credential.helper store
```

**常见问题解决：**

| 问题 | 解决方案 |
|------|----------|
| 认证失败 | 检查用户名、密码/令牌配置 |
| SSH密钥问题 | 重新生成并配置SSH密钥 |
| 网络超时 | 检查网络连接，配置代理 |
| 权限不足 | 确保账户有仓库访问权限 |

**Git配置修复脚本：**

```bash
#!/bin/bash
# fix-git-config.sh

echo "修复Git配置..."

# 设置Git用户信息
git config --global user.name "$GITEE_USERNAME"
git config --global user.email "$GITEE_EMAIL"

# 配置凭据存储
git config --global credential.helper store

# 测试Gitee连接
echo "测试Gitee连接..."
if git ls-remote "$GITEE_REPO_URL" &> /dev/null; then
    echo "✓ Gitee连接正常"
else
    echo "✗ Gitee连接失败"
fi

# 测试GitHub连接
echo "测试GitHub连接..."
if git ls-remote "$GITHUB_REPO_URL" &> /dev/null; then
    echo "✓ GitHub连接正常"
else
    echo "✗ GitHub连接失败"
fi
```

### 4. 镜像同步问题

#### 问题：镜像同步失败

**症状：**
- 同步任务一直处于pending状态
- 镜像拉取失败
- 推送到目标仓库失败

**诊断步骤：**

```bash
# 1. 检查Docker服务
systemctl status docker

# 2. 测试镜像拉取
docker pull nginx:latest

# 3. 测试镜像推送
docker tag nginx:latest registry.cn-hangzhou.aliyuncs.com/namespace/nginx:latest
docker push registry.cn-hangzhou.aliyuncs.com/namespace/nginx:latest

# 4. 检查网络连接
ping docker.io
ping registry.cn-hangzhou.aliyuncs.com
```

**常见错误处理：**

| 错误类型 | 原因 | 解决方案 |
|----------|------|----------|
| `pull access denied` | 源镜像不存在或无权限 | 检查镜像名称和标签 |
| `push access denied` | 目标仓库无权限 | 检查阿里云账户配置 |
| `network timeout` | 网络连接问题 | 检查网络，配置镜像加速器 |
| `disk space` | 磁盘空间不足 | 清理Docker镜像和容器 |

**Docker清理脚本：**

```bash
#!/bin/bash
# docker-cleanup.sh

echo "开始清理Docker资源..."

# 清理停止的容器
docker container prune -f

# 清理未使用的镜像
docker image prune -f

# 清理未使用的网络
docker network prune -f

# 清理未使用的卷
docker volume prune -f

# 显示清理后的空间
echo "清理完成，当前磁盘使用情况："
df -h
```

### 5. 网络连接问题

#### 问题：API请求失败

**症状：**
- 前端无法连接后端
- API请求超时
- CORS错误

**诊断步骤：**

```bash
# 1. 检查后端服务
curl -v http://localhost:8080/api/v1/health

# 2. 检查防火墙
sudo ufw status
sudo iptables -L

# 3. 检查Nginx配置
nginx -t
systemctl status nginx
```

**解决方案：**

```bash
# 修复CORS问题
# 在config.yaml中添加：
security:
  cors:
    allowed_origins: ["http://localhost:3000", "https://yourdomain.com"]
    allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
    allowed_headers: ["Content-Type", "Authorization"]

# 重启服务
systemctl restart docker-sync
```

#### 问题：外部API访问失败

**症状：**
- GitHub API调用失败
- Gitee API调用失败
- 阿里云API调用失败

**诊断脚本：**

```bash
#!/bin/bash
# test-external-apis.sh

echo "测试外部API连接..."

# 测试GitHub API
echo "1. 测试GitHub API..."
if curl -s -H "Authorization: token $GITHUB_TOKEN" https://api.github.com/user > /dev/null; then
    echo "   ✓ GitHub API连接正常"
else
    echo "   ✗ GitHub API连接失败"
fi

# 测试Gitee API
echo "2. 测试Gitee API..."
if curl -s https://gitee.com/api/v5/user?access_token=$GITEE_TOKEN > /dev/null; then
    echo "   ✓ Gitee API连接正常"
else
    echo "   ✗ Gitee API连接失败"
fi

# 测试阿里云镜像仓库
echo "3. 测试阿里云镜像仓库..."
if curl -s https://registry.cn-hangzhou.aliyuncs.com/v2/ > /dev/null; then
    echo "   ✓ 阿里云镜像仓库连接正常"
else
    echo "   ✗ 阿里云镜像仓库连接失败"
fi
```

### 6. 性能问题

#### 问题：系统响应慢

**症状：**
- API响应时间长
- 页面加载慢
- 同步任务执行缓慢

**性能诊断：**

```bash
# 1. 检查系统负载
top
htop
iostat -x 1

# 2. 检查内存使用
free -h
ps aux --sort=-%mem | head -10

# 3. 检查磁盘I/O
iotop
df -h

# 4. 检查网络
iftop
netstat -i
```

**性能优化建议：**

| 问题 | 优化方案 |
|------|----------|
| 高CPU使用 | 减少并发任务数，优化算法 |
| 高内存使用 | 增加内存，优化内存使用 |
| 磁盘I/O高 | 使用SSD，优化数据库查询 |
| 网络延迟 | 使用CDN，优化网络配置 |

**性能监控脚本：**

```bash
#!/bin/bash
# performance-monitor.sh

LOG_FILE="/var/log/performance-monitor.log"

while true; do
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    
    # CPU使用率
    CPU_USAGE=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | sed 's/%us,//')
    
    # 内存使用率
    MEM_USAGE=$(free | awk 'NR==2{printf "%.1f", $3*100/$2}')
    
    # 磁盘使用率
    DISK_USAGE=$(df /opt/docker-sync | awk 'NR==2 {print $5}' | sed 's/%//')
    
    # API响应时间
    API_RESPONSE=$(curl -o /dev/null -s -w '%{time_total}' http://localhost:8080/api/v1/health)
    
    echo "$TIMESTAMP,CPU:${CPU_USAGE}%,MEM:${MEM_USAGE}%,DISK:${DISK_USAGE}%,API:${API_RESPONSE}s" >> $LOG_FILE
    
    sleep 60
done
```

### 7. 配置问题

#### 问题：配置文件错误

**症状：**
- 服务启动时报配置错误
- 功能异常
- 参数不生效

**配置验证脚本：**

```bash
#!/bin/bash
# validate-config.sh

CONFIG_FILE="/opt/docker-sync/config.yaml"
ENV_FILE="/opt/docker-sync/.env"

echo "验证配置文件..."

# 检查YAML语法
if ! python3 -c "import yaml; yaml.safe_load(open('$CONFIG_FILE'))" 2>/dev/null; then
    echo "✗ config.yaml语法错误"
    exit 1
else
    echo "✓ config.yaml语法正确"
fi

# 检查必要配置项
REQUIRED_CONFIGS=(
    "server.port"
    "database.host"
    "database.username"
    "database.password"
    "database.database"
)

for config in "${REQUIRED_CONFIGS[@]}"; do
    if ! yq eval ".$config" "$CONFIG_FILE" &>/dev/null; then
        echo "✗ 缺少必要配置: $config"
    else
        echo "✓ 配置项存在: $config"
    fi
done

# 检查环境变量
if [ -f "$ENV_FILE" ]; then
    echo "✓ .env文件存在"
    
    # 检查必要的环境变量
    REQUIRED_ENVS=(
        "MYSQL_PASSWORD"
        "GITEE_USERNAME"
        "GITEE_PASSWORD"
        "GITHUB_TOKEN"
        "ALIYUN_USERNAME"
        "ALIYUN_PASSWORD"
    )
    
    for env in "${REQUIRED_ENVS[@]}"; do
        if grep -q "^$env=" "$ENV_FILE"; then
            echo "✓ 环境变量存在: $env"
        else
            echo "✗ 缺少环境变量: $env"
        fi
    done
else
    echo "✗ .env文件不存在"
fi
```

### 8. 日志问题

#### 问题：日志文件过大或丢失

**症状：**
- 磁盘空间被日志占满
- 找不到日志文件
- 日志轮转不工作

**日志管理脚本：**

```bash
#!/bin/bash
# log-management.sh

LOG_DIR="/opt/docker-sync/logs"
MAX_SIZE="100M"
MAX_FILES=5

echo "管理日志文件..."

# 检查日志目录
if [ ! -d "$LOG_DIR" ]; then
    echo "创建日志目录: $LOG_DIR"
    mkdir -p "$LOG_DIR"
    chown app:app "$LOG_DIR"
fi

# 检查日志文件大小
for log_file in "$LOG_DIR"/*.log; do
    if [ -f "$log_file" ]; then
        size=$(du -h "$log_file" | cut -f1)
        echo "日志文件: $(basename "$log_file"), 大小: $size"
        
        # 如果文件过大，进行轮转
        if [ $(stat -f%z "$log_file" 2>/dev/null || stat -c%s "$log_file") -gt 104857600 ]; then
            echo "轮转日志文件: $log_file"
            mv "$log_file" "${log_file}.$(date +%Y%m%d_%H%M%S)"
            touch "$log_file"
            chown app:app "$log_file"
        fi
    fi
done

# 清理旧日志文件
find "$LOG_DIR" -name "*.log.*" -mtime +7 -delete

echo "日志管理完成"
```

## 紧急恢复程序

### 1. 服务快速恢复

```bash
#!/bin/bash
# emergency-recovery.sh

echo "=== 紧急恢复程序 ==="

# 停止所有服务
echo "1. 停止服务..."
systemctl stop docker-sync
systemctl stop nginx
systemctl stop mysql

# 检查并修复数据库
echo "2. 检查数据库..."
systemctl start mysql
sleep 5

if mysql -u root -p"$MYSQL_ROOT_PASSWORD" -e "SELECT 1" &>/dev/null; then
    echo "   数据库正常"
else
    echo "   数据库异常，尝试修复..."
    mysql_upgrade -u root -p"$MYSQL_ROOT_PASSWORD"
fi

# 恢复配置文件
echo "3. 恢复配置文件..."
if [ -f "/backup/config.yaml.backup" ]; then
    cp /backup/config.yaml.backup /opt/docker-sync/config.yaml
fi

if [ -f "/backup/.env.backup" ]; then
    cp /backup/.env.backup /opt/docker-sync/.env
fi

# 清理临时文件
echo "4. 清理临时文件..."
rm -rf /opt/docker-sync/temp/*
rm -rf /tmp/docker-sync-*

# 重启服务
echo "5. 重启服务..."
systemctl start mysql
sleep 5
systemctl start docker-sync
sleep 5
systemctl start nginx

# 验证恢复
echo "6. 验证服务状态..."
if curl -f http://localhost:8080/api/v1/health &>/dev/null; then
    echo "✓ 服务恢复成功"
else
    echo "✗ 服务恢复失败"
fi

echo "=== 恢复程序完成 ==="
```

### 2. 数据恢复

```bash
#!/bin/bash
# data-recovery.sh

BACKUP_FILE=$1

if [ -z "$BACKUP_FILE" ]; then
    echo "用法: $0 <backup_file.sql>"
    exit 1
fi

echo "=== 数据恢复程序 ==="

# 停止应用服务
systemctl stop docker-sync

# 备份当前数据
echo "1. 备份当前数据..."
mysqldump -u root -p"$MYSQL_ROOT_PASSWORD" docker_sync > "/backup/before_recovery_$(date +%Y%m%d_%H%M%S).sql"

# 恢复数据
echo "2. 恢复数据..."
mysql -u root -p"$MYSQL_ROOT_PASSWORD" docker_sync < "$BACKUP_FILE"

if [ $? -eq 0 ]; then
    echo "✓ 数据恢复成功"
else
    echo "✗ 数据恢复失败"
    exit 1
fi

# 重启服务
echo "3. 重启服务..."
systemctl start docker-sync

echo "=== 数据恢复完成 ==="
```

## 监控和告警

### 1. 系统监控脚本

```bash
#!/bin/bash
# system-monitor.sh

ALERT_EMAIL="admin@example.com"
ALERT_WEBHOOK="https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK"

send_alert() {
    local message="$1"
    local level="$2"
    
    # 发送邮件告警
    echo "$message" | mail -s "[$level] Docker Sync Platform Alert" "$ALERT_EMAIL"
    
    # 发送Slack告警
    curl -X POST -H 'Content-type: application/json' \
        --data "{\"text\":\"[$level] $message\"}" \
        "$ALERT_WEBHOOK"
}

# 检查服务状态
if ! systemctl is-active --quiet docker-sync; then
    send_alert "Docker Sync服务已停止" "CRITICAL"
fi

# 检查磁盘空间
DISK_USAGE=$(df /opt/docker-sync | awk 'NR==2 {print $5}' | sed 's/%//')
if [ "$DISK_USAGE" -gt 90 ]; then
    send_alert "磁盘空间不足: ${DISK_USAGE}%" "WARNING"
fi

# 检查内存使用
MEM_USAGE=$(free | awk 'NR==2{printf "%.0f", $3*100/$2}')
if [ "$MEM_USAGE" -gt 90 ]; then
    send_alert "内存使用过高: ${MEM_USAGE}%" "WARNING"
fi

# 检查API响应
if ! curl -f http://localhost:8080/api/v1/health &>/dev/null; then
    send_alert "API健康检查失败" "CRITICAL"
fi
```

### 2. 定时任务配置

```bash
# 添加到crontab
# crontab -e

# 每分钟检查服务状态
* * * * * /opt/docker-sync/scripts/health-check.sh

# 每5分钟检查系统资源
*/5 * * * * /opt/docker-sync/scripts/system-monitor.sh

# 每小时清理临时文件
0 * * * * /opt/docker-sync/scripts/cleanup.sh

# 每天备份数据库
0 2 * * * /opt/docker-sync/scripts/backup-db.sh

# 每周清理Docker资源
0 3 * * 0 /opt/docker-sync/scripts/docker-cleanup.sh
```

## 联系支持

如果以上解决方案无法解决您的问题，请通过以下方式联系技术支持：

1. **GitHub Issues**: https://github.com/your-username/docker-image-sync-platform/issues
2. **邮箱支持**: support@example.com
3. **在线文档**: https://docs.example.com

提交问题时，请包含以下信息：
- 系统环境信息（操作系统、Docker版本等）
- 错误日志和堆栈跟踪
- 配置文件内容（敏感信息请脱敏）
- 重现步骤
- 期望的行为和实际行为

这样可以帮助我们更快地定位和解决问题。