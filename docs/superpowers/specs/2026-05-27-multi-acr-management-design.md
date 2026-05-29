# 多 ACR 镜像仓库管理功能设计

## 1. 概述

### 1.1 背景

阿里云 ACR 个人版有 300 个镜像存储限制。用户需要管理多个 ACR 镜像仓库，以便：
- 分散存储镜像，避免单个 ACR 达到上限
- 按用途（生产、测试、开发）分类存储
- 灵活选择目标 ACR 进行同步

### 1.2 目标

- 支持添加和管理多个 ACR 配置
- 支持设置默认 ACR
- 同步时可选择目标 ACR
- 根据选择的 ACR 传递不同参数给 GitHub Action

## 2. 数据库设计

### 2.1 新建 acr_registries 表

```sql
CREATE TABLE `acr_registries` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `registry_url` VARCHAR(255) NOT NULL COMMENT '镜像仓库地址',
  `namespace` VARCHAR(100) NOT NULL COMMENT '命名空间',
  `username` VARCHAR(100) NOT NULL COMMENT '用户名',
  `password` VARCHAR(500) NOT NULL COMMENT '密码（加密存储）',
  `is_default` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否默认ACR',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` DATETIME DEFAULT NULL,
  PRIMARY KEY (`id`),
  INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='ACR镜像仓库配置表';
```

### 2.2 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT | 主键 |
| registry_url | VARCHAR | 镜像仓库地址（如 registry.cn-hangzhou.aliyuncs.com） |
| namespace | VARCHAR | 命名空间（如 lpx03） |
| username | VARCHAR | 用户名 |
| password | VARCHAR | 密码（加密存储） |
| is_default | TINYINT | 是否为默认 ACR（0=否，1=是） |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |
| deleted_at | DATETIME | 软删除时间 |

### 2.3 业务规则

- 每个用户可以有多个 ACR 配置
- 同一时间只能有一个默认 ACR
- 设置新的默认 ACR 时，自动取消旧的默认标记
- 删除 ACR 时，如果是默认 ACR，需要先取消默认或设置其他 ACR 为默认

## 3. 后端 API 设计

### 3.1 ACR 配置管理 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/acr-registries | 获取所有 ACR 配置列表 |
| POST | /api/v1/acr-registries | 添加新的 ACR 配置 |
| PUT | /api/v1/acr-registries/:id | 更新 ACR 配置 |
| DELETE | /api/v1/acr-registries/:id | 删除 ACR 配置 |
| PUT | /api/v1/acr-registries/:id/default | 设置为默认 ACR |
| GET | /api/v1/acr-registries/default | 获取默认 ACR 配置 |

### 3.2 请求/响应格式

#### 获取所有 ACR 配置

**响应：**
```json
{
  "status": "success",
  "data": [
    {
      "id": 1,
      "registry_url": "registry.cn-hangzhou.aliyuncs.com",
      "namespace": "lpx03",
      "username": "user1",
      "is_default": true,
      "created_at": "2026-05-27T10:00:00Z",
      "updated_at": "2026-05-27T10:00:00Z"
    },
    {
      "id": 2,
      "registry_url": "registry.cn-shanghai.aliyuncs.com",
      "namespace": "lpx04",
      "username": "user2",
      "is_default": false,
      "created_at": "2026-05-27T11:00:00Z",
      "updated_at": "2026-05-27T11:00:00Z"
    }
  ]
}
```

#### 添加 ACR 配置

**请求：**
```json
{
  "registry_url": "registry.cn-hangzhou.aliyuncs.com",
  "namespace": "lpx03",
  "username": "user1",
  "password": "your_password"
}
```

**响应：**
```json
{
  "status": "success",
  "data": {
    "id": 1,
    "registry_url": "registry.cn-hangzhou.aliyuncs.com",
    "namespace": "lpx03",
    "username": "user1",
    "is_default": false,
    "created_at": "2026-05-27T10:00:00Z",
    "updated_at": "2026-05-27T10:00:00Z"
  }
}
```

#### 设置默认 ACR

**响应：**
```json
{
  "status": "success",
  "message": "默认ACR已更新"
}
```

### 3.3 同步 API 修改

同步 API 需要新增 `acr_registry_id` 参数：

**POST /api/v1/sync/submit**
```json
{
  "images": ["nginx:1.27", "redis:7.0"],
  "acr_registry_id": 1,
  "architecture": "amd64"
}
```

**POST /api/v1/sync/batch**
```json
{
  "images": [
    {"source_image": "nginx:1.27"},
    {"source_image": "redis:7.0"}
  ],
  "acr_registry_id": 1,
  "max_concurrent": 3
}
```

## 4. 前端设计

### 4.1 配置页面（AliyunConfigForm.vue）

#### 布局结构

```
┌─────────────────────────────────────────────────────────────┐
│ 阿里云镜像仓库配置                               [添加新ACR] │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  默认 ACR：[下拉框选择默认 ACR                ▼]            │
│                                                             │
│  ┌─────────────────────────────────────────────────────────┐│
│  │ ACR 列表                                                ││
│  ├─────┬──────────────┬────────────┬──────────┬────────────┤│
│  │ 序号│ 镜像仓库地址  │ 命名空间   │ 用户名   │ 操作       ││
│  ├─────┼──────────────┼────────────┼──────────┼────────────┤│
│  │ 1   │ registry...  │ lpx03      │ user1    │ 编辑 删除  ││
│  │ 2   │ registry...  │ lpx04      │ user2    │ 编辑 删除  ││
│  └─────┴──────────────┴────────────┴──────────┴────────────┘│
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

#### 添加/编辑 ACR 对话框

```
┌─────────────────────────────────────────┐
│ 添加 ACR 配置                           │
├─────────────────────────────────────────┤
│                                         │
│ 镜像仓库地址：[                         ]│
│ 命名空间：    [                         ]│
│ 用户名：      [                         ]│
│ 密码：        [                         ]│
│                                         │
│        [测试连接]  [取消]  [确定]        │
│                                         │
└─────────────────────────────────────────┘
```

### 4.2 同步页面（SyncView.vue）

#### 单个同步表单

```
┌─────────────────────────────────────────────────────────────┐
│ 单镜像同步                                                  │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ 目标 ACR：  [下拉框选择 ACR                    ▼]           │
│             （显示命名空间，默认选中默认 ACR）               │
│                                                             │
│ 镜像名称：  [                                             ] │
│ 镜像标签：  [                                             ] │
│ 目标架构：  [amd64 ▼]                                       │
│                                                             │
│                                              [开始同步]     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

#### 批量同步表单

```
┌─────────────────────────────────────────────────────────────┐
│ 批量同步                                                    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ 目标 ACR：  [下拉框选择 ACR                    ▼]           │
│             （显示命名空间，默认选中默认 ACR）               │
│                                                             │
│ 镜像列表：  [                                             ] │
│             [                                             ] │
│             [                                             ] │
│             （每行一个镜像）                                │
│                                                             │
│                                              [开始同步]     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## 5. 数据流

### 5.1 配置管理流程

```
用户操作 → 前端 API 调用 → 后端处理 → 数据库更新 → 返回结果
```

### 5.2 同步流程

```
用户选择 ACR → 提交同步请求（含 acr_registry_id）
    → 后端获取 ACR 配置
    → 更新 images.txt
    → 触发 workflow_dispatch（传递 ACR 参数）
    → GitHub Action 执行同步
```

### 5.3 workflow_dispatch 参数

```json
{
  "ref": "main",
  "inputs": {
    "aliyun_registry": "registry.cn-hangzhou.aliyuncs.com",
    "aliyun_namespace": "lpx03",
    "aliyun_registry_user": "user1",
    "aliyun_registry_password": "password"
  }
}
```

## 6. 实现要点

### 6.1 密码加密

- ACR 密码使用现有加密服务（EncryptionService）加密存储
- 前端展示时显示占位符 `***`
- 更新密码时，如果密码为 `***` 则不更新

### 6.2 默认 ACR 逻辑

- 添加第一个 ACR 时自动设为默认
- 设置新默认 ACR 时，使用事务确保原子性：
  ```sql
  BEGIN;
  UPDATE acr_registries SET is_default = 0 WHERE is_default = 1;
  UPDATE acr_registries SET is_default = 1 WHERE id = ?;
  COMMIT;
  ```

### 6.3 删除保护

- 删除默认 ACR 时，提示用户先设置其他 ACR 为默认
- 或者自动将列表中的第一个 ACR 设为默认

### 6.4 同步时的 ACR 选择

- 前端在页面加载时获取 ACR 列表
- 下拉框默认选中默认 ACR
- 用户可以手动选择其他 ACR
- 提交时传递 `acr_registry_id` 给后端

## 7. 测试要点

### 7.1 功能测试

- 添加 ACR 配置
- 编辑 ACR 配置
- 删除 ACR 配置
- 设置默认 ACR
- 使用不同 ACR 进行同步

### 7.2 边界测试

- 删除唯一的 ACR
- 删除默认 ACR
- 添加重复的 ACR
- 密码为空的情况

### 7.3 集成测试

- 同步时参数正确传递给 GitHub Action
- workflow_dispatch 使用正确的 ACR 配置

## 8. 迁移计划

### 8.1 数据库迁移

1. 创建 `acr_registries` 表
2. 将现有阿里云配置迁移到新表（可选）
3. 更新同步记录表，添加 `acr_registry_id` 字段（可选）

### 8.2 代码迁移

1. 后端：新增 ACR 管理 API
2. 前端：修改配置页面和同步页面
3. 同步逻辑：修改为使用选择的 ACR 配置

### 8.3 兼容性

- 保留现有阿里云配置 API，标记为废弃
- 新功能使用新的 ACR 管理 API
- 逐步迁移现有配置到新表
