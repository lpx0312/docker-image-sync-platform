---
name: docker-image-sync-acr
description: >-
  将 Docker 镜像同步到阿里云容器镜像服务(ACR)。支持单镜像和批量同步，自动轮询状态并返回 ACR 拉取地址。

  触发词（中文）：同步镜像、镜像同步、docker镜像同步、ACR同步、阿里云镜像同步、同步到ACR、镜像搬运、
  镜像迁移、把xxx镜像同步、帮我同步、拉取镜像到ACR、同步nginx、同步redis、同步某个镜像、镜像转存、
  Docker Hub到阿里云、海外镜像同步、gcr.io同步、quay.io同步、ghcr.io同步、k8s镜像同步、gcr镜像、
  quay镜像、ghcr镜像、docker.io镜像同步。

  触发词（英文）：sync docker image、sync image to acr、image sync、mirror docker image、
  docker image sync、sync nginx、sync redis、sync to aliyun、acr sync、container image sync。

  使用场景：(1) 用户想将 Docker Hub/Google Container Registry/Quay/GitHub Container Registry
  等海外镜像同步到阿里云 ACR；(2) 用户提到"同步"和"镜像"的组合；(3) 用户需要获取镜像的 ACR 拉取地址；
  (4) 用户提到"ACR"、"阿里云镜像"相关操作；(5) 批量同步多个镜像。

  后端 API：POST /sync/submit（单镜像）、POST /sync/batch（批量）、GET /sync/status/:taskId（查询状态）。
---

# Docker 镜像同步平台 · ACR 地址获取

## 前提

- 后端已启动并可访问（默认 `http://192.168.0.180:10003`）。
- 基路径固定为 **`/api/v1`**。
- **所有业务接口需要 JWT 认证**，必须先登录获取 Token，然后在每个请求中携带 `Authorization: Bearer <token>` 头。
- 提交接口**异步**执行；ACR 地址在镜像同步成功写入库后出现在状态接口的 **`acr_image`** 字段。
- 若用户未指定地址，使用环境变量 **`IMAGE_SYNC_API_BASE`**（例如 `http://192.168.0.180:10003/api/v1`），勿在末尾加多余斜杠。

## 认证（必须先执行）

**登录获取 Token**

- `POST {BASE}/auth/login`
- JSON 体：

```json
{
  "username": "admin",
  "password": "admin123"
}
```

- 也可通过环境变量 **`IMAGE_SYNC_USERNAME`** 和 **`IMAGE_SYNC_PASSWORD`** 覆盖默认账密。
- 响应含 **`token`** 字段，后续所有请求需在 Header 中携带 `Authorization: Bearer <token>`。
- Token 有效期默认 24 小时，无需频繁登录。

**Shell 获取 Token 示例**

```bash
BASE="${IMAGE_SYNC_API_BASE:-http://localhost:8080/api/v1}"
USERNAME="${IMAGE_SYNC_USERNAME:-admin}"
PASSWORD="${IMAGE_SYNC_PASSWORD:-admin123}"
TOKEN=$(curl -sS -X POST "$BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" | jq -r '.token')
```

后续所有 curl 请求加上 `-H "Authorization: Bearer $TOKEN"`。

## 单镜像同步

**请求**

- `POST {BASE}/sync/submit`
- JSON 体：

```json
{
  "images": ["nginx:latest"],
  "architecture": "amd64",
  "description": "optional"
}
```

- `images`：字符串数组，每项可为 `repo:tag`；架构可选 `amd64` / `arm64`（默认与后端一致）。

**响应（200）**：含 `task_id`、`status`、`message` 等；记下 **`task_id`**。

## 多镜像（批量）同步

**请求**

- `POST {BASE}/sync/batch`
- JSON 体：

```json
{
  "images": [
    {
      "source_image": "nginx:latest",
      "target_tag": "",
      "architecture": "amd64",
      "priority": 0,
      "description": ""
    }
  ],
  "max_concurrent": 3,
  "auto_retry": false,
  "retry_count": 0
}
```

- 每项至少 **`source_image`**；可选 **`target_tag`**、**`architecture`**、**`priority`**、**`description`**。
- **`max_concurrent`**：1–10，默认 3。

**响应（200）**：含 **`task_id`** 等。

## 查询状态并取 ACR 地址

**请求**

- `GET {BASE}/sync/status/{taskId}`

**响应要点**

- **`status`**：任务状态，常见 `pending`、`running`、`completed`、`failed`、`partial_success` 等；轮询直至进入**终态**（如 `completed`、`failed`、`partial_success`）。
- **`images.records`**：镜像记录数组；每条为 `ImageSyncRecord` 结构。
- **ACR 拉取地址**：每条记录的 **`acr_image`**（JSON 字段名）。同步未完成时可能为空字符串。
- 同时可参考 **`original_image`**、**`tag`**、**`architecture`**、**`sync_status`**（单条镜像状态：`pending` / `syncing` / `success` / `failed` 等）、**`error_message`**。

## 轮询建议

1. 提交后睡眠 3–10 秒再首次查询（镜像大或 Actions 慢时需更长）。
2. 间隔 5–15 秒轮询 `GET /sync/status/:taskId`，超时例如 30–60 分钟（按镜像数量与用户预期调整）。
3. 终态后汇总：对所有 `images.records` 中 **`sync_status == "success"`** 的项读取 **`acr_image`**；失败项读 **`error_message`**。

## 说明

- **`GET /api/v1/sync/batch/status/:taskId`** 在后端为**废弃**接口（410），不要使用；单任务与批量任务均用 **`GET /sync/status/:taskId`**。
- 健康检查：`GET` 去掉 `/api/v1` 前缀后的根路径健康接口见项目路由（如 `/api/v1/health`）。

## 完整示例（需 `curl`、`jq`）

```bash
BASE="${IMAGE_SYNC_API_BASE:-http://localhost:8080/api/v1}"
USERNAME="${IMAGE_SYNC_USERNAME:-admin}"
PASSWORD="${IMAGE_SYNC_PASSWORD:-admin123}"

# 1. 登录获取 Token
TOKEN=$(curl -sS -X POST "$BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" | jq -r '.token')

# 2. 提交同步
TASK=$(curl -sS -X POST "$BASE/sync/submit" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"images":["nginx:latest"],"architecture":"amd64"}' | jq -r '.task_id')

# 3. 轮询略；终态后查询结果：
curl -sS -H "Authorization: Bearer $TOKEN" "$BASE/sync/status/$TASK" \
  | jq '.images.records[] | {source:.original_image, acr:.acr_image, ok:.sync_status}'
```

## 与本仓库的关系

技能对应本仓库后端实现：`internal/handlers/sync.go`（`SubmitSync`、`SubmitBatchSync`、`GetSyncStatus`），模型字段 `acr_image` 见 `internal/models/models.go`。

## 在 Cursor 中启用

项目级技能通常放在 **`.cursor/skills/<skill-name>/`**。若本文件仅在仓库 `skill/` 目录下，可将本目录复制或软链到 `.cursor/skills/docker-image-sync-acr/`，以便按描述自动匹配。
