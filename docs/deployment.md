# 部署指南

## 概述

Docker镜像同步平台支持多种部署方式，包括Docker Compose、Kubernetes、手动部署等。本文档详细介绍各种部署方法和最佳实践。

## 系统要求

### 硬件要求

| 环境 | CPU | 内存 | 存储 | 网络 |
|------|-----|------|------|------|
| 开发环境 | 2核 | 4GB | 20GB | 10Mbps |
| 测试环境 | 4核 | 8GB | 50GB | 50Mbps |
| 生产环境 | 8核 | 16GB | 200GB | 100Mbps |

### 软件要求

- **操作系统**: Linux (Ubuntu 20.04+, CentOS 8+), macOS, Windows
- **Docker**: 20.10+
- **Docker Compose**: 2.0+
- **Git**: 2.30+
- **MySQL**: 8.0+ (如果独立部署)

### 网络要求

- 能够访问GitHub API (api.github.com)
- 能够访问Gitee API (gitee.com)
- 能够访问阿里云容器镜像服务
- 能够访问Docker Hub等镜像仓库

## 快速部署 (Docker Compose)

### 1. 准备工作

```bash
# 克隆项目
git clone https://github.com/your-username/docker-image-sync-platform.git
cd docker-image-sync-platform

# 创建环境变量文件
cp .env.example .env
```

### 2. 配置环境变量

编辑 `.env` 文件，配置必要的参数：

```bash
# 数据库配置
MYSQL_ROOT_PASSWORD=your_strong_password
MYSQL_DATABASE=docker_sync
MYSQL_USER=docker_sync
MYSQL_PASSWORD=your_app_password

# Git仓库配置
GITEE_REPO_URL=https://gitee.com/your-username/docker-images.git
GITEE_USERNAME=your-username
GITEE_PASSWORD=your-password-or-token
GITEE_EMAIL=your-email@example.com

GITHUB_REPO_URL=https://github.com/your-username/docker-images.git
GITHUB_USERNAME=your-username
GITHUB_TOKEN=REDACTED_GITHUB_TOKEN

# 阿里云配置
ALIYUN_REGISTRY=registry.cn-hangzhou.aliyuncs.com
ALIYUN_NAMESPACE=your-namespace
ALIYUN_USERNAME=your-aliyun-username
ALIYUN_PASSWORD=your-aliyun-password
```

### 3. 启动服务

```bash
# 一键部署
./deploy.sh

# 或者手动启动
docker-compose up -d
```

### 4. 验证部署

```bash
# 检查服务状态
docker-compose ps

# 查看日志
docker-compose logs -f

# 健康检查
curl http://localhost:8080/api/v1/health
```

### 5. 访问服务

- **前端界面**: http://localhost:80
- **后端API**: http://localhost:8080
- **API文档**: http://localhost:8080/swagger/index.html

## 生产环境部署

### 1. 使用反向代理 (Nginx)

创建 `nginx.conf` 配置文件：

```nginx
upstream backend {
    server app:8080;
}

server {
    listen 80;
    server_name your-domain.com;
    
    # 重定向到HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;
    
    # SSL证书配置
    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512;
    
    # 前端静态文件
    location / {
        root /usr/share/nginx/html;
        try_files $uri $uri/ /index.html;
        
        # 缓存配置
        location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg)$ {
            expires 1y;
            add_header Cache-Control "public, immutable";
        }
    }
    
    # API代理
    location /api/ {
        proxy_pass http://backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 超时配置
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }
    
    # WebSocket支持（如果需要）
    location /ws/ {
        proxy_pass http://backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### 2. 生产环境 Docker Compose

创建 `docker-compose.prod.yml`：

```yaml
version: '3.8'

services:
  mysql:
    image: mysql:8.0
    container_name: docker-sync-mysql
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD}
      MYSQL_DATABASE: ${MYSQL_DATABASE}
      MYSQL_USER: ${MYSQL_USER}
      MYSQL_PASSWORD: ${MYSQL_PASSWORD}
    volumes:
      - mysql_data:/var/lib/mysql
      - ./sql/init.sql:/docker-entrypoint-initdb.d/init.sql
    ports:
      - "3306:3306"
    command: --default-authentication-plugin=mysql_native_password
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      timeout: 20s
      retries: 10

  app:
    build: .
    container_name: docker-sync-app
    restart: unless-stopped
    depends_on:
      mysql:
        condition: service_healthy
    environment:
      - GIN_MODE=release
      - LOG_LEVEL=info
    env_file:
      - .env
    volumes:
      - ./logs:/app/logs
      - ./temp:/app/temp
      - ./config.yaml:/app/config.yaml
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/api/v1/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  nginx:
    image: nginx:alpine
    container_name: docker-sync-nginx
    restart: unless-stopped
    depends_on:
      - app
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
      - ./ssl:/etc/nginx/ssl
      - ./web/dist:/usr/share/nginx/html
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/api/v1/health"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  mysql_data:
    driver: local

networks:
  default:
    name: docker-sync-network
```

### 3. 启动生产环境

```bash
# 构建并启动
docker-compose -f docker-compose.prod.yml up -d

# 查看状态
docker-compose -f docker-compose.prod.yml ps

# 查看日志
docker-compose -f docker-compose.prod.yml logs -f
```

## Kubernetes 部署

### 1. 创建命名空间

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: docker-sync
```

### 2. 配置 ConfigMap

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: docker-sync-config
  namespace: docker-sync
data:
  config.yaml: |
    server:
      port: 8080
      mode: release
      host: "0.0.0.0"
    database:
      host: "mysql-service"
      port: 3306
      username: "docker_sync"
      database: "docker_sync"
      charset: "utf8mb4"
      parse_time: true
      loc: "Local"
      max_idle_conns: 10
      max_open_conns: 100
      conn_max_lifetime: 3600
    log:
      level: "info"
      file_path: "./logs/app.log"
      max_size: 100
      max_backups: 3
      max_age: 28
      compress: true
```

### 3. 配置 Secret

```yaml
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: docker-sync-secret
  namespace: docker-sync
type: Opaque
data:
  mysql-password: <base64-encoded-password>
  gitee-password: <base64-encoded-password>
  github-token: <base64-encoded-token>
  aliyun-password: <base64-encoded-password>
```

### 4. MySQL 部署

```yaml
# mysql.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql
  namespace: docker-sync
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mysql
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
      - name: mysql
        image: mysql:8.0
        env:
        - name: MYSQL_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: docker-sync-secret
              key: mysql-password
        - name: MYSQL_DATABASE
          value: "docker_sync"
        - name: MYSQL_USER
          value: "docker_sync"
        - name: MYSQL_PASSWORD
          valueFrom:
            secretKeyRef:
              name: docker-sync-secret
              key: mysql-password
        ports:
        - containerPort: 3306
        volumeMounts:
        - name: mysql-storage
          mountPath: /var/lib/mysql
        livenessProbe:
          exec:
            command:
            - mysqladmin
            - ping
            - -h
            - localhost
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command:
            - mysqladmin
            - ping
            - -h
            - localhost
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: mysql-storage
        persistentVolumeClaim:
          claimName: mysql-pvc

---
apiVersion: v1
kind: Service
metadata:
  name: mysql-service
  namespace: docker-sync
spec:
  selector:
    app: mysql
  ports:
  - port: 3306
    targetPort: 3306

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mysql-pvc
  namespace: docker-sync
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
```

### 5. 应用部署

```yaml
# app.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: docker-sync-app
  namespace: docker-sync
spec:
  replicas: 2
  selector:
    matchLabels:
      app: docker-sync-app
  template:
    metadata:
      labels:
        app: docker-sync-app
    spec:
      containers:
      - name: app
        image: your-registry/docker-sync:latest
        env:
        - name: GIN_MODE
          value: "release"
        - name: LOG_LEVEL
          value: "info"
        - name: MYSQL_PASSWORD
          valueFrom:
            secretKeyRef:
              name: docker-sync-secret
              key: mysql-password
        - name: GITEE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: docker-sync-secret
              key: gitee-password
        - name: GITHUB_TOKEN
          valueFrom:
            secretKeyRef:
              name: docker-sync-secret
              key: github-token
        - name: ALIYUN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: docker-sync-secret
              key: aliyun-password
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: config-volume
          mountPath: /app/config.yaml
          subPath: config.yaml
        - name: logs-volume
          mountPath: /app/logs
        livenessProbe:
          httpGet:
            path: /api/v1/health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/v1/health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
      volumes:
      - name: config-volume
        configMap:
          name: docker-sync-config
      - name: logs-volume
        emptyDir: {}

---
apiVersion: v1
kind: Service
metadata:
  name: docker-sync-service
  namespace: docker-sync
spec:
  selector:
    app: docker-sync-app
  ports:
  - port: 8080
    targetPort: 8080
  type: ClusterIP
```

### 6. Ingress 配置

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: docker-sync-ingress
  namespace: docker-sync
  annotations:
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/proxy-body-size: "100m"
spec:
  tls:
  - hosts:
    - your-domain.com
    secretName: docker-sync-tls
  rules:
  - host: your-domain.com
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: docker-sync-service
            port:
              number: 8080
      - path: /
        pathType: Prefix
        backend:
          service:
            name: frontend-service
            port:
              number: 80
```

### 7. 部署到 Kubernetes

```bash
# 创建命名空间
kubectl apply -f namespace.yaml

# 创建配置
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml

# 部署数据库
kubectl apply -f mysql.yaml

# 部署应用
kubectl apply -f app.yaml

# 配置Ingress
kubectl apply -f ingress.yaml

# 查看状态
kubectl get pods -n docker-sync
kubectl get services -n docker-sync
kubectl get ingress -n docker-sync
```

## 手动部署

### 1. 环境准备

```bash
# 安装Go 1.21+
wget https://golang.org/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 安装Node.js 18+
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt-get install -y nodejs

# 安装MySQL 8.0
sudo apt update
sudo apt install mysql-server
sudo mysql_secure_installation
```

### 2. 数据库配置

```sql
-- 创建数据库和用户
CREATE DATABASE docker_sync CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'docker_sync'@'localhost' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON docker_sync.* TO 'docker_sync'@'localhost';
FLUSH PRIVILEGES;
```

### 3. 构建应用

```bash
# 克隆代码
git clone https://github.com/your-username/docker-image-sync-platform.git
cd docker-image-sync-platform

# 构建后端
go mod download
go build -o docker-sync main.go

# 构建前端
cd web
npm install
npm run build
cd ..

# 复制前端文件到静态目录
mkdir -p static
cp -r web/dist/* static/
```

### 4. 配置文件

创建 `config.yaml`：

```yaml
server:
  port: 8080
  mode: release
  host: "0.0.0.0"

database:
  host: "localhost"
  port: 3306
  username: "docker_sync"
  password: "your_password"
  database: "docker_sync"
  charset: "utf8mb4"
  parse_time: true
  loc: "Local"

# ... 其他配置
```

### 5. 创建系统服务

创建 `/etc/systemd/system/docker-sync.service`：

```ini
[Unit]
Description=Docker Image Sync Platform
After=network.target mysql.service

[Service]
Type=simple
User=app
Group=app
WorkingDirectory=/opt/docker-sync
ExecStart=/opt/docker-sync/docker-sync
Restart=always
RestartSec=5
Environment=GIN_MODE=release
EnvironmentFile=/opt/docker-sync/.env

# 安全配置
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/docker-sync/logs /opt/docker-sync/temp

[Install]
WantedBy=multi-user.target
```

### 6. 启动服务

```bash
# 创建用户和目录
sudo useradd -r -s /bin/false app
sudo mkdir -p /opt/docker-sync
sudo cp docker-sync /opt/docker-sync/
sudo cp config.yaml /opt/docker-sync/
sudo cp .env /opt/docker-sync/
sudo chown -R app:app /opt/docker-sync

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable docker-sync
sudo systemctl start docker-sync

# 查看状态
sudo systemctl status docker-sync
```

## 监控和日志

### 1. 日志配置

```yaml
# config.yaml
log:
  level: "info"
  file_path: "./logs/app.log"
  max_size: 100      # MB
  max_backups: 3     # 保留文件数
  max_age: 28        # 保留天数
  compress: true     # 压缩旧文件
```

### 2. 日志收集 (ELK Stack)

```yaml
# docker-compose.monitoring.yml
version: '3.8'

services:
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:7.15.0
    environment:
      - discovery.type=single-node
      - "ES_JAVA_OPTS=-Xms512m -Xmx512m"
    ports:
      - "9200:9200"
    volumes:
      - es_data:/usr/share/elasticsearch/data

  logstash:
    image: docker.elastic.co/logstash/logstash:7.15.0
    volumes:
      - ./logstash.conf:/usr/share/logstash/pipeline/logstash.conf
      - ./logs:/logs
    depends_on:
      - elasticsearch

  kibana:
    image: docker.elastic.co/kibana/kibana:7.15.0
    ports:
      - "5601:5601"
    environment:
      - ELASTICSEARCH_HOSTS=http://elasticsearch:9200
    depends_on:
      - elasticsearch

volumes:
  es_data:
```

### 3. 监控配置 (Prometheus + Grafana)

```yaml
# docker-compose.monitoring.yml (续)
  prometheus:
    image: prom/prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus_data:/prometheus

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana_data:/var/lib/grafana
      - ./grafana/dashboards:/etc/grafana/provisioning/dashboards
      - ./grafana/datasources:/etc/grafana/provisioning/datasources

volumes:
  prometheus_data:
  grafana_data:
```

## 备份和恢复

### 1. 数据库备份

```bash
#!/bin/bash
# backup-db.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backup/mysql"
DB_NAME="docker_sync"

mkdir -p $BACKUP_DIR

# 备份数据库
mysqldump -u root -p$MYSQL_ROOT_PASSWORD \
  --single-transaction \
  --routines \
  --triggers \
  $DB_NAME > $BACKUP_DIR/docker_sync_$DATE.sql

# 压缩备份文件
gzip $BACKUP_DIR/docker_sync_$DATE.sql

# 删除7天前的备份
find $BACKUP_DIR -name "*.sql.gz" -mtime +7 -delete

echo "数据库备份完成: docker_sync_$DATE.sql.gz"
```

### 2. 完整备份脚本

```bash
#!/bin/bash
# backup-full.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backup/full"
APP_DIR="/opt/docker-sync"

mkdir -p $BACKUP_DIR

# 创建备份目录
BACKUP_PATH="$BACKUP_DIR/backup_$DATE"
mkdir -p $BACKUP_PATH

# 备份数据库
mysqldump -u root -p$MYSQL_ROOT_PASSWORD \
  --single-transaction \
  --routines \
  --triggers \
  docker_sync > $BACKUP_PATH/database.sql

# 备份配置文件
cp $APP_DIR/config.yaml $BACKUP_PATH/
cp $APP_DIR/.env $BACKUP_PATH/

# 备份日志文件
cp -r $APP_DIR/logs $BACKUP_PATH/

# 备份Git仓库
cp -r $APP_DIR/temp $BACKUP_PATH/

# 创建压缩包
cd $BACKUP_DIR
tar -czf backup_$DATE.tar.gz backup_$DATE/
rm -rf backup_$DATE/

# 删除30天前的备份
find $BACKUP_DIR -name "backup_*.tar.gz" -mtime +30 -delete

echo "完整备份完成: backup_$DATE.tar.gz"
```

### 3. 恢复脚本

```bash
#!/bin/bash
# restore.sh

if [ $# -ne 1 ]; then
    echo "用法: $0 <backup_file.tar.gz>"
    exit 1
fi

BACKUP_FILE=$1
RESTORE_DIR="/tmp/restore_$(date +%s)"
APP_DIR="/opt/docker-sync"

# 解压备份文件
mkdir -p $RESTORE_DIR
tar -xzf $BACKUP_FILE -C $RESTORE_DIR

# 停止服务
sudo systemctl stop docker-sync

# 恢复数据库
mysql -u root -p$MYSQL_ROOT_PASSWORD docker_sync < $RESTORE_DIR/backup_*/database.sql

# 恢复配置文件
cp $RESTORE_DIR/backup_*/config.yaml $APP_DIR/
cp $RESTORE_DIR/backup_*/.env $APP_DIR/

# 恢复日志文件（可选）
# cp -r $RESTORE_DIR/backup_*/logs $APP_DIR/

# 恢复Git仓库
cp -r $RESTORE_DIR/backup_*/temp $APP_DIR/

# 设置权限
sudo chown -R app:app $APP_DIR

# 启动服务
sudo systemctl start docker-sync

# 清理临时文件
rm -rf $RESTORE_DIR

echo "恢复完成"
```

## 性能优化

### 1. 数据库优化

```sql
-- MySQL配置优化
-- /etc/mysql/mysql.conf.d/mysqld.cnf

[mysqld]
# 连接配置
max_connections = 200
max_connect_errors = 10000

# 缓存配置
innodb_buffer_pool_size = 1G
query_cache_size = 256M
query_cache_type = 1

# 日志配置
slow_query_log = 1
slow_query_log_file = /var/log/mysql/slow.log
long_query_time = 2

# 字符集配置
character-set-server = utf8mb4
collation-server = utf8mb4_unicode_ci
```

### 2. 应用优化

```yaml
# config.yaml
database:
  max_idle_conns: 20     # 增加空闲连接数
  max_open_conns: 200    # 增加最大连接数
  conn_max_lifetime: 1800 # 减少连接生存时间

sync:
  max_concurrent_jobs: 5  # 根据服务器性能调整
  timeout_minutes: 45     # 增加超时时间

log:
  level: "warn"          # 生产环境减少日志级别
```

### 3. Nginx优化

```nginx
# nginx.conf
worker_processes auto;
worker_connections 1024;

http {
    # 启用gzip压缩
    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_types text/plain text/css application/json application/javascript;

    # 缓存配置
    proxy_cache_path /var/cache/nginx levels=1:2 keys_zone=api_cache:10m max_size=1g inactive=60m;

    # 限流配置
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;

    server {
        # API缓存
        location /api/v1/images/stats {
            proxy_cache api_cache;
            proxy_cache_valid 200 5m;
            proxy_pass http://backend;
        }

        # 限流
        location /api/ {
            limit_req zone=api burst=20 nodelay;
            proxy_pass http://backend;
        }
    }
}
```

## 安全配置

### 1. 防火墙配置

```bash
# Ubuntu/Debian
sudo ufw enable
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 80/tcp    # HTTP
sudo ufw allow 443/tcp   # HTTPS
sudo ufw deny 3306/tcp   # 禁止外部访问MySQL
sudo ufw deny 8080/tcp   # 禁止外部访问后端

# CentOS/RHEL
sudo firewall-cmd --permanent --add-service=ssh
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

### 2. SSL证书配置

```bash
# 使用Let's Encrypt
sudo apt install certbot python3-certbot-nginx

# 获取证书
sudo certbot --nginx -d your-domain.com

# 自动续期
sudo crontab -e
# 添加以下行
0 12 * * * /usr/bin/certbot renew --quiet
```

### 3. 安全加固

```bash
# 创建专用用户
sudo useradd -r -s /bin/false docker-sync

# 设置文件权限
sudo chmod 600 /opt/docker-sync/.env
sudo chmod 644 /opt/docker-sync/config.yaml

# 禁用不必要的服务
sudo systemctl disable apache2
sudo systemctl disable sendmail
```

## 故障排除

### 1. 常见问题

#### 服务无法启动
```bash
# 检查端口占用
sudo netstat -tlnp | grep :8080

# 检查配置文件
./docker-sync --config-test

# 查看系统日志
sudo journalctl -u docker-sync -f
```

#### 数据库连接失败
```bash
# 测试数据库连接
mysql -h localhost -u docker_sync -p docker_sync

# 检查MySQL状态
sudo systemctl status mysql

# 查看MySQL日志
sudo tail -f /var/log/mysql/error.log
```

#### Git操作失败
```bash
# 测试Git连接
git clone https://gitee.com/username/repo.git

# 检查SSH密钥
ssh -T git@gitee.com

# 检查访问令牌
curl -H "Authorization: token YOUR_TOKEN" https://api.github.com/user
```

### 2. 性能问题

#### 同步速度慢
```bash
# 检查网络连接
ping docker.io
ping registry.cn-hangzhou.aliyuncs.com

# 检查磁盘空间
df -h

# 检查系统负载
top
htop
```

#### 内存使用过高
```bash
# 检查内存使用
free -h
ps aux --sort=-%mem | head

# 调整配置
# 减少并发任务数
# 增加系统内存
```

### 3. 监控脚本

```bash
#!/bin/bash
# monitor.sh

LOG_FILE="/var/log/docker-sync-monitor.log"

check_service() {
    if ! systemctl is-active --quiet docker-sync; then
        echo "$(date): 服务已停止，正在重启..." >> $LOG_FILE
        systemctl start docker-sync
    fi
}

check_disk_space() {
    USAGE=$(df /opt/docker-sync | awk 'NR==2 {print $5}' | sed 's/%//')
    if [ $USAGE -gt 80 ]; then
        echo "$(date): 磁盘使用率过高: ${USAGE}%" >> $LOG_FILE
    fi
}

check_memory() {
    USAGE=$(free | awk 'NR==2{printf "%.0f", $3*100/$2}')
    if [ $USAGE -gt 80 ]; then
        echo "$(date): 内存使用率过高: ${USAGE}%" >> $LOG_FILE
    fi
}

# 执行检查
check_service
check_disk_space
check_memory

echo "$(date): 监控检查完成" >> $LOG_FILE
```

## 升级指南

### 1. 版本升级

```bash
#!/bin/bash
# upgrade.sh

NEW_VERSION=$1
BACKUP_DIR="/backup/upgrade_$(date +%Y%m%d_%H%M%S)"

if [ -z "$NEW_VERSION" ]; then
    echo "用法: $0 <new_version>"
    exit 1
fi

# 创建备份
mkdir -p $BACKUP_DIR
./backup-full.sh

# 停止服务
sudo systemctl stop docker-sync

# 备份当前版本
cp /opt/docker-sync/docker-sync $BACKUP_DIR/

# 下载新版本
wget https://github.com/user/repo/releases/download/$NEW_VERSION/docker-sync
chmod +x docker-sync

# 更新应用
sudo cp docker-sync /opt/docker-sync/

# 更新数据库（如果需要）
# mysql -u root -p docker_sync < migrations/$NEW_VERSION.sql

# 启动服务
sudo systemctl start docker-sync

# 验证升级
sleep 10
if curl -f http://localhost:8080/api/v1/health; then
    echo "升级成功"
else
    echo "升级失败，正在回滚..."
    sudo cp $BACKUP_DIR/docker-sync /opt/docker-sync/
    sudo systemctl start docker-sync
fi
```

### 2. 配置迁移

```bash
#!/bin/bash
# migrate-config.sh

OLD_CONFIG="/opt/docker-sync/config.yaml.old"
NEW_CONFIG="/opt/docker-sync/config.yaml"

# 备份旧配置
cp $NEW_CONFIG $OLD_CONFIG

# 合并配置（示例）
yq eval-all 'select(fileIndex == 0) * select(fileIndex == 1)' $OLD_CONFIG $NEW_CONFIG > config.merged.yaml
mv config.merged.yaml $NEW_CONFIG
```

这个部署指南涵盖了从开发环境到生产环境的各种部署方式，包括监控、备份、安全配置等运维最佳实践。根据实际需求选择合适的部署方式。