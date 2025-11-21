## 结论
- 仅用 Vercel（前端）+ Supabase（后端与数据库）可以实现你的项目核心需求，但需要对后端进行较大改造（由 Go/Gin/MySQL 迁移到 Supabase Edge Functions + Postgres），并将镜像实际同步动作继续交给 GitHub Actions 完成。
- 关键点：你当前代码已支持“API模式”更新 images.txt（无需 git clone），并通过 GitHub Actions 执行镜像同步。这一点非常适合在 Supabase Edge Functions 中复用，避免在无服务器环境中做 Docker 操作。

## 架构变更概览
- 前端：部署在 Vercel，仍用现有 Vue/Vite 前端；通过重写/环境变量将 `/api/v1` 指向 Supabase Edge Functions 的 HTTP 入口。
- 后端：用 Supabase Edge Functions（Deno/TypeScript）重写现有 Go API 的主要端点（同步提交、任务状态、配置管理、镜像查询统计等）。
- 数据库：将 `MySQL` 表结构迁移到 `Supabase Postgres`（保留等价的表与索引）。敏感配置（如 token/password）通过 Supabase 密钥与应用层加密保存。
- 同步执行：继续使用 GitHub Actions 完成镜像拉取与推送 ACR。Edge Functions 负责写入 `images.txt` 到仓库（GitHub/Gitee 内容 API）并做状态轮询（定时任务）。

## 后端替换方案（端点映射）
- `/api/v1/sync/submit`、`/api/v1/sync/batch`：Edge Function 接收请求，写数据库任务/镜像记录，生成 `images.txt` 内容并调用 Git 内容 API 提交，返回任务ID。
- `/api/v1/sync/status/:taskId`：查询 Postgres 中任务与镜像记录的实时状态。
- `/api/v1/sync/history`：分页查询任务历史。
- `/api/v1/config/*`：读取/更新系统配置（Postgres），保留“环境变量优先级”设计，必要时在 Edge Functions 内融合 Supabase `secrets`。
- `/api/v1/images/*`：列表、详情、删除、统计、重试、存在性检查等，均改为访问 Postgres；存在性检查通过 OCI Registry HTTP/Manifest 接口（JS/TS库或直接请求）实现。
- `/api/v1/github/*`：通过 GitHub REST API 获取工作流运行信息与速率限制。

## 数据库迁移（MySQL → Postgres）
- 表：
  - `system_configs`（键值对 + 加密标记 + 分组/显示顺序）
  - `image_sync_records`（镜像记录：状态、错误、时间、架构、大小、顺序等）
  - `sync_tasks`（任务：状态、并发、统计、GitHub 运行ID/URL、commit SHA 等）
- 字段与索引：保持等价（枚举可用 `CHECK` 或 `TEXT + 约束`）。时间精度与默认值按 Postgres 适配。
- 数据访问层：由 GORM（Go）改为 Edge Functions 直接调用 Supabase PostgREST/`@supabase/postgrest-js` 或 `pg` 客户端。

## Git/Actions 集成策略
- 文件更新：使用 GitHub/Gitee Contents API 在指定仓库与分支下更新 `images.txt`（已有“API模式”理念）。
- 触发工作流：提交会自动触发 Actions；Edge Functions 记录提交 `commit_sha`。
- 状态监控：
  - 改造为“定时轮询”而非单次长轮询：使用 Supabase Scheduled Functions（Cron）每 30s-60s 扫描状态为 `running` 的任务，查询 GitHub Run 状态并回写 Postgres。
  - 这样规避 Vercel/Supabase 单次函数的超时限制，同时保证可靠性。

## ACR 镜像验证（无 Docker）
- 使用 OCI Registry API：
  - 依据 `registry/namespace/image:tag` 拼出 Manifest URL，带上 ACR 认证（Basic/Bearer，取决于 ACR 接口），获取 200 即存在。
- JS/TS 实现：可选用轻量库或直接 `fetch` 调用，记录 `acr_image` 与验证结果。

## 前端适配
- `web/src/api/index.js` 的 `baseURL:'/api/v1'` 保持不变，Vercel 上新增 Rewrites：`/api/(.*)` → `https://<supabase-edge-url>/(.*)`，做到零/微改动前端。
- 或引入 `VITE_API_BASE`，生产指向 Supabase，开发保留本地代理（可选）。

## 安全与配置管理
- GitHub/Gitee Token、ACR 凭据存放：Supabase 项目 `Secrets`；Edge Functions 读取。
- 数据库敏感值：沿用应用层 AES 加密（参考你现有 Go 逻辑），在 Edge Functions 中用 Node/Deno Crypto 实现 `ENC:` 前缀的加密存储。
- 访问控制与速率限制：前端维持原有限流 UI 逻辑；Edge Functions 添加基础限流/校验。

## 风险与取舍
- 技术栈迁移：Go → TS/Deno，所有后端逻辑需重写；工作量较大。
- 数据库差异：MySQL → Postgres，枚举、时间精度、索引语法需适配。
- 函数超时：单次函数不宜长轮询，必须改造为“定时轮询 + 状态机”。
- 依赖 GitHub API：需妥善处理速率限制与失败重试（已有接口与监控设计）。

## 实施步骤
1. 在 Supabase 创建项目与 Postgres，建表迁移（3张表、索引、约束）。
2. 实现 Edge Functions：
   - 同步提交、状态查询、历史、配置、镜像管理、GitHub 接口。
   - 封装 Contents API 写 `images.txt` 与记录 `commit_sha`。
3. 配置 Scheduled Functions：轮询 GitHub Run 状态并更新任务/镜像记录。
4. 实现 ACR 镜像存在性检测（OCI Registry）。
5. Vercel 部署前端并配置 Rewrites 到 Supabase Edge Functions。
6. 接口与前端联调：提交流程、状态页、配置页、镜像列表与统计。

## 测试与验证
- 接口测试：健康检查、提交同步、状态轮询、历史分页、配置读写、镜像检测。
- 前端验证：按钮操作与状态展示；批量/单条任务流与错误提示。
- 数据库验证：Postgres 表数据、默认配置注入、加密字段格式（`ENC:`）。
- 集成测试：提交后 GitHub Actions 是否触发、监控是否正确归档成功/失败、ACR 验证是否生效。

如确认该方案，我将按照上述步骤开始迁移与实现，并在每个阶段提供接口测试与前端验证的可见结果。