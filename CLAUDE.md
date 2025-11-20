# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Docker image synchronization platform built with Go backend and Vue.js frontend. It provides automated synchronization of Docker images from various registries (Docker Hub, GCR, etc.) to Alibaba Cloud Container Registry (ACR) through GitHub Actions workflows.

## Architecture Overview

### Backend (Go)
- **Framework**: Gin (REST API)
- **Database**: MySQL with GORM ORM
- **Configuration**: Viper for config management
- **Logging**: Zap for structured logging
- **Git Operations**: go-git library with optimized caching
- **Container Registry**: go-containerregistry

### Frontend (Vue.js)
- **Framework**: Vue 3 with Composition API
- **UI Library**: Element Plus
- **State Management**: Pinia
- **Routing**: Vue Router
- **Build Tool**: Vite
- **HTTP Client**: Axios

### Project Structure
```
docker-image-sync-platform/
├── main.go                    # Application entry point
├── config.yaml               # Configuration file
├── internal/                 # Backend internal packages
│   ├── config/              # Configuration management
│   ├── database/            # Database connection and migrations
│   ├── handlers/            # HTTP request handlers
│   ├── middleware/          # HTTP middleware (CORS, logging, rate limiting)
│   ├── models/              # Data models and structs
│   ├── services/            # Business logic services
│   └── logger/              # Logging utilities
├── web/                     # Frontend Vue.js application
│   ├── src/
│   │   ├── api/            # API service layer
│   │   ├── components/     # Vue components
│   │   ├── router/         # Vue Router configuration
│   │   ├── stores/         # Pinia stores
│   │   └── views/          # Page views
│   └── package.json
├── deploy/                  # Docker deployment configurations
├── scripts/                 # Utility scripts
└── Makefile                # Build and development commands
```

## Core Services

### 1. Git Service Factory
- `internal/services/git_factory.go`
- Dynamically creates Git service instances (Gitee/GitHub)
- Handles repository cloning and image file parsing

### 2. GitHub Service
- `internal/services/github.go`
- Monitors GitHub Actions workflows
- Checks workflow status and retrieves logs

### 3. Config Service
- `internal/services/config.go`
- Manages encrypted configuration storage
- Handles database CRUD operations for settings

### 4. Sync Handler
- `internal/handlers/sync.go`
- Processes image sync requests
- Manages batch and individual sync operations

### 5. Git Optimized Service
- `internal/services/git_optimized.go`
- Provides optimized Git operations with caching and sparse checkout
- Includes GitHub code operations testing functionality
- Methods: `PullImagesFileForTesting()`, `UpdateImagesFileForTesting()`

## API Structure

### Base URL: `/api/v1`

### Sync Operations
- `POST /sync/submit` - Submit single image sync
- `POST /sync/batch` - Submit batch image sync
- `GET /sync/status/:taskId` - Get sync status
- `GET /sync/history` - Get sync history

### Image Management
- `GET /images/list` - List images with pagination
- `GET /images/:id` - Get image details
- `DELETE /images/:id` - Delete image record
- `POST /images/:id/retry` - Retry failed sync

### Configuration
- `GET /config/all` - Get all configurations
- `GET /config/git` - Get Git configurations
- `PUT /config/git/gitee` - Update Gitee config
- `PUT /config/git/github` - Update GitHub config
- `POST /config/git/test` - Test Git connection
- `POST /config/git-test-operations` - Test GitHub code pull and commit operations
- `PUT /config/aliyun-db` - Update Aliyun ACR config

### GitHub Integration
- `GET /github/runs` - List workflow runs
- `GET /github/runs/:runId` - Get workflow details
- `GET /github/rate-limit` - Check API rate limits

## Configuration

### Main Config File: `config.yaml`
Key sections:
- `server`: HTTP server configuration
- `database`: MySQL connection settings
- `git`: Gitee/GitHub repository configuration
- `aliyun`: Alibaba Cloud Registry settings
- `log`: Logging configuration
- `sync`: Sync task settings (timeouts, concurrency)

### Security Notes
- Sensitive data (passwords, tokens) are encrypted in database
- Rate limiting applied to API endpoints
- CORS configuration for frontend access
- Request logging and error handling middleware

## Database Models

### Core Tables
- `image_sync_records`: Individual image sync records
- `sync_tasks`: Batch sync task management
- `system_configs`: Encrypted configuration storage

### Key Fields
- Sync status tracking (pending, running, success, failed)
- GitHub Actions workflow integration
- Multi-architecture support (amd64, arm64)
- Retry mechanisms and priority levels

## Frontend Components

### Main Views
- `SyncView.vue` - Image sync operations
- `ImagesView.vue` - Image management and status
- `ConfigView.vue` - System configuration
- `GitHubView.vue` - Workflow monitoring

### Key Components
- `SingleSyncForm.vue` - Single image sync
- `BatchSyncForm.vue` - Batch image sync
- `GitConfigForm.vue` - Git repository configuration with GitHub testing feature
- `GitTestResultDialog.vue` - GitHub operations test results display
- `AliyunConfigForm.vue` - ACR configuration

## GitHub Code Operations Testing Feature

### Overview
The platform includes a comprehensive GitHub code operations testing feature that allows users to validate their GitHub configuration before using it for Docker image synchronization.

### Features
- **Three-Step Testing Process**:
  1. Pull images.txt file from GitHub repository
  2. Commit test content to images.txt
  3. Verify the commit and push changes
- **Detailed Timing Information**: Records elapsed time for each operation (pull, commit, push)
- **Comprehensive Error Reporting**: Provides detailed error messages for debugging
- **User-Friendly Interface**: Modern Vue.js dialog with step-by-step results display

### API Endpoint
- **URL**: `POST /api/v1/config/git-test-operations`
- **Request Parameters**:
  ```json
  {
    "repo_url": "https://github.com/username/repository.git",
    "username": "github-username",
    "token": "github-personal-access-token",
    "email": "user@example.com",
    "branch": "main"
  }
  ```
- **Response Structure**:
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

### Frontend Components
- **GitConfigForm.vue**: GitHub configuration form with "测试代码拉取和提交" button
- **GitTestResultDialog.vue**: Results display dialog showing:
  - Overall test status with success indicators
  - Step-by-step breakdown with timing
  - Error messages and troubleshooting hints
  - Commit SHA with GitHub link (when successful)
  - Statistical information in tabular format

### Implementation Details
- **Backend**: Uses temporary directories for testing (`/tmp/git-test-operations`)
- **Git Operations**: Implements actual Git clone, commit, and push operations
- **Error Handling**: Comprehensive error catching with user-friendly messages
- **Security**: Tests use isolated temporary environments, no impact on production data

### Usage
1. Navigate to the Configuration page in the web interface
2. Configure GitHub repository settings (URL, username, token, email, branch)
3. Click "测试代码拉取和提交" button in the GitHub configuration section
4. Review detailed test results in the popup dialog
5. Verify all operations succeed before using the configuration for image sync

### Benefits
- **Configuration Validation**: Ensures GitHub credentials and permissions are correct
- **Early Error Detection**: Identifies connectivity or authentication issues before actual sync operations
- **Performance Monitoring**: Provides timing metrics for Git operations
- **User Confidence**: Gives users confidence that their GitHub integration will work properly

## 本地开发测试方法

### 开发环境
- **操作系统**: Windows 11
- **终端**: PowerShell

### 服务启动流程

#### 前端服务启动 (固定端口 3000)
```bash
cd web
npm install  # 首次运行需要安装依赖
npm run dev
```

#### 后端服务启动 (固定端口 8080)
```bash
go run main.go
```

### 开发注意事项

#### 日志管理
- **后端日志**: 存放在 `logs/app.log` 文件中
- **日志查看**: 实时监控开发过程中的错误和信息


#### 前后端重启
- **前端重启**: 固定使用 3000端口,重启前需要先终止占用3000端口的进程,然后在重启前端服务
- **后端重启**: 固定使用 8080端口,重启前需要先终止占用8080端口的进程,然后在重启后端服务
停止方法
```
  # 查找占用端口的进程PID
  netstat -ano | findstr ":端口号"

  # 直接使用PID停止进程
  powershell -Command "Stop-Process -Id [PID] -Force"

  # 或者查找并停止特定端口的所有进程
  powershell -Command "Get-Process | Where-Object {$_.ProcessName -eq 'node' -or $_.ProcessName -eq 'go'} | Stop-Process -Force"
```

#### 代码更新
- **前端修改**: 重启前后端
- **后端修改**: 重启前后端
- **只要修改了代码，都需要重新启动前后端**

### 自动化测试

#### Chrome DevTools 测试
平台支持使用 Chrome DevTools MCP进行自动化测试：
- **页面元素操作**: 点击、输入、导航等
- **表单验证**: 配置表单的完整性和功能测试
- **API 接口测试**: 验证前后端数据交互
- **界面渲染测试**: 确认修改生效

使用Chrome DevTools MCP生成的所有文件，放在当前目录的ChromeDevTools-Files目录下,没有这个目录，需要先创建这个目录。



#### 测试功能定位
如需测试特定功能或界面，且你不清楚在哪里，可询问我


#### 常见测试场景
1. **配置管理测试**: Git配置保存、验证、切换功能
2. **镜像同步测试**: 单个和批量同步操作
3. **状态监控测试**: 同步进度和结果查看
4. **错误处理测试**: 异常情况的处理和提示

### 开发工作流

1. **环境准备**: 确保端口未被占用，启动前后端服务
2. **代码修改**: 按功能需求修改前端或后端代码
3. **服务重启**: 修改后重新启动对应服务
4. **功能测试**: 使用 Chrome DevTools 进行自动化验证
5. **结果确认**: 确保功能正常运行，无报错信息

## Deployment

### Docker Options
1. **Separate Containers**: `deploy/docker-signal/`
   - Frontend and backend in separate containers
   - Good for development and microservices

2. **All-in-One**: `deploy/docker-all/`
   - Single container with both frontend and backend
   - Simplified production deployment

### Environment Variables
- Database connection settings
- Git repository credentials
- Alibaba Cloud Registry configuration
- Server and logging configuration

## Troubleshooting

### Common Issues
1. **Database Connection**: Check MySQL service and credentials
2. **Git Operations**: Verify repository access and permissions
3. **Sync Failures**: Check GitHub Actions workflow status
4. **Frontend Build**: Ensure Node.js dependencies installed
5. **Port Conflicts**: Use netstat to identify and resolve conflicts

### Health Checks
- API health endpoint: `/api/v1/health`
- Service status: `make health-check`
- Logs: `logs/app.log` or `make docker-logs`

## Security Considerations

- Rate limiting on sync endpoints
- Encrypted storage of sensitive configuration
- CORS protection
- Request logging and monitoring
- Input validation and sanitization


## 使用中文回答和询问