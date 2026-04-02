---
name: docker-image-sync-acr
description: >-
  Calls the Docker Image Sync Platform backend to submit single-image (POST /sync/submit)
  or multi-image (POST /sync/batch) sync jobs, polls GET /sync/status/:taskId until
  completion, and extracts ACR pull addresses from image records (acr_image field).
  Use when the user wants to sync Docker images via this project's API, obtain ACR
  mirror URLs after GitHub Actions sync, or automate curl/http workflows against
  localhost:8080 or a deployed API base URL.
---

# Docker 镜像同步平台 · ACR 地址获取

## 前提

- 后端已启动并可访问（默认 `http://192.168.0.180:10003`）。
- 基路径固定为 **`/api/v1`**。
- 提交接口**异步**执行；ACR 地址在镜像同步成功写入库后出现在状态接口的 **`acr_image`** 字段。
- 若用户未指定地址，使用环境变量 **`IMAGE_SYNC_API_BASE`**（例如 `http://192.168.0.180:10003/api/v1`），勿在末尾加多余斜杠。

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

## 一行示例（需 `curl`、`jq`）

```bash
BASE="${IMAGE_SYNC_API_BASE:-http://localhost:8080/api/v1}"
TASK=$(curl -sS -X POST "$BASE/sync/submit" -H "Content-Type: application/json" \
  -d '{"images":["nginx:latest"],"architecture":"amd64"}' | jq -r '.task_id')
# 轮询略；终态后：
curl -sS "$BASE/sync/status/$TASK" | jq '.images.records[] | {source:.original_image, acr:.acr_image, ok:.sync_status}'
```

## 与本仓库的关系

技能对应本仓库后端实现：`internal/handlers/sync.go`（`SubmitSync`、`SubmitBatchSync`、`GetSyncStatus`），模型字段 `acr_image` 见 `internal/models/models.go`。

## 在 Cursor 中启用

项目级技能通常放在 **`.cursor/skills/<skill-name>/`**。若本文件仅在仓库 `skill/` 目录下，可将本目录复制或软链到 `.cursor/skills/docker-image-sync-acr/`，以便按描述自动匹配。
