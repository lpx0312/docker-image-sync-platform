# ACR 镜像管理功能设计

## 1. 概述

### 1.1 背景

用户需要管理存储在阿里云 ACR 中的镜像，包括：
- 查看 ACR 中的镜像列表和 tag 信息
- 手动添加镜像名称到管理列表
- 从同步记录中自动提取镜像名称
- 查看镜像的架构、digest、大小等详细信息

### 1.2 目标

- 提供独立的镜像管理页面
- 支持选择不同的 ACR 配置
- 支持手动和自动添加镜像名称
- 调用 ACR API 获取镜像的 tag 详细信息
- 支持搜索和筛选功能

## 2. 数据库设计

### 2.1 新建 acr_repositories 表

```sql
CREATE TABLE `acr_repositories` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `acr_registry_id` BIGINT UNSIGNED NOT NULL COMMENT '关联的ACR配置ID',
  `repository_name` VARCHAR(255) NOT NULL COMMENT '镜像名称（不含tag）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` DATETIME DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_acr_repo` (`acr_registry_id`, `repository_name`),
  INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='ACR镜像仓库列表';
```

### 2.2 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 主键 |
| acr_registry_id | BIGINT | 关联的 ACR 配置 ID |
| repository_name | VARCHAR | 镜像名称（如 nginx、library/mysql） |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |
| deleted_at | DATETIME | 软删除时间 |

### 2.3 业务规则

- 同一个 ACR 下，镜像名称唯一
- 删除 ACR 配置时，关联的镜像记录也应删除（级联删除）
- 从同步记录提取时，自动去重

## 3. 后端 API 设计

### 3.1 ACR 镜像管理 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/acr-repositories | 获取指定 ACR 的镜像列表 |
| POST | /api/v1/acr-repositories | 添加单个镜像名称 |
| POST | /api/v1/acr-repositories/batch | 批量添加镜像名称 |
| DELETE | /api/v1/acr-repositories/:id | 删除镜像名称 |
| POST | /api/v1/acr-repositories/sync-from-records | 从同步记录中提取镜像名称 |

### 3.2 ACR Tag 查询 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/acr-tags | 获取指定镜像的 tag 列表 |
| GET | /api/v1/acr-tags/:tag/detail | 获取指定 tag 的详细信息 |

### 3.3 请求/响应格式

#### 获取镜像列表

**请求参数：**
- `acr_registry_id` (必填): ACR 配置 ID

**响应：**
```json
{
  "status": "success",
  "data": [
    {
      "id": 1,
      "acr_registry_id": 1,
      "repository_name": "nginx",
      "created_at": "2026-05-28T10:00:00Z"
    }
  ]
}
```

#### 添加镜像名称

**请求：**
```json
{
  "acr_registry_id": 1,
  "repository_name": "nginx"
}
```

#### 批量添加镜像名称

**请求：**
```json
{
  "acr_registry_id": 1,
  "repository_names": ["nginx", "redis", "mysql"]
}
```

#### 从同步记录提取

**请求：**
```json
{
  "acr_registry_id": 1
}
```

#### 获取 Tag 列表

**请求参数：**
- `acr_registry_id` (必填): ACR 配置 ID
- `repository_name` (必填): 镜像名称

**响应：**
```json
{
  "status": "success",
  "data": ["1.27.0", "1.27.1", "latest"]
}
```

#### 获取 Tag 详细信息

**请求参数：**
- `acr_registry_id` (必填): ACR 配置 ID
- `repository_name` (必填): 镜像名称
- `tag` (必填): Tag 名称

**响应：**
```json
{
  "status": "success",
  "data": {
    "tag": "1.27.1",
    "architectures": ["amd64", "arm64"],
    "digests": {
      "amd64": "sha256:abc123...",
      "arm64": "sha256:def456..."
    },
    "sizes": {
      "amd64": 1024000,
      "arm64": 1048576
    },
    "pushed_at": {
      "amd64": "2026-05-28T10:00:00Z",
      "arm64": "2026-05-28T10:00:00Z"
    }
  }
}
```

## 4. ACR API 集成

### 4.1 认证

参考 `acr.sh` 脚本，使用以下方式获取 Token：

```go
// 获取 Token
func getACRToken(registry, username, password, service, scope string) (string, error) {
    // POST https://dockerauth.{region}.aliyuncs.com/auth
    // 参数：service, scope
    // 认证：Basic Auth (username:password)
}
```

### 4.2 获取 Tag 列表

```go
// GET https://{registry}/v2/{namespace}/{repo}/tags/list
// Header: Authorization: Bearer {token}
func getTags(registry, namespace, repo, token string) ([]string, error)
```

### 4.3 获取 Manifest（架构、digest、大小）

```go
// GET https://{registry}/v2/{namespace}/{repo}/manifests/{tag}
// Header: Authorization: Bearer {token}
// Header: Accept: application/vnd.oci.image.index.v1+json,...
func getManifest(registry, namespace, repo, tag, token string) (*Manifest, error)
```

### 4.4 配置信息

从数据库获取 ACR 配置：
- `registry_url`: 镜像仓库地址
- `namespace`: 命名空间
- `username`: 用户名
- `password`: 密码（解密后使用）

## 5. 前端设计

### 5.1 页面结构

```
┌─────────────────────────────────────────────────────────────┐
│ 镜像管理                                                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ 选择 ACR：[下拉框选择 ACR                    ▼]  [刷新]     │
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 镜像列表                                                │ │
│ ├─────┬──────────────┬──────────────┬─────────────────────┤ │
│ │ 序号│ 镜像名称      │ 创建时间     │ 操作               │ │
│ ├─────┼──────────────┼──────────────┼─────────────────────┤ │
│ │ 1   │ nginx        │ 2026-05-28   │ [添加Tag] [删除]    │ │
│ │ 2   │ redis        │ 2026-05-28   │ [添加Tag] [删除]    │ │
│ └─────┴──────────────┴──────────────┴─────────────────────┘ │
│                                                             │
│ [添加镜像] [批量添加] [从同步记录导入]                      │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 5.2 Tag 详情弹窗

```
┌─────────────────────────────────────────────────────────────┐
│ nginx - Tag 列表                                            │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ 搜索：[Tag名称] [SHA256] [架构: amd64/arm64 ▼]             │
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Tag      │ 架构        │ Digest      │ 大小   │ 操作   │ │
│ ├──────────┼─────────────┼─────────────┼────────┼────────┤ │
│ │ 1.27.1   │ amd64       │ sha256:abc..│ 50MB   │ [复制] │ │
│ │          │ arm64       │ sha256:def..│ 52MB   │        │ │
│ │ latest   │ amd64       │ sha256:ghi..│ 50MB   │ [复制] │ │
│ └──────────┴─────────────┴─────────────┴────────┴────────┘ │
│                                                             │
│ [关闭]                                                      │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 5.3 添加镜像对话框

```
┌─────────────────────────────────────────┐
│ 添加镜像                                │
├─────────────────────────────────────────┤
│                                         │
│ 镜像名称：[                            ]│
│                                         │
│ 提示：输入镜像名称，不含tag和registry   │
│                                         │
│        [取消]  [确定]                   │
│                                         │
└─────────────────────────────────────────┘
```

### 5.4 批量添加对话框

```
┌─────────────────────────────────────────┐
│ 批量添加镜像                            │
├─────────────────────────────────────────┤
│                                         │
│ 镜像列表：                              │
│ [                                     ] │
│ [                                     ] │
│ 每行一个镜像名称                        │
│                                         │
│        [取消]  [确定]                   │
│                                         │
└─────────────────────────────────────────┘
```

## 6. 数据流

### 6.1 手动添加流程

```
用户输入镜像名称 → 前端验证 → 调用 API → 保存到数据库 → 刷新列表
```

### 6.2 自动添加流程（从同步记录导入）

```
用户点击"从同步记录导入" → 调用 API → 查询同步记录 → 提取镜像名称 → 去重 → 保存到数据库 → 返回结果
```

### 6.3 查看 Tag 流程

```
用户点击镜像名称 → 打开 Tag 详情弹窗 → 调用 ACR API 获取 Tag 列表 → 展示列表
用户点击某个 Tag → 调用 ACR API 获取详细信息 → 展示架构、digest、大小等
```

## 7. 实现要点

### 7.1 ACR API 调用

- Token 缓存：避免频繁请求 Token
- 错误处理：ACR API 调用失败时的友好提示
- 超时设置：合理设置 API 调用超时时间

### 7.2 自动添加逻辑

从同步记录中提取镜像名称：
```go
// 查询指定 ACR 的成功同步记录
var records []ImageSyncRecord
db.Where("acr_registry_id = ? AND sync_status = ?", acrRegistryID, "success").
   Find(&records)

// 提取镜像名称（不含 tag）
for _, record := range records {
    repoName := extractRepoName(record.OriginalImage)
    // 去重后添加到 acr_repositories 表
}
```

### 7.3 前端组件结构

```
web/src/views/ImagesManageView.vue    # 主页面
web/src/components/AcrRepositoryList.vue  # 镜像列表组件
web/src/components/AcrTagList.vue         # Tag 列表组件
web/src/components/AddRepositoryDialog.vue # 添加镜像对话框
```

## 8. 测试要点

### 8.1 功能测试

- 添加镜像名称
- 批量添加镜像名称
- 从同步记录导入
- 删除镜像名称
- 查看 Tag 列表
- 查看 Tag 详细信息

### 8.2 边界测试

- 添加重复的镜像名称
- 删除不存在的镜像
- ACR API 调用失败
- 空列表展示

### 8.3 集成测试

- ACR API Token 获取
- Tag 列表获取
- Manifest 详细信息获取
