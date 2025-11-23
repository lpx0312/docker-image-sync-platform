# Swagger API 文档使用说明

## 概述

本项目已成功集成 Swagger API 文档功能，提供了完整的交互式 API 文档界面，方便开发人员测试和调试后端接口。

## 访问方式

### 1. Swagger UI 界面
- **URL**: `http://localhost:8080/api/v1/docs.html`
- **功能**: 提供美观的交互式 API 文档界面
- **特点**:
  - 支持在线 API 测试
  - 详细的请求参数说明
  - 完整的响应示例
  - 支持按标签分组查看

### 2. Swagger JSON 配置
- **URL**: `http://localhost:8080/api/v1/swagger.json`
- **功能**: 获取原始的 Swagger 配置文件
- **用途**: 可用于集成到其他 API 文档工具

### 3. 文档静态资源
- **URL**: `http://localhost:8080/api/v1/docs/`
- **功能**: 访问 docs 目录下的所有静态文件

## 支持的 API 接口

### 镜像同步 (Sync)
- `POST /api/v1/sync/submit` - 提交单个镜像同步任务
- `POST /api/v1/sync/batch` - 提交批量镜像同步任务
- `GET /api/v1/sync/status/{taskId}` - 查询同步任务状态
- `GET /api/v1/sync/history` - 获取同步历史记录

### 镜像管理 (Images)
- `GET /api/v1/images/list` - 获取镜像列表
- `GET /api/v1/images/{id}` - 获取镜像详情
- `DELETE /api/v1/images/{id}` - 删除镜像记录
- `POST /api/v1/images/{id}/retry` - 重试镜像同步

### GitHub 集成 (GitHub)
- `GET /api/v1/github/runs` - 获取 GitHub Actions 工作流运行列表
- `GET /api/v1/github/runs/{runId}` - 获取工作流运行详情

### 系统配置 (Config)
- `GET /api/v1/config/all` - 获取所有系统配置
- `GET /api/v1/config/git` - 获取 Git 配置
- `PUT /api/v1/config/git/github` - 更新 GitHub 配置
- `PUT /api/v1/config/git/gitee` - 更新 Gitee 配置
- `POST /api/v1/config/git/test` - 测试 Git 连接
- `GET /api/v1/config/aliyun-db` - 获取阿里云配置
- `PUT /api/v1/config/aliyun-db` - 更新阿里云配置
- `POST /api/v1/config/aliyun/test` - 测试阿里云连接

### 健康检查 (Health)
- `GET /api/v1/health` - 系统健康检查

## 使用方法

### 1. 在浏览器中访问
直接打开 `http://localhost:8080/api/v1/docs.html` 即可看到完整的 API 文档界面。

### 2. 测试 API 接口
1. 在 Swagger UI 中找到要测试的接口
2. 点击展开接口详情
3. 点击 "Try it out" 按钮
4. 填写请求参数
5. 点击 "Execute" 执行请求
6. 查看响应结果

### 3. 查看接口文档
每个接口都包含：
- 请求方法 (GET, POST, PUT, DELETE)
- 请求路径和参数
- 请求体格式
- 响应状态码和格式
- 详细的功能描述

## 技术实现

### 文件结构
```
docs/
├── swagger.json          # Swagger API 配置文件
├── swagger-ui.html       # Swagger UI 界面文件
└── SWAGGER使用说明.md     # 本使用说明文档
```

### 路由配置
在 `main.go` 中添加了以下路由：
```go
// GET /api/v1/docs - Swagger API文档界面
api.Static("/docs", "./docs")

// GET /api/v1/docs.html - 直接访问Swagger UI
api.StaticFile("/docs.html", "./docs/swagger-ui.html")

// GET /api/v1/swagger.json - 获取Swagger JSON配置
api.StaticFile("/swagger.json", "./docs/swagger.json")
```

## 注意事项

1. **服务启动**: 使用 Swagger 功能前，请确保后端服务已正常启动在 8080 端口
2. **跨域访问**: Swagger UI 已配置支持跨域请求，可以在任何域名下访问
3. **请求认证**: 当前版本暂未实现复杂的认证机制，可直接测试所有接口
4. **数据安全**: 测试时请注意不要在生产环境中使用真实的敏感数据

## 更新维护

### 添加新的 API 文档
1. 更新 `docs/swagger.json` 文件
2. 在 `paths` 中添加新的 API 路径定义
3. 在 `definitions` 中添加相关的数据模型定义
4. 重启服务使更改生效

### 自定义 Swagger UI
可以通过修改 `docs/swagger-ui.html` 来自定义 Swagger UI 的外观和功能。

## 故障排除

### 常见问题
1. **无法访问 Swagger UI**: 检查后端服务是否正常启动
2. **API 调用失败**: 检查请求参数是否正确，服务是否正常运行
3. **页面显示异常**: 清除浏览器缓存后重新访问

### 联系支持
如果遇到问题，请检查：
- 后端服务日志 (`logs/app.log`)
- 控制台输出信息
- 浏览器开发者工具中的错误信息

---

**版本**: 1.0.0
**更新时间**: 2025-11-23
**维护团队**: Docker Image Sync Platform Team