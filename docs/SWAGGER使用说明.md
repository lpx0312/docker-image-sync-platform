# Swagger API 文档使用说明

## 概述

本项目已集成 Swagger API 文档功能，提供完整的交互式 API 文档界面，方便开发人员在线浏览和调试所有后端接口。

## 访问方式

### 1. Swagger UI 界面（推荐）
- **URL**: `http://localhost:8080/api/v1/docs.html`
- **功能**: 美观的交互式 API 文档界面，支持在线测试所有接口
- **特点**:
  - 支持在线 API 测试（Try it out）
  - 详细的请求参数说明
  - 完整的响应示例和数据模型
  - 支持按标签分组查看（Sync / Images / GitHub / Config / Health）
  - 支持接口搜索和过滤

### 2. Swagger JSON 规范文件
- **URL**: `http://localhost:8080/api/v1/swagger.json`
- **功能**: 获取原始的 OpenAPI 2.0 规范文件
- **用途**: 可导入 Postman、Insomnia 等 API 工具，或集成到 CI/CD 流程

### 3. 文档静态资源目录
- **URL**: `http://localhost:8080/api/v1/docs/`
- **功能**: 访问 `docs/` 目录下的所有静态文件

## 支持的 API 接口

### 镜像同步 (Sync)
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/sync/submit` | 提交单个镜像同步任务 |
| POST | `/api/v1/sync/batch` | 提交批量镜像同步任务 |
| POST | `/api/v1/sync/batch/mock` | 提交模拟批量同步任务（测试用） |
| GET  | `/api/v1/sync/status/{taskId}` | 查询单个同步任务状态 |
| GET  | `/api/v1/sync/batch/status/{taskId}` | 查询批量同步任务状态 |
| GET  | `/api/v1/sync/history` | 获取同步历史记录 |

### 镜像管理 (Images)
| 方法 | 路径 | 说明 |
|------|------|------|
| GET    | `/api/v1/images/list` | 获取镜像列表（分页+筛选） |
| GET    | `/api/v1/images/stats` | 获取各状态镜像数量统计 |
| POST   | `/api/v1/images/batch-check` | 批量检查镜像是否存在于ACR |
| GET    | `/api/v1/images/{id}` | 获取单个镜像详情 |
| DELETE | `/api/v1/images/{id}` | 删除镜像记录（软删除） |
| POST   | `/api/v1/images/{id}/retry` | 重试失败的镜像同步 |
| POST   | `/api/v1/images/{id}/check` | 检查单个镜像是否存在于ACR |

### GitHub Actions (GitHub)
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/github/runs` | 获取工作流运行列表 |
| GET | `/api/v1/github/runs/{runId}` | 获取工作流运行详情 |
| GET | `/api/v1/github/rate-limit` | 查询 GitHub API 速率限制 |

### 系统配置 (Config)
| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/api/v1/config/status` | 获取配置状态（不含敏感信息） |
| GET  | `/api/v1/config/all` | 获取所有系统配置 |
| GET  | `/api/v1/config/debug/{key}` | 调试：获取指定配置项 |
| GET  | `/api/v1/config/git-repository` | 获取当前Git仓库类型 |
| PUT  | `/api/v1/config/git-repository` | 切换Git仓库类型（gitee/github） |
| GET  | `/api/v1/config/git` | 获取Git详细配置 |
| PUT  | `/api/v1/config/git/gitee` | 更新Gitee配置 |
| PUT  | `/api/v1/config/git/github` | 更新GitHub配置 |
| POST | `/api/v1/config/git/test` | 测试Git连接 |
| POST | `/api/v1/config/git-test-operations` | 测试Git代码拉取和提交操作 |
| GET  | `/api/v1/config/git-optimization` | 获取Git优化配置 |
| PUT  | `/api/v1/config/git-optimization` | 更新Git优化配置 |
| GET  | `/api/v1/config/git-performance` | 获取Git性能指标 |
| GET  | `/api/v1/config/git-network-test` | 测试Git网络质量 |
| GET  | `/api/v1/config/aliyun-db` | 获取阿里云ACR配置 |
| PUT  | `/api/v1/config/aliyun-db` | 更新阿里云ACR配置 |
| POST | `/api/v1/config/aliyun/test` | 测试阿里云ACR连接 |

### 健康检查 (Health)
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/health` | 系统健康检查 |

## 使用方法

### 1. 在浏览器中访问
直接打开 `http://localhost:8080/api/v1/docs.html` 即可看到完整的 API 文档界面。

### 2. 在线测试 API 接口
1. 在 Swagger UI 中找到要测试的接口
2. 点击接口展开详情
3. 点击 **"Try it out"** 按钮
4. 填写请求参数（路径参数、查询参数、请求体等）
5. 点击 **"Execute"** 执行请求
6. 在下方查看响应状态码、响应体和请求 curl 命令

### 3. 导入到 API 工具
- **Postman**: 使用 Import → From URL → 填入 `http://localhost:8080/api/v1/swagger.json`
- **Insomnia**: 使用 Import → From URL → 填入同上

## 技术实现

### 文件结构
```
docs/
├── swagger.json          # OpenAPI 2.0 规范文件（API定义）
├── swagger-ui.html       # Swagger UI 界面入口
└── SWAGGER使用说明.md     # 本文档
```

### 路由配置（main.go）
```go
// 访问 docs 目录下所有静态文件
api.Static("/docs", "./docs")

// 直接访问 Swagger UI 页面
api.StaticFile("/docs.html", "./docs/swagger-ui.html")

// 获取 OpenAPI JSON 规范文件
api.StaticFile("/swagger.json", "./docs/swagger.json")
```

## 注意事项

1. **服务启动**: 确保后端服务已在 8080 端口正常启动
2. **跨域访问**: Swagger UI 已配置支持跨域请求，可从任意域名访问
3. **请求认证**: 当前版本无鉴权机制，所有接口可直接测试
4. **数据安全**: 测试时请勿在生产环境使用真实的敏感凭据（token、密码等）

## 更新维护

### 添加或修改 API 文档
1. 在 `main.go` 中新增或修改路由
2. 在 `docs/swagger.json` 的 `paths` 中添加/修改对应的路径定义
3. 如有新的请求/响应数据结构，在 `definitions` 中补充
4. 重启后端服务使更改生效

### 更新 API 规范文件
`docs/swagger.json` 遵循 [OpenAPI 2.0 (Swagger) 规范](https://swagger.io/specification/v2/)，
可使用 [Swagger Editor](https://editor.swagger.io/) 在线编辑和验证。

## 故障排除

| 问题 | 解决方案 |
|------|----------|
| 无法访问 Swagger UI | 检查后端服务是否已启动（`ss -ltnp \| grep 8080`） |
| 页面样式错乱 | 检查网络能否访问 unpkg.com CDN，或改用本地资源 |
| API 调用返回错误 | 检查请求参数格式，查看后端日志 `logs/app.log` |
| 页面显示异常 | 清除浏览器缓存后重新访问 |

---

**版本**: 1.0.0  
**更新时间**: 2026-03-30  
**维护团队**: Docker Image Sync Platform Team
