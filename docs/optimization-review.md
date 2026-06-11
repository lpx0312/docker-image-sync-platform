# 系统优化审查报告

> 文档版本：2.1  
> 审查日期：2026-06-10（2026-06-11 逐条与代码核对修订）  
> 状态：规划文档（具体代码修改待后续实施）

本文档基于对当前代码库的全面梳理，从**代码复用率**、**数据结构/注释**、**单元测试**、**E2E 测试**、**安全**、**性能**、**前端/部署**、**数据库 Schema**、**可观测性**等维度给出优化建议与实施路线图。本文档仅作规划参考，不包含已实施的代码变更。

---

## 总体判断

| 维度 | 现状 | 风险等级 |
|------|------|----------|
| 代码复用 | 有工厂/接口等抽象，但 Handler、Git、前端页面层重复明显 | 中 |
| 数据结构/注释 | 同步域文档很好，Auth/ACR/RBAC 与 Swagger 滞后 | 中 |
| 单元测试 | 几乎空白（`go test` 整体约 **0.4%** 覆盖率） | 高 |
| E2E 测试 | 仅有手工指南，无自动化套件、无 CI | 高 |
| 安全 | JWT 不校验实时状态、密码进日志、CORS/限流配置未接入 | **高** |
| 性能 | 多处 N+1 查询、config.yaml 部分配置写了但未生效 | 中 |
| 前端/部署 | 死代码、Bundle 膨胀、Docker HEALTHCHECK 可能失败 | 中 |
| 工程化 | config 与 Go 结构体漂移、Makefile 若干陷阱 | 中 |

---

## 一、代码复用率

### 1.1 已做得好的部分

| 区域 | 文件 | 说明 |
|------|------|------|
| Git 抽象 | `internal/services/git_interfaces.go` | `GitServiceInterface` 统一契约 |
| Git 工厂 | `internal/services/git_factory.go` | 按 `gitee/github` 动态选实现，带缓存 |
| 配置服务 | `internal/services/config.go` | `SetGitConfig(platform, ...)` / `GetGitConfig(platform)` 已参数化 |
| 配置读取 | `internal/utils/git_config.go` | `GetConfigValueWithDecrypt` 被 git 服务复用 |
| URL 解析 | `internal/utils/git_url.go` | GitHub/Gitee URL 解析集中 |
| 错误基础设施 | `internal/middleware/error.go` | `APIError`、`HandleError`、`ValidationError` 等已定义 |
| 前端 API | `web/src/api/index.js` | 单一 axios 实例 + 分组 API |
| 前端工具 | `web/src/utils/status.js`、`repositoryResult.js`、`messageBox.js` | 状态映射、批量结果格式化已抽取 |
| 状态管理 | `web/src/stores/image.js` | 镜像 CRUD/检测逻辑已下沉到 store |

### 1.2 主要重复点（按收益排序）

#### P0-1：Handler 响应格式三套并存

当前存在三种以上响应格式：

- **格式 A** — Auth/Role/Sync/Image 错误：`{"error": "..."}`
- **格式 B** — Config/ACR：`{"status": "error/success", "message": "...", "data": ...}`
- **格式 C** — Config Git 优化接口（约 L1378+）：`{"success": true/false, "data": ..., "message": ...}`
- **格式 D** — 限流 429 与 panic 恢复：`{"code","message"}`（`middleware/ratelimit.go`、`middleware/error.go` 的 `ErrorResponse`）

此外 `role.go` 成功响应用裸 `{"data": ...}`，`sync.go` 用自定义字段（`task_id`、`total/page/data`），`image.go` 用 `{"message": "..."}`，实际变体更多。

`middleware/error.go` 里已有完整错误体系，但 **handlers 中零引用** `HandleError` / `ValidationError`，每个 Handler 手写 `c.JSON + return`。panic 能被 `ErrorHandler` 捕获，但业务错误未统一。

前端 interceptor 主要读 `error.response?.data?.error`（`web/src/api/index.js`），对 `status/message` 格式需各页面自行处理。

**建议**：

- 新建 `internal/handlers/response.go`，提供 `OK(c, data)`、`Fail(c, code, msg)`，全量迁移
- 或让 Handler 统一调用 `middleware.HandleError`

#### P0-2：ID 解析与校验重复

`strconv.ParseUint(c.Param("id"), ...)` 相同模式实测共 **20 处**，分布于：

- `internal/handlers/acr_registry.go`（4 处）
- `internal/handlers/acr_repository.go`（2 处）
- `internal/handlers/acr_tag.go`（3 处）
- `internal/handlers/auth.go`（4 处）
- `internal/handlers/role.go`（3 处）
- `internal/handlers/image.go`（4 处）

**建议**：提取 `handlers/param.go`：

```go
func ParseUintParam(c *gin.Context, name string) (uint, error)
func MustParseID(c *gin.Context) (uint, bool) // 失败时已写响应
```

#### P0-3：Git 服务三份实现并行

| 重复内容 | 位置 |
|----------|------|
| `getCurrentGitConfig` ~75 行 | `git.go` 与 `git_optimized.go` |
| GitHub/Gitee 文件读写 | `git_api.go` 中两套 struct 约 95% 相似 |
| 镜像文件读写 | `git.go` / `git_optimized.go` / `git_api.go` |

**建议**：

- 抽出 `internal/services/git_config_loader.go`
- 抽出 `internal/services/git_images_file.go`
- GitHub/Gitee 共用 `contentsClient` 策略（减少约 400 行重复）

#### P0-4：前端页面逻辑镜像

> **v2.1 核对修正**：重试/删除/检测/批量检测的重复逻辑位于 **`SyncView.vue`（490–578 行）与死代码 `ImagesView.vue`（549–631 行）之间**，二者均调用 `imageStore.retrySync` / `deleteImage` / `checkImageExists` / `batchCheckImages`。`ImagesManageView.vue`（`/images`）实际是 **ACR 仓库管理页**（`acrRepositoryAPI` 删除/导入/清理无效镜像），与 SyncView 业务不同，v2.0 中「两页逻辑镜像」的对象描述有误。

- `web/src/views/ImagesView.vue`（962 行）**未注册路由**，属于死代码，且内部引用了 `api/index.js` 中**不存在的 `configAPI`**（398、431 行），若误挂路由会直接报错——应直接删除。
- `SyncView.vue` 内联了状态映射（473–480 行）；使用 `@/utils/status` 的恰是死代码 `ImagesView.vue`，`ImagesManageView.vue` 并未使用。
- 后端模型已定义 `retrying`、`skipped` 同步状态（`models.go`），但 `status.js` 与 `SyncView` 内联映射**均未覆盖**，UI 会显示「未知」。

**建议**：

- 删除 `ImagesView.vue` 死代码（其与 SyncView 的重复随之消除）
- `SyncView` 改用 `@/utils/status`，并补全 `retrying`、`skipped` 状态映射
- 若后续仍有跨页共享需求，再提取 `composables/useImageActions.js`（当前不存在 `composables/` 目录）

#### 其它可收敛点

| 问题 | 位置 | 建议 |
|------|------|------|
| Gitee/GitHub 配置更新重复 | `handlers/config.go` | 合并为 `updateGitConfig(c, platform string)` |
| 临时 Service 实例化 | `sync.go`、`acr_registry.go`、`acr_repository.go` | Handler 构造时注入 `*AcrAffinityService` |
| 超大 Handler 文件 | `sync.go`（2231 行）、`config.go`（1766 行）、`image.go`（1005 行） | 按领域拆分 |
| ACR Handler 同构 CRUD | `acr_registry.go`、`acr_repository.go`、`acr_tag.go` | 共享 bind/response helper |
| GitConfigForm 保存逻辑 | `GitConfigForm.vue` | `saveGitPlatformConfig(platform, ...)` 参数化 |
| Dialog 组件模式 | `AddRepositoryDialog.vue`、`BatchAddRepositoryDialog.vue` | `useDialogForm` composable 或 `BaseFormDialog.vue` |
| API 错误双弹窗 | `api/index.js` + 各页面 catch | interceptor 统一处理，页面 catch 只做 refresh |
| ACR 结果 struct 重叠 | `services/acr_repository.go` | 基础 `RepositoryOperationResult` + embed |
| AcrRegistry Request/Update 重复 | `models/acr_registry.go` | 单一 struct + 创建/更新 validator |
| Aliyun 与 AcrRegistry 双轨 | `utils/acr.go`、`sync.go` | 统一走 `AcrRegistryService.GetDefault()` |
| 解密辅助函数分散 | `encryption.go`、`utils/secure_config.go` | 完整实现在 `services/encryption.go`，`utils/secure_config.go` 仅是 `DecryptSystemConfigValue` 辅助（非完整双份），仍建议归口 |
| 未使用的 struct（已确认） | `models.ImageRequest`（`models.go` 306–309 行） | 全仓库无引用，可直接删除 |
| Git 工厂双实例 | `git_factory.go` | 启动建 gitee 实例、github 懒创建，内存可并存两个 `GitService`，简化为单实例 |
| GitHub 路由 inline 在 main.go | `main.go`（283–328 行） | 迁入 `handlers/github.go`，统一错误处理 |
| 后端/前端结果消息重复格式化 | `acr_repository.go` + `repositoryResult.js` | 只保留一处生成 message |

---

## 二、数据结构与注释

### 2.1 文档质量分层

#### 做得好

`internal/models/models.go` 中同步相关实体/DTO 注释完整，例如 `ImageSyncRecord` 含状态流转说明、`BatchSyncRequest` 含验证规则说明。

`internal/middleware/error.go`、`internal/services/github.go`（WorkflowRun 等）、`internal/services/acr_affinity.go` 也有较好文档。

`internal/database/database.go` 中 `AutoMigrate()` 有功能、表清单、注意事项说明。

#### 明显落后

| 区域 | 问题 |
|------|------|
| `User`、`LoginLog` | 仅一行类型名，无字段/业务说明 |
| `Permission`、`Role`（`rbac.go`） | 各一行，无字段说明 |
| ACR 实体与 Request | `acr_registry.go`、`acr_repository.go` 几乎无字段注释 |
| Handler 私有 DTO | `auth.go`、`role.go` 中 `loginRequest` 等无 struct 文档 |
| Service 结果 struct | `SyncFromRecordsResult` 等只有类型名，字段无说明 |
| 数据库迁移辅助函数 | `initRolesAndPermissions` 等无 godoc |
| ACR 迁移 | `migrations.go` 注释偏薄，与 `AutoMigrate()` 入口分裂 |

### 2.2 JSON / Swagger / 前端文档漂移

**JSON/gorm 标签**：整体 snake_case 一致，敏感字段用 `json:"-"`，分层清晰。

**主要漂移点**：

| 问题 | 说明 |
|------|------|
| Swagger 手写维护 | 无 swag 注解；`docs/swagger.json` 与 Go struct 手工对应 |
| Auth 接口缺失 | `/auth/login`、`/auth/me`、`/auth/users` 等无 definitions |
| ACR 全家桶缺失 | `/acr/registries`、`/acr/repositories`、`/acr/tags` 未收录 |
| 字段滞后 | Go 已有 `acr_registry_id`，Swagger 未收录 |
| 单位不一致 | Go `Duration` 注释为「秒」，Swagger 写「毫秒」 |
| 状态枚举滞后 | Go `SyncTask.Status` 含 `partial_success`，Swagger enum 未含 |
| 命名不一致 | Go `BatchTaskStatusResponse` vs Swagger `BatchStatusResponse` |
| 前端 JSDoc 过时 | `api/index.js` 中 `submitSync` 仍写 `original_image`/`acr_image`，实际为 `SyncRequest` |
| 无 TypeScript | 全项目 0 个 `.ts` 文件，无 `@typedef` 与后端 struct 对应 |

### 2.3 注释优化建议

1. 补齐 `User`、`AcrRegistry`、`AcrRepository` 实体文档（对齐 `ImageSyncRecord` 风格）
2. Auth/ACR 响应改为命名 struct，替代 `gin.H` / `map[string]interface{}`
3. 引入 swag 从代码生成 Swagger，或建立「改 struct 必改 swagger.json」检查
4. 修正 `web/src/api/index.js` 中与后端 Request 不一致的 JSDoc
5. 统一 `TableName()` 显式声明（ACR 实体目前依赖 GORM 默认）

---

## 三、单元测试

### 3.1 现状（2026-06-10 实测）

```text
go test ./... -cover 结果：

internal/services  coverage: 0.4%
其余包              coverage: 0.0%
前端                0 个测试文件，package.json 无 test 脚本
CI                  无 .github/workflows
```

**测试文件统计**：

| 目录 | 源文件 | 测试文件 | 测试占比 |
|------|--------|----------|----------|
| `internal/` | 42 个 `.go`（非测试） | 2 | ~4.8% |
| `web/src/` | 32 个 | 0 | 0% |

**现有测试文件**：

| 文件 | 评价 |
|------|------|
| `internal/services/acr_repository_test.go` | **较好**：表驱动测试 `ExtractRepoName`，覆盖边界场景 |
| `internal/services/github_test.go` | **较差**：只测本地 map/JSON 序列化，**未调用** `github.go` 生产代码，易制造「有测试」假象 |

**Makefile 现状**：

- `make test` / `make test-backend`：跑 `go test -v -race -coverprofile=coverage.out ./...`
- 无 `test-frontend`、`e2e`、`integration` 目标
- `make fmt` 调用不存在的 `npm run lint:fix`（`web/package.json` 无此脚本）

### 3.2 核心业务零覆盖

| 区域 | 关键模块 |
|------|----------|
| 同步核心 | `handlers/sync.go`、`stores/sync.js`、单/批量同步表单 |
| 认证与 RBAC | `services/auth.go`、`user.go`、`role.go`、`middleware/auth.go` |
| 镜像管理 | `handlers/image.go`、`handlers/acr_*.go`、ACR API 服务 |
| Git 集成 | `git.go`、`git_optimized.go`、`git_api.go`、`git_factory.go` |
| GitHub Actions | `services/github.go`（测试文件未覆盖） |
| 配置与加密 | `services/config.go`、`encryption.go`、`utils/secure_config.go` |
| 数据库 | `database/`、`models/` |
| 中间件 | CORS、限流、日志、错误处理 |
| 前端全部 | 9 个 View、10+ 组件、3 个 Pinia store |

### 3.3 「测试辅助」代码（非自动化测试）

以下能力支持手工/运行时验证，不能替代回归测试：

| 能力 | 位置 |
|------|------|
| Mock 批量同步 | `POST /api/v1/sync/batch/mock` |
| Git 连接/网络/代码操作测试 | `POST /config/git/test`、`GET /config/git-network-test`、`POST /config/git-test-operations` |
| ACR 连接测试 | `POST /config/aliyun/test` |
| Git 测试专用方法 | `PullImagesFileForTesting()`、`UpdateImagesFileForTesting()` |

> **安全提示**：`POST /sync/batch/mock` 在生产路由中仅需 `PermSync` 即可访问，应通过环境变量或 release 模式门控。

### 3.4 建议测试分层

#### P0 — 纯函数单元测试（投入小、收益高）

```
utils/acr.go          → 镜像名解析、ACR 地址构建
utils/git_url.go      → URL 解析边界
utils/registry.go     → registry 规范化
services/auth.go      → JWT 签发/校验、密码哈希
services/encryption.go → 加解密 round-trip
```

#### P1 — Handler 层 httptest

```
handlers/auth.go      → 登录成功/失败、401
handlers/sync.go      → 参数校验、限流 mock
middleware/auth.go      → PermissionRequired 行为
```

#### P2 — 集成测试

```
database/             → AutoMigrate + RBAC seed（testcontainers 或 sqlite）
services/acr_repository.go → 批量创建逻辑
```

#### P3 — 前端 Vitest

```
stores/auth.js        → token 持久化、权限判断
utils/status.js       → 状态映射
components/           → 表单校验（SingleSyncForm 等）
package.json          → 添加 test 脚本与 vitest 配置
```

#### CI 最小门禁（建议）

```yaml
# .github/workflows/test.yml
go test -race ./...
npm run lint && npm run build
# 可选：coverage 阈值从 5% 逐步提到 30%
```

#### 需处理的「假测试」

- 改写 `github_test.go` 为测试 `github.go` 中实际导出函数，或删除以免误导

---

## 四、E2E 测试

### 4.1 现状

| 类型 | 状态 |
|------|------|
| 手工 E2E 指南 | `docs/e2e-test-guide.md`（T0–T10，覆盖登录、同步、配置、用户管理等） |
| 浏览器自动化 Skill | `.cursor/skills/chrome-devtools-cli/SKILL.md`（Agent 驱动，不可 CI 化） |
| 冒烟脚本 | `scripts/health-check.sh`、`scripts/monitor.sh` |
| Playwright/Cypress | **无** |
| `ChromeDevTools-Files/` | **不存在**（CLAUDE.md 提及但未创建） |
| CI 流水线 | **无** `.github/workflows/` |

E2E 为**文档驱动 + 人工/Agent 执行**，非可重复 CI 流水线。

**脚本缺口**：

- `scripts/health-check.sh` 未带 JWT，生产环境对需认证接口会 401
- `scripts/monitor.sh` 文档提及 `--config` 但未实现；告警仅为 `log "ALERT"`
- 无 `restore.sh`、`migrate.sh`；容器名在 health-check 与 monitor 间不一致

### 4.2 建议 E2E 策略

#### 短期（保留现有指南，增强可重复性）

- 将 `e2e-test-guide.md` 中的 curl 步骤抽成 `scripts/e2e-smoke.sh`（登录 → health → 列表 → mock 同步）
- 扩展 `scripts/health-check.sh`：支持 `login → token → 探测` 或专用 internal health

#### 中期（可 CI 的 E2E）

```
Playwright（推荐，与 Vue 生态契合）
  ├── auth.spec.js      登录 / 权限跳转
  ├── sync.spec.js      mock 批量同步 + 历史列表
  └── config.spec.js    Git 连接测试（mock 后端或 test 环境）

docker-compose.test.yml
  ├── mysql
  ├── backend (test config)
  └── playwright runner
```

#### 长期

- PR 门禁：smoke E2E + 单元测试
- 关键路径（登录 → 提交同步 → 查状态）每次发布必跑

---

## 五、安全

### 5.1 P0 — 应立即处理

| # | 发现 | 文件 |
|---|------|------|
| S1 | **ACR 明文密码写入应用日志**：`TriggerWorkflow` 用 `zap.Any("inputs", inputs)` 记录 workflow 输入，含 `aliyun_registry_password` | `internal/services/github.go`（约 628–631 行） |
| S2 | **JWT 不校验用户实时状态**：`AuthRequired` 只验签 JWT，不查库；账号被 disabled 或角色被改后，旧 Token 在过期前仍有效 | `internal/middleware/auth.go`（12–41 行） |
| S3 | **JWT 角色变更不生效**：权限中间件用 JWT 中的 `roleID` 查权限；管理员修改用户 `role_id` 后，旧 Token 仍携带旧角色 | `internal/services/auth.go`（56–66 行）；`internal/middleware/auth.go`（54–61 行） |
| S4 | **JWT 默认密钥 fallback**：配置为空时硬编码 `docker-sync-platform-jwt-secret-change-me` | `internal/services/auth.go`（32–35、78–82 行）；`config.yaml`（94 行） |
| S5 | **调试接口泄露解密配置**：非 release 模式下 `GET /api/v1/config/debug/:key` 返回完整解密值；默认 `mode: debug` | `main.go`（352–354 行）；`handlers/config.go`（381–403 行） |
| S6 | **config.yaml 含真实凭据且已被 git 跟踪**：GitHub PAT、Gitee Token、阿里云密码、JWT secret 等明文。虽在 `.gitignore`（第 50 行），但 `git ls-files config.yaml` 显示**仍被跟踪并已入库**，git 历史中含全部凭据，必须 `git rm --cached` 并轮换全部凭据 | `config.yaml` |
| S7 | **ACR 凭据经 GitHub Actions inputs 传递**：解密密码后放入 `workflow_dispatch` inputs，会出现在 GitHub Actions UI/日志中 | `handlers/sync.go`（2179–2215 行）；`github.go`（625–637 行） |

### 5.2 P1 — 高优先级

| # | 发现 | 说明 |
|---|------|------|
| S8 | CORS 回显任意 Origin | `CORS()` 有 Origin 就回显并设 `Allow-Credentials: true`；`CORSWithConfig` 和 `security.cors` 已实现但未使用 |
| S9 | 限流配置未接入 | `main.go` 硬编码 `RateLimit(100, 200)`，忽略 `config.yaml` 的 `security.rate_limit` |
| S10 | 登录无专项 brute-force 防护 | 仅依赖全局限流 |
| S11 | Sync 限流按路由独立 | submit/batch/mock 各 5 次/分钟，合计可达 ~15 次/分钟 |
| S12 | Swagger/API 文档公开 | `/api/v1/docs`、`swagger.json` 无需认证 |
| S13 | Mock 同步在生产路由可用 | `POST /sync/batch/mock` 仅需 `PermSync` |
| S14 | RBAC 信息泄露 | `GET /auth/roles/options` 仅需登录，返回所有角色及 permissions |
| S15 | Logout 不吊销 Token | 无 JWT 黑名单，登出后 Token 仍有效至过期 |
| S16 | 内部错误直接返回客户端 | 多处 Handler 将 `err.Error()` 写入 JSON |
| S17 | 阿里云测试用 fmt.Printf 打 stdout | `handlers/config.go`（1073–1336 行） |
| S18 | 生产 GORM SQL Info 级日志 | 可能记录含敏感值的 SQL（`database/database.go` 84–87 行） |
| S19 | 缺少安全响应头 | 无 HSTS、CSP、`X-Frame-Options` 等 |

### 5.3 P2 — 其它安全细节

| # | 发现 | 说明 |
|---|------|------|
| S20 | 默认 admin 密码过弱 | `admin123`（`config.yaml`、`database.go` seed） |
| S21 | `ENCRYPTION_KEY` 未走 viper | 仅 `os.Getenv`；dev 模式用可预测默认密钥 `docker-sync-platform-default-key-2024`。**v2.1 核对**：`GIN_MODE=release` 时未设置会直接报错（已有改进），但默认 debug 模式仍静默用默认密钥 |
| S22 | 登录日志区分「用户不存在」与「密码错误」 | 对客户端 API 统一返回「用户名或密码错误」（无直接枚举）；但 `GET /auth/login-logs`（需 `PermUsers`）可见失败原因，存在面向管理员的枚举信息 |
| S23 | SQL 注入 | GORM 参数化查询为主，**无明显 SQL 注入** |
| S24 | `POST /config/git-test-operations` 可传任意 token 做 Git 操作测试 | 需 `PermConfig`，但请求体可注入外部 token，存在滥用面（v2.1 新增） |
| S25 | workflow 错误响应体进入错误链 | `github.go` L655 `fmt.Errorf(..., string(resp.Body()))`，GitHub 响应体可能随 `err.Error()` 返回客户端（v2.1 新增） |
| S26 | `docs/e2e-test-guide.md` 含明文测试账号 | 第 5 行 `zwh / Abc2020##`（v2.1 新增） |

### 5.4 安全修复建议

1. **立刻**：去掉 workflow inputs 日志中的密码（S1）；轮换已暴露 token/密码（S6）
2. **短期**：JWT 中间件增加用户 status/role_id 实时校验（S2–S3）；接入 `security.cors`/`security.rate_limit`（S8–S9）；登录专项限流（S10）
3. **架构**：ACR 凭据不应经 GitHub Actions plaintext inputs 传递，改用 GitHub Secrets/OIDC（S7）

---

## 六、性能

### 6.1 数据库与查询

| # | 发现 | 文件 |
|---|------|------|
| P1 | **连接池 yaml 未生效**：`config.yaml` 有 `max_idle_conns` 等，但 `DatabaseConfig` 无对应字段，`InitDatabase` 硬编码 10/100/1h | `config.yaml`；`config/config.go`；`database/database.go`（105–107 行） |
| P2 | **N+1：批量镜像检测** — 循环内对每个 image 单独查 AcrRegistry | `handlers/image.go`（894–906 行） |
| P3 | **N+1：单镜像检测** | `handlers/image.go`（725–731 行） |
| P4 | **N+1：同步流程** — `buildACRImageForRecord` 每条 record 查一次 ACR | `handlers/sync.go`（2218–2228 行） |
| P5 | **N+1：用户列表** — `ListUsers` 对每个 user 调 `GetRolePermissions` | `handlers/auth.go`；`services/user.go` |
| P6 | **N+1：角色列表** — 每角色查 permissions + user_count | `services/role.go` |
| P7 | **GetImageStats 5 次独立 COUNT** | `handlers/image.go`（588–608 行），可合并为 `GROUP BY sync_status` |
| P8 | **缺少复合索引** | `acr_repositories(acr_registry_id, repository_name)`；`image_sync_records(task_id, sync_status)`；`(sync_status, created_at)` |

### 6.2 运行时与缓存

| # | 发现 | 说明 |
|---|------|------|
| P9 | GitHub 轮询间隔硬编码 30s | 未读 `github_actions.status_check_interval_seconds`（config 默认 60s） |
| P10 | 每个 `RateLimit()` 启动独立 cleanup goroutine | IP 映射在 steady traffic 下可能增长 |
| P11 | ACR tokenCache 无淘汰 | 按 registry 键缓存，无 size/TTL |
| P12 | GetTagsWithDetails 拉全量 tag 再并发取详情 | 大仓库开销大（`acr_api.go`） |
| P13 | SyncHandler 后台 goroutine 有 wg+ctx 优雅关闭（较好） | mock 批量同步可 spawn 大量 goroutine |
| P14 | Git 文件缓存基本正确 | 更新 gitee/github 凭据时会清 file/github cache |

---

## 七、配置与架构漂移

### 7.1 config.yaml 写了但 Go 未读取

| 配置块 | 说明 |
|--------|------|
| `git.operation_mode`、`git.api.*`、`git.sparse.*`、`git.full.*` | Git 优化参数未映射到 Go struct |
| `security.cors`、`security.rate_limit` | 中间件硬编码，未读配置 |
| DB 连接池参数 | 见性能 P1 |
| `auth.auto_logout_minutes` | 可能仅前端使用，后端未实现 |
| `server.host` | 未映射 |

### 7.2 环境变量与 viper

- 已绑定：`JWT_SECRET`、`DB_*`、`GITEE_*`、`GITHUB_*`、`ALIYUN_*`
- 未绑定：`TOKEN_EXPIRY`、`ENCRYPTION_KEY`、`security.*`、`sync.*`、`github_actions.*`
- `viper.AutomaticEnv()` 无 prefix，任意环境变量可能意外覆盖嵌套配置
- `ENCRYPTION_KEY` 仅 `os.Getenv`，dev 模式用可预测默认密钥

### 7.3 main.go 路由与中间件

当前中间件顺序：

```text
RequestLogger → ErrorHandler(panic) → CORS → RateLimit(100,200)
→ Static(/, /static) → /api/v1 路由组
```

| # | 发现 | 说明 |
|---|------|------|
| R1 | API 文档公开挂载 | `/api/v1/docs`、`swagger.json` |
| R2 | protected 组内重复创建 service/handler | 略影响启动可读性 |
| R3 | GitHub 路由 inline 在 main.go | 错误处理直接 `err.Error()` |
| R4 | debug 路由默认开启 | `config.yaml` 默认 `mode: debug` |
| R5 | 优雅关闭顺序正确 | HTTP Shutdown → `syncHandler.Shutdown()` |
| R6 | 定时任务未实现 | `main.go` L142：`// TODO: 定时任务初始化` |

`SkipPaths` 已在 `middleware/logger.go` 实现但未接入，建议跳过 `/api/v1/health`。

---

## 八、前端优化

### 8.1 Bundle 与路由

| 问题 | 路径 | 建议 |
|------|------|------|
| 全量注册 Element Plus 图标（200+） | `web/src/main.js` | 按需 import |
| 全量 CSS `element-plus/dist/index.css` | 同上 | `unplugin-vue-components` 按需样式 |
| 无 vendor 分包 | `web/vite.config.js` | `manualChunks`（vue/element-plus/axios） |
| 无 bundle 分析 | 同上 | `rollup-plugin-visualizer` |
| 路由懒加载不一致 | `web/src/router/index.js` | `LoginView`、`SyncView`、`GitHubView`、`ConfigView` 应改为 `() => import(...)` |

### 8.2 Pinia / Axios

| 问题 | 路径 | 建议 |
|------|------|------|
| 路由守卫绕过 store，直接读 localStorage | `router/index.js` | 统一用 `useAuthStore().permissions` |
| Store 与 router 双轨权限源 | `stores/auth.js` vs `router/index.js` | 权限只存 Pinia |
| 模块加载时启动定时器 | `stores/auth.js` | 移到 `login()` / `App.vue onMounted` |
| 401 用 window.location.href 硬跳转 | `api/index.js` | 改用 `router.push` + `authStore.clearAuth()` |
| 全局 ElMessage.error 无 opt-out | `api/index.js` | 支持 `config.skipGlobalErrorHandler` |
| 生产环境 debug 日志 | `api/index.js` testGitOperations | 包在 `import.meta.env.DEV` |
| 无 i18n | 全项目 | 文案硬编码中文；若有多语言需求引入 vue-i18n |

### 8.3 超大组件与死代码

| 文件 | 行数 | 动作 |
|------|------|------|
| `ImagesView.vue` | 961 | **未注册路由，死代码** — 删除或合并 |
| `SyncView.vue` | 846 | 拆 `SyncHistoryTable` / `SyncStatsCards` / `SyncPolling` |
| `GitConfigForm.vue` | 676 | 拆 Gitee/GitHub 面板 |
| `BatchSyncForm.vue` | 670 | 拆手动输入/文件导入/ACR 选择 |
| `ImagesManageView.vue` | 584 | 拆 ACR 切换、仓库表、批量操作 |
| `AcrTagListPanel.vue` | 501 | 拆列表与详情加载 |

### 8.4 无障碍（a11y）

- 全项目 **0** 处 `aria-*` / `role=`
- 汉堡菜单无 `aria-label`（`App.vue`）
- Logo 用 `@click` 的 `<div>`，键盘不可达
- 建议：导航加 `aria-current="page"`；表单错误关联 `aria-describedby`

---

## 九、部署与 DevOps

### 9.1 P0 — 密钥与健康检查

| 问题 | 路径 |
|------|------|
| Compose 默认值含真实 token/密码 | `deploy/docker-signal/docker-compose.yml`；`docker-compose-all.yml` |
| `env-example` 含可识别凭据 | `deploy/docker-signal/env-example` |
| Makefile 明文 ACR 密码 | `deploy/docker-signal/Makefile` |
| 后端容器以 **root** 运行 | `Dockerfile-backend`（无 `USER`） |
| **HEALTHCHECK 可能恒失败** | 三个 Dockerfile 用 `wget`，运行镜像**未安装 wget** |

```dockerfile
# deploy/docker-signal/Dockerfile-backend
RUN apk add --no-cache ca-certificates tzdata git   # 无 wget
HEALTHCHECK CMD wget ... http://localhost:8080/api/v1/health   # 会失败
```

**动作**：`apk add wget` 或改用已安装工具；默认值改为占位符；凭据仅来自 `.env`/secret。

### 9.2 P1 — 构建与 Compose

| 问题 | 说明 |
|------|------|
| Go 版本漂移 | 镜像 `1.26.1`，`go.mod` 写 `1.21` |
| builder Alpine 3.23 vs runtime 3.18 | 统一基础镜像 |
| `COPY . .` 破坏 layer 缓存 | 先 COPY 源码目录，最后再 COPY 其余 |
| docker-all 健康检查只测 nginx `/` | 应增加 `/api/v1/health` |
| nginx API 代理 timeout 30s | 同步/批量接口建议 120s+ |
| frontend depends_on 无 healthy 条件 | 改为 `condition: service_healthy` |
| compose 默认 DB_HOST=192.168.0.180 | 无 MySQL 版易连错 |
| MySQL healthcheck 密码 fallback 不一致 | `root123456` vs `123456` |
| `version: '3.8'` 已废弃 | 全部 compose 文件 |

### 9.3 根目录 Makefile 陷阱

| 问题 | 说明 |
|------|------|
| `.PHONY` 声明 `docker-build` 等但未定义 | 误导使用者 |
| `make dev` 后台 go run，Ctrl+C 可能遗留 Go 进程 | 进程管理不完善 |
| `make deps` 写全局 `go env -w GOPROXY` | 影响全局环境 |
| `make fmt` 调用不存在的 `npm run lint:fix` | 命令失败 |
| `make clean` 执行 `docker-compose down` | 根目录无 compose 文件 |
| `make init` 大段注释掉 | 体验不完整 |

### 9.4 dev.sh 与备份脚本

- `dev.sh` 宣称 air 热重载但实际用 `go run`
- `backup.sh` 密码与 `dev.sh` 不一致，应从 config/.env 读取

---

## 十、数据库 Schema

| 问题 | 路径 | 建议 |
|------|------|------|
| `acr_repositories` 无唯一约束 | `models/acr_repository.go` | 加 `(acr_registry_id, repository_name)` uniqueIndex |
| `is_default` 无 DB 级唯一保证 | `models/acr_registry.go` | 部分索引或应用层事务 + CHECK |
| ACR 迁移与主迁移分裂 | `database.go` vs `migrations.go` | 合并进 `AutoMigrate()`；失败应阻断启动 |
| `RunMigrations` 错误只 log 不 return | `migrations.go` | 启动 fast-fail |
| 需手工 SQL 回填 | `scripts/backfill_acr_registry_id.sql` | 新环境加 NOT NULL + 迁移 hook |
| `sync_tasks.acr_registry_id` 用 `0` 作 sentinel | `models/models.go` | 改为 `*uint` nullable FK |
| 缺复合索引（高频列表查询） | `ImageSyncRecord` | 如 `(sync_status, created_at)`、`(acr_registry_id, sync_status)` |
| `login_logs` 无留存策略 | `models/models.go` | 定时清理或分区 |
| `AutoMigrate` 注释未列 ACR/RBAC 全表 | `database/database.go` | 更新文档与迁移清单一致 |

---

## 十一、日志与可观测性

| 问题 | 路径 | 建议 |
|------|------|------|
| **三套日志**：Zap + logrus + `log` | `main.go`；`config.go`；`database.go` | 统一 Zap adapter |
| GORM 生产仍 Info 级 SQL | `database/database.go` | 按 LOG_LEVEL 设为 Warn/Error |
| `/health` 不检测 DB | `main.go` | 返回 `{db:"ok"}` 或拆 live/ready |
| GitHub API 完整 response body 打 Info | `github.go` | 降级为 Debug 并截断 |
| 无 Prometheus `/metrics` | 全项目 | 加 request latency、sync 任务 gauge |
| 前端无错误上报 | `api/index.js` | 可选 Sentry |
| sync/config Handler 混用 logger 与 fmt.Printf | `handlers/config.go` | 统一结构化日志 |

---

## 十二、依赖版本

### Go（`go.mod`）

| 包 | 当前 | 建议 |
|----|------|------|
| Go | 1.21 | 对齐 Dockerfile，升级到 1.22+ |
| `github.com/gin-gonic/gin` | v1.9.1 | 升级到 v1.10.x |
| `gorm.io/gorm` | v1.25.5 | 升级到 v1.25.12+ / v1.26.x |
| `github.com/spf13/viper` | v1.17.0 | 升级到 v1.19+ |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | 较新，可保持 |
| 间接：`github.com/docker/docker` | v24.0.0 | 随 go-containerregistry 升级评估 |

### 前端（`web/package.json`）

| 包 | 当前 | 建议 |
|----|------|------|
| `axios` | ^1.5.0 | 升级到 1.7.x+ |
| `vite` | ^4.4.9 | 考虑 Vite 5.x |
| `vue` | ^3.3.4 | 升级到 3.4.x+ |
| `element-plus` | ^2.3.9 | 升级到最新 2.x patch |

---

## 十三、实施路线图

### 13.1 统一优先级总表

| 优先级 | 事项 | 预期收益 |
|--------|------|----------|
| **立即** | 去掉 workflow inputs 日志密码；轮换已暴露 token | 消除凭据泄露 |
| **立即** | 修复 Docker HEALTHCHECK（安装 wget 或换 probe） | 容器编排可用 |
| **P0** | JWT 中间件增加用户 status/role 实时校验 | 账号/角色变更即时生效 |
| **P0** | 接入 security.cors / security.rate_limit；登录专项限流 | 安全策略与配置一致 |
| **P0** | 统一 Handler 响应格式 + 启用 error 中间件 | 减少前后端重复处理 |
| **P0** | `utils/*` + `auth` 单元测试 + GitHub Actions CI | 防回归、PR 有门禁 |
| **P0** | 前端 useImageActions + 统一 status；删除 ImagesView 死代码 | 减少重复、降低维护成本 |
| **P0** | Element Plus 按需 + 路由全量 lazy load | 首屏体积下降 |
| **P1** | 修复 BatchCheck / ListUsers / sync 的 N+1；补复合索引 | 列表/检测性能提升 |
| **P1** | 连接池读配置；GitHub 轮询读 config | 配置与运行时一致 |
| **P1** | 提取 Git 配置/文件操作共享模块 | 减少 ~500 行重复 |
| **P1** | Auth/Sync httptest；修正 Swagger + JSDoc 漂移 | 文档与行为一致 |
| **P1** | health-check.sh 支持认证；e2e-smoke.sh | 发布前冒烟可靠 |
| **P1** | acr_repositories 唯一索引；迁移合并 | 数据完整性 |
| **P1** | 生产关闭 mock 同步与 debug 接口 | 降低滥用风险 |
| **P2** | Playwright + docker-compose.test | 可 CI 的全链路回归 |
| **P2** | ACR 配置单轨化；ACR 凭据改 GitHub Secrets/OIDC | 消除双配置源与明文传递 |
| **P2** | 拆分 sync.go / config.go 大文件 | 可维护性与可测试性 |

### 13.2 阶段规划

```
立即（安全/可用性，1–3 天）
├── 日志脱敏（github.go inputs）
├── 轮换 config.yaml / compose 中已暴露凭据
├── 修复 Docker HEALTHCHECK
└── JWT 实时校验（或缩短 Token 有效期 + 角色变更强制重登）

P0（1–2 周）
├── 统一 Handler 响应 + error 中间件
├── 接入 CORS/限流配置 + 登录限流
├── useImageActions composable + 删除 ImagesView 死代码
├── Element Plus 按需 + manualChunks + 路由 lazy load
├── utils 单元测试 + CI
└── 提取 getCurrentGitConfig

P1（2–4 周）
├── N+1 修复 + 复合索引 + 连接池配置
├── Git API 抽象 + httptest 核心 Handler
├── Swagger/Go 字段对齐 + health-check 认证
├── acr_repositories 唯一索引 + 迁移合并
└── e2e-smoke.sh

P2（长期）
├── Playwright E2E
├── AliyunConfig → AcrRegistry 单轨化
├── GitHub Secrets/OIDC 替代 plaintext inputs
├── 拆分 sync/config 大文件
└── Prometheus metrics + DB ready health
```

### 13.3 建议起步项（任选其一）

1. **安全快修**：日志脱敏 + Docker HEALTHCHECK + 删除 `ImagesView.vue` 死代码（改动小、收益明确）
2. **工程质量**：`handlers/response.go` 示范迁移 + `utils/acr.go`/`git_url.go` 单元测试 + 最小 CI
3. **前端体验**：Element Plus 按需 + 路由全量 lazy load + `useImageActions.js`

---

## 十四、附录

### 14.1 相关文档

- [端到端测试指南](e2e-test-guide.md)
- [Swagger 使用说明](SWAGGER使用说明.md)
- [CLAUDE.md 开发指引](../CLAUDE.md)

### 14.2 审查范围

- 后端：`internal/`（handlers、services、models、middleware、utils、database）
- 前端：`web/src/`（views、components、stores、api、utils）
- 部署：`deploy/`（Dockerfile、compose、nginx）
- 测试与 CI：`*_test.go`、`Makefile`、`docs/`、`.github/workflows/`
- 运维脚本：`scripts/`

### 14.3 修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.0 | 2026-06-10 | 初版：代码复用、注释、单元测试、E2E 四维审查 |
| 2.0 | 2026-06-10 | 整合第二轮审查：安全、性能、配置漂移、前端/部署、数据库 Schema、可观测性、依赖版本；统一实施路线图 |
