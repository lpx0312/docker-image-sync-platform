# API接口文档

## 概述

Docker镜像同步平台提供RESTful API接口，支持镜像同步、状态查询、GitHub Actions集成等功能。

### 基础信息

- **Base URL**: `http://localhost:8080/api/v1`
- **Content-Type**: `application/json`
- **字符编码**: `UTF-8`

### 响应格式

所有API响应都遵循统一的格式：

```json
{
  "code": 200,
  "message": "success",
  "data": {}
}
```

### 状态码说明

| 状态码 | 说明 |
|--------|------|
| 200 | 请求成功 |
| 400 | 请求参数错误 |
| 401 | 未授权访问 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

## 镜像管理 API

### 1. 获取镜像列表

获取所有镜像同步记录的分页列表。

**请求**
```http
GET /api/v1/images/list?page=1&page_size=10&status=&search=
```

**查询参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页数量，默认10，最大100 |
| status | string | 否 | 状态筛选：pending/syncing/success/failed |
| search | string | 否 | 搜索关键词，支持镜像名称模糊搜索 |

**响应示例**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "total": 150,
    "page": 1,
    "page_size": 10,
    "total_pages": 15,
    "items": [
      {
        "id": 1,
        "source_image": "nginx:latest",
        "target_image": "registry.cn-hangzhou.aliyuncs.com/namespace/nginx:latest",
        "platform": "linux/amd64",
        "status": "success",
        "description": "Web服务器镜像",
        "created_at": "2024-01-15T10:30:00Z",
        "updated_at": "2024-01-15T10:35:00Z",
        "sync_duration": 300,
        "error_message": "",
        "github_run_id": "123456789",
        "github_run_url": "https://github.com/user/repo/actions/runs/123456789"
      }
    ]
  }
}
```

### 2. 获取镜像详情

获取指定镜像的详细信息。

**请求**
```http
GET /api/v1/images/{id}
```

**路径参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 镜像记录ID |

**响应示例**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": 1,
    "source_image": "nginx:latest",
    "target_image": "registry.cn-hangzhou.aliyuncs.com/namespace/nginx:latest",
    "platform": "linux/amd64",
    "status": "success",
    "description": "Web服务器镜像",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:35:00Z",
    "sync_duration": 300,
    "error_message": "",
    "github_run_id": "123456789",
    "github_run_url": "https://github.com/user/repo/actions/runs/123456789",
    "sync_logs": [
      {
        "timestamp": "2024-01-15T10:30:00Z",
        "level": "info",
        "message": "开始同步镜像"
      },
      {
        "timestamp": "2024-01-15T10:35:00Z",
        "level": "info",
        "message": "同步完成"
      }
    ]
  }
}
```

### 3. 删除镜像记录

删除指定的镜像同步记录。

**请求**
```http
DELETE /api/v1/images/{id}
```

**路径参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 镜像记录ID |

**响应示例**
```json
{
  "code": 200,
  "message": "删除成功",
  "data": null
}
```

### 4. 获取统计信息

获取镜像同步的统计数据。

**请求**
```http
GET /api/v1/images/stats
```

**响应示例**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "total_images": 150,
    "success_count": 120,
    "failed_count": 15,
    "pending_count": 10,
    "syncing_count": 5,
    "success_rate": 80.0,
    "today_sync_count": 25,
    "this_week_sync_count": 100,
    "this_month_sync_count": 150
  }
}
```

### 5. 重试同步

重新执行失败的镜像同步任务。

**请求**
```http
POST /api/v1/images/{id}/retry
```

**路径参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int | 是 | 镜像记录ID |

**响应示例**
```json
{
  "code": 200,
  "message": "重试任务已提交",
  "data": {
    "task_id": "retry_123456",
    "status": "pending"
  }
}
```

## 同步操作 API

### 1. 提交同步任务

提交新的镜像同步任务。

**请求**
```http
POST /api/v1/sync/submit
```

**请求体**
```json
{
  "source_image": "nginx:latest",
  "target_tag": "nginx-custom",
  "platform": "linux/amd64",
  "description": "自定义Nginx镜像"
}
```

**请求参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| source_image | string | 是 | 源镜像地址，如：nginx:latest |
| target_tag | string | 否 | 目标标签，默认使用源镜像标签 |
| platform | string | 否 | 目标平台，默认linux/amd64 |
| description | string | 否 | 镜像描述信息 |

**响应示例**
```json
{
  "code": 200,
  "message": "同步任务已提交",
  "data": {
    "task_id": "sync_123456",
    "image_id": 151,
    "status": "pending",
    "estimated_duration": 300
  }
}
```

### 2. 批量提交同步任务

批量提交多个镜像同步任务。

**请求**
```http
POST /api/v1/sync/batch
```

**请求体**
```json
{
  "images": [
    {
      "source_image": "nginx:latest",
      "target_tag": "nginx-latest",
      "platform": "linux/amd64",
      "description": "Web服务器"
    },
    {
      "source_image": "redis:alpine",
      "target_tag": "redis-alpine",
      "platform": "linux/amd64",
      "description": "缓存服务器"
    }
  ],
  "concurrent_limit": 3
}
```

**请求参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| images | array | 是 | 镜像列表，每个元素包含source_image等字段 |
| concurrent_limit | int | 否 | 并发限制，默认3，最大10 |

**响应示例**
```json
{
  "code": 200,
  "message": "批量同步任务已提交",
  "data": {
    "batch_id": "batch_123456",
    "total_count": 2,
    "submitted_count": 2,
    "failed_count": 0,
    "task_ids": ["sync_123456", "sync_123457"]
  }
}
```

### 3. 获取同步历史

获取同步任务的历史记录。

**请求**
```http
GET /api/v1/sync/history?page=1&page_size=10&start_date=&end_date=
```

**查询参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页数量，默认10 |
| start_date | string | 否 | 开始日期，格式：2024-01-01 |
| end_date | string | 否 | 结束日期，格式：2024-01-31 |

**响应示例**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "total": 50,
    "page": 1,
    "page_size": 10,
    "items": [
      {
        "id": 1,
        "task_id": "sync_123456",
        "source_image": "nginx:latest",
        "status": "success",
        "start_time": "2024-01-15T10:30:00Z",
        "end_time": "2024-01-15T10:35:00Z",
        "duration": 300,
        "error_message": ""
      }
    ]
  }
}
```

## GitHub Actions API

### 1. 获取工作流列表

获取GitHub仓库的工作流列表。

**请求**
```http
GET /api/v1/github/workflows
```

**响应示例**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "total_count": 1,
    "workflows": [
      {
        "id": 123456,
        "name": "Docker Build and Push",
        "path": ".github/workflows/docker.yaml",
        "state": "active",
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-15T10:00:00Z",
        "url": "https://github.com/user/repo/actions/workflows/123456",
        "html_url": "https://github.com/user/repo/blob/main/.github/workflows/docker.yaml"
      }
    ]
  }
}
```

### 2. 获取工作流运行记录

获取指定工作流的运行记录。

**请求**
```http
GET /api/v1/github/workflows/{workflow_id}/runs?page=1&per_page=10&status=
```

**路径参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| workflow_id | int | 是 | 工作流ID |

**查询参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 否 | 页码，默认1 |
| per_page | int | 否 | 每页数量，默认10 |
| status | string | 否 | 状态筛选：queued/in_progress/completed |

**响应示例**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "total_count": 100,
    "workflow_runs": [
      {
        "id": 789012,
        "name": "Docker Build and Push",
        "head_branch": "main",
        "head_sha": "abc123def456",
        "status": "completed",
        "conclusion": "success",
        "workflow_id": 123456,
        "created_at": "2024-01-15T10:30:00Z",
        "updated_at": "2024-01-15T10:35:00Z",
        "run_started_at": "2024-01-15T10:30:30Z",
        "html_url": "https://github.com/user/repo/actions/runs/789012",
        "jobs_url": "https://api.github.com/repos/user/repo/actions/runs/789012/jobs"
      }
    ]
  }
}
```

### 3. 获取API限制信息

获取GitHub API的使用限制和剩余配额。

**请求**
```http
GET /api/v1/github/rate-limit
```

**响应示例**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "core": {
      "limit": 5000,
      "remaining": 4950,
      "reset": 1642248000,
      "used": 50,
      "resource": "core"
    },
    "search": {
      "limit": 30,
      "remaining": 30,
      "reset": 1642248000,
      "used": 0,
      "resource": "search"
    },
    "actions": {
      "limit": 1000,
      "remaining": 995,
      "reset": 1642248000,
      "used": 5,
      "resource": "actions"
    }
  }
}
```

### 4. 触发工作流

手动触发GitHub Actions工作流。

**请求**
```http
POST /api/v1/github/workflows/{workflow_id}/dispatches
```

**路径参数**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| workflow_id | int | 是 | 工作流ID |

**请求体**
```json
{
  "ref": "main",
  "inputs": {
    "image_list": "nginx:latest,redis:alpine"
  }
}
```

**响应示例**
```json
{
  "code": 200,
  "message": "工作流已触发",
  "data": {
    "dispatch_id": "dispatch_123456",
    "status": "triggered"
  }
}
```

## 系统监控 API

### 1. 健康检查

检查系统各组件的健康状态。

**请求**
```http
GET /api/v1/health
```

**响应示例**
```json
{
  "code": 200,
  "message": "系统健康",
  "data": {
    "status": "healthy",
    "timestamp": "2024-01-15T10:30:00Z",
    "version": "1.0.0",
    "uptime": 86400,
    "components": {
      "database": {
        "status": "healthy",
        "response_time": 5,
        "last_check": "2024-01-15T10:30:00Z"
      },
      "git_repository": {
        "status": "healthy",
        "last_sync": "2024-01-15T10:25:00Z",
        "last_check": "2024-01-15T10:30:00Z"
      },
      "github_api": {
        "status": "healthy",
        "rate_limit_remaining": 4950,
        "last_check": "2024-01-15T10:30:00Z"
      },
      "aliyun_registry": {
        "status": "healthy",
        "last_login": "2024-01-15T10:20:00Z",
        "last_check": "2024-01-15T10:30:00Z"
      }
    }
  }
}
```

### 2. 系统信息

获取系统运行信息和统计数据。

**请求**
```http
GET /api/v1/system/info
```

**响应示例**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "version": "1.0.0",
    "build_time": "2024-01-01T00:00:00Z",
    "go_version": "go1.21.5",
    "git_commit": "abc123def456",
    "uptime": 86400,
    "start_time": "2024-01-14T10:30:00Z",
    "memory_usage": {
      "alloc": 10485760,
      "total_alloc": 52428800,
      "sys": 20971520,
      "num_gc": 15
    },
    "goroutines": 25,
    "cpu_count": 8
  }
}
```

## 配置管理 API

### 1. 获取系统配置

获取当前系统配置信息（敏感信息已脱敏）。

**请求**
```http
GET /api/v1/config
```

**响应示例**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "server": {
      "port": 8080,
      "mode": "release",
      "host": "0.0.0.0"
    },
    "database": {
      "host": "localhost",
      "port": 3306,
      "database": "docker_sync",
      "max_idle_conns": 10,
      "max_open_conns": 100
    },
    "git": {
      "gitee": {
        "repo_url": "https://gitee.com/user/repo.git",
        "username": "user",
        "email": "user@example.com",
        "local_path": "./gitee-repo"
      },
      "github": {
        "repo_url": "https://github.com/user/repo",
        "username": "user",
        "local_path": "./github-repo"
      }
    },
    "aliyun": {
      "registry": "registry.cn-hangzhou.aliyuncs.com",
      "namespace": "namespace",
      "username": "user"
    },
    "sync": {
      "timeout_minutes": 30,
      "max_concurrent_jobs": 3,
      "max_retry_count": 3,
      "retry_interval_minutes": 5
    }
  }
}
```

### 2. 更新系统配置

更新系统配置（部分配置项支持热更新）。

**请求**
```http
PUT /api/v1/config
```

**请求体**
```json
{
  "sync": {
    "timeout_minutes": 45,
    "max_concurrent_jobs": 5
  },
  "log": {
    "level": "debug"
  }
}
```

**响应示例**
```json
{
  "code": 200,
  "message": "配置更新成功",
  "data": {
    "updated_fields": ["sync.timeout_minutes", "sync.max_concurrent_jobs", "log.level"],
    "requires_restart": false
  }
}
```

## 错误处理

### 错误响应格式

```json
{
  "code": 400,
  "message": "请求参数错误",
  "data": {
    "error": "validation_failed",
    "details": [
      {
        "field": "source_image",
        "message": "镜像地址不能为空"
      }
    ]
  }
}
```

### 常见错误码

| 错误码 | 说明 | 解决方案 |
|--------|------|----------|
| 1001 | 参数验证失败 | 检查请求参数格式和必填字段 |
| 1002 | 镜像地址无效 | 确认镜像地址格式正确 |
| 1003 | 同步任务已存在 | 等待现有任务完成或取消后重试 |
| 2001 | 数据库连接失败 | 检查数据库配置和网络连接 |
| 2002 | Git仓库访问失败 | 检查Git配置和访问权限 |
| 2003 | GitHub API调用失败 | 检查GitHub Token和API限制 |
| 2004 | 阿里云ACR访问失败 | 检查阿里云配置和网络连接 |
| 3001 | 系统资源不足 | 等待系统资源释放或调整配置 |
| 3002 | 并发限制超出 | 减少并发任务数量 |

## SDK 示例

### JavaScript/Node.js

```javascript
const axios = require('axios');

class DockerSyncAPI {
  constructor(baseURL = 'http://localhost:8080/api/v1') {
    this.client = axios.create({
      baseURL,
      timeout: 30000,
      headers: {
        'Content-Type': 'application/json'
      }
    });
  }

  // 提交同步任务
  async submitSync(sourceImage, options = {}) {
    const response = await this.client.post('/sync/submit', {
      source_image: sourceImage,
      ...options
    });
    return response.data;
  }

  // 获取镜像列表
  async getImages(page = 1, pageSize = 10, filters = {}) {
    const response = await this.client.get('/images/list', {
      params: {
        page,
        page_size: pageSize,
        ...filters
      }
    });
    return response.data;
  }

  // 获取统计信息
  async getStats() {
    const response = await this.client.get('/images/stats');
    return response.data;
  }
}

// 使用示例
const api = new DockerSyncAPI();

// 提交同步任务
api.submitSync('nginx:latest', {
  target_tag: 'nginx-custom',
  platform: 'linux/amd64',
  description: '自定义Nginx镜像'
}).then(result => {
  console.log('同步任务已提交:', result.data.task_id);
});

// 获取镜像列表
api.getImages(1, 10, { status: 'success' }).then(result => {
  console.log('成功同步的镜像:', result.data.items);
});
```

### Python

```python
import requests
import json

class DockerSyncAPI:
    def __init__(self, base_url='http://localhost:8080/api/v1'):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.headers.update({
            'Content-Type': 'application/json'
        })

    def submit_sync(self, source_image, **options):
        """提交同步任务"""
        data = {
            'source_image': source_image,
            **options
        }
        response = self.session.post(f'{self.base_url}/sync/submit', json=data)
        return response.json()

    def get_images(self, page=1, page_size=10, **filters):
        """获取镜像列表"""
        params = {
            'page': page,
            'page_size': page_size,
            **filters
        }
        response = self.session.get(f'{self.base_url}/images/list', params=params)
        return response.json()

    def get_stats(self):
        """获取统计信息"""
        response = self.session.get(f'{self.base_url}/images/stats')
        return response.json()

# 使用示例
api = DockerSyncAPI()

# 提交同步任务
result = api.submit_sync('nginx:latest', 
                        target_tag='nginx-custom',
                        platform='linux/amd64',
                        description='自定义Nginx镜像')
print(f"同步任务已提交: {result['data']['task_id']}")

# 获取镜像列表
result = api.get_images(page=1, page_size=10, status='success')
print(f"成功同步的镜像: {len(result['data']['items'])} 个")
```

### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
)

type DockerSyncAPI struct {
    BaseURL string
    Client  *http.Client
}

type SyncRequest struct {
    SourceImage string `json:"source_image"`
    TargetTag   string `json:"target_tag,omitempty"`
    Platform    string `json:"platform,omitempty"`
    Description string `json:"description,omitempty"`
}

func NewDockerSyncAPI(baseURL string) *DockerSyncAPI {
    return &DockerSyncAPI{
        BaseURL: baseURL,
        Client:  &http.Client{},
    }
}

func (api *DockerSyncAPI) SubmitSync(req SyncRequest) (map[string]interface{}, error) {
    data, _ := json.Marshal(req)
    resp, err := api.Client.Post(api.BaseURL+"/sync/submit", "application/json", bytes.NewBuffer(data))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    return result, nil
}

func (api *DockerSyncAPI) GetImages(page, pageSize int, filters map[string]string) (map[string]interface{}, error) {
    u, _ := url.Parse(api.BaseURL + "/images/list")
    q := u.Query()
    q.Set("page", fmt.Sprintf("%d", page))
    q.Set("page_size", fmt.Sprintf("%d", pageSize))
    for k, v := range filters {
        q.Set(k, v)
    }
    u.RawQuery = q.Encode()

    resp, err := api.Client.Get(u.String())
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    return result, nil
}

// 使用示例
func main() {
    api := NewDockerSyncAPI("http://localhost:8080/api/v1")

    // 提交同步任务
    result, _ := api.SubmitSync(SyncRequest{
        SourceImage: "nginx:latest",
        TargetTag:   "nginx-custom",
        Platform:    "linux/amd64",
        Description: "自定义Nginx镜像",
    })
    fmt.Printf("同步任务已提交: %v\n", result["data"])

    // 获取镜像列表
    result, _ = api.GetImages(1, 10, map[string]string{"status": "success"})
    fmt.Printf("获取镜像列表成功: %v\n", result["data"])
}
```

## 测试工具

### Postman Collection

可以导入以下Postman Collection进行API测试：

```json
{
  "info": {
    "name": "Docker Sync Platform API",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "item": [
    {
      "name": "镜像管理",
      "item": [
        {
          "name": "获取镜像列表",
          "request": {
            "method": "GET",
            "header": [],
            "url": {
              "raw": "{{baseUrl}}/api/v1/images/list?page=1&page_size=10",
              "host": ["{{baseUrl}}"],
              "path": ["api", "v1", "images", "list"],
              "query": [
                {"key": "page", "value": "1"},
                {"key": "page_size", "value": "10"}
              ]
            }
          }
        }
      ]
    }
  ],
  "variable": [
    {
      "key": "baseUrl",
      "value": "http://localhost:8080"
    }
  ]
}
```

### cURL 示例

```bash
# 获取镜像列表
curl -X GET "http://localhost:8080/api/v1/images/list?page=1&page_size=10" \
  -H "Content-Type: application/json"

# 提交同步任务
curl -X POST "http://localhost:8080/api/v1/sync/submit" \
  -H "Content-Type: application/json" \
  -d '{
    "source_image": "nginx:latest",
    "target_tag": "nginx-custom",
    "platform": "linux/amd64",
    "description": "自定义Nginx镜像"
  }'

# 获取统计信息
curl -X GET "http://localhost:8080/api/v1/images/stats" \
  -H "Content-Type: application/json"

# 健康检查
curl -X GET "http://localhost:8080/api/v1/health" \
  -H "Content-Type: application/json"
```