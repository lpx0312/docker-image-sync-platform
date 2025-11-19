# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Docker image synchronization platform built with Go backend and Vue.js frontend. It provides automated synchronization of Docker images from various registries (Docker Hub, GCR, etc.) to Alibaba Cloud Container Registry (ACR) through GitHub Actions workflows.

## Key Commands

### Development
```bash
# Initialize project environment
make init

# Install all dependencies (Go modules + npm packages)
make deps

# Start full development environment (frontend + backend)
make dev

# Start only frontend (Vue.js dev server on :3000)
make frontend

# Start only backend (Gin server on :8080)
make backend

# Run tests
make test

# Format code
make fmt

# Run linting
make lint
```

### Building
```bash
# Build complete application
make build

# Build only frontend
make build-frontend

# Build only backend
make build-backend

# Run built application
make run
```

### Docker Deployment
```bash
# Build and run with Docker
make docker-build
make docker-run

# View logs
make docker-logs

# Stop services
make docker-stop

# Clean Docker resources
make docker-clean
```

### Health Check
```bash
# Check if services are running
make health-check

# Direct API health check
curl http://localhost:8080/api/v1/health
```

## Architecture Overview

### Backend (Go)
- **Framework**: Gin (REST API)
- **Database**: MySQL with GORM ORM
- **Configuration**: Viper for config management
- **Logging**: Zap for structured logging
- **Git Operations**: go-git library
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

## Development Workflow

1. **Local Development**:
   - Use `make deps` to install dependencies
   - Use `make dev` to start both frontend and backend
   - Frontend runs on :3000, backend on :8080

2. **Database Setup**:
   - MySQL 8.0+ required
   - Auto-migration on startup
   - Connection pooling configured

3. **Configuration**:
   - Copy `config.yaml.example` to `config.yaml`
   - Set up Git repository credentials
   - Configure Alibaba Cloud Registry access

4. **Testing**:
   - `make test` runs all tests
   - Coverage reports generated in `coverage.html`

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
    "branch": "main",
    "local_path": "/tmp/test-repo"
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



### 本地开发

### 开发机器平台
- 操作系统：Windows 11
- 处理器：AMD Ryzen 7 5800H
- 内存：16GB DDR4
- 显卡：RTX 3060 Laptop 6GB
- 存储：512GB SSD

### 前端启动：
```bash
cd web
npm install
npm run dev
```

### 后端启动：
```bash
go run main.go
```

### 本地开发日志
本地开发的后端日志存放在 `logs/app.log` 文件中。

### 启动开发环境时
需要固定后端端口8080，前端端口3000, 如果之前有其他服务占用这两个端口，请先把强制杀掉

### 代码修改后都必须重新启动前后端
