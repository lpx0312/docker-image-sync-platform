# dsync 命令行客户端

`dsync` 是 docker-image-sync-platform 的 CLI 客户端，通过平台 REST API（`/api/v1`）完成登录、镜像仓库（阿里云 ACR / 华为云 SWR）/镜像/Tag 查询、镜像同步提交与进度跟踪，适合日常使用与脚本自动化。

> 镜像仓库以**别名（ALIAS）**标识展示与选择（ACR/SWR 命名空间可能同名）；`--acr` 参数优先按别名匹配，也兼容直接传 namespace（同名冲突时会要求改用别名）。

> 平台支持多仓库实例混管：阿里云 ACR 与华为云 SWR 使用同一套 `--acr <namespace>` 引用方式（SWR 的 namespace 即组织名），`dsync acr list` 的 TYPE 列会标注类型。

## 安装

方式一：下载预编译二进制（GitHub Actions 自动构建，见下方"发布流程"）：

```bash
# Linux ARM64 示例，其他平台同理（linux/darwin × amd64/arm64）
curl -LO https://github.com/lpx0312/docker-image-sync-platform/releases/latest/download/dsync-linux-arm64.tar.gz
tar -xzf dsync-linux-arm64.tar.gz
sudo install -m 0755 dsync-linux-arm64 /usr/local/bin/dsync
```

方式二：本地构建（需要 Go 1.21+）：

```bash
make cli            # 构建到 bin/dsync
sudo make cli-install   # 安装到 /usr/local/bin/dsync
```

## 发布流程（CI 多平台构建）

`.github/workflows/cli-release.yml` 负责交叉编译并发布：

- **手动触发**（Actions → cli-release → Run workflow）：只构建并上传 artifact，用于测试，不创建 Release；
- **推送标签** `cli-v*`（如 `git tag cli-v1.0.0 && git push origin cli-v1.0.0`）：构建 4 个平台（linux/darwin × amd64/arm64）的 `tar.gz` + `SHA256SUMS` 校验文件，并自动创建 GitHub Release。

版本号与构建时间通过 `ldflags` 注入 `dsync --version`。

## 快速开始

```bash
# 1. 登录（交互输入密码，凭据保存在 ~/.config/dsync/config.json，权限 0600）
dsync login --server http://localhost:8080

# 2. 查看仓库列表（引用仓库时优先使用 ALIAS 列的值）
dsync acr list

# 3. 同步一个镜像（默认阻塞等待，完成后打印目标镜像地址）
dsync sync nginx:1.25

# 4. 搜索镜像与 Tag
dsync search nginx
```

## 命令总览

| 命令 | 说明 |
|------|------|
| `dsync login [--server URL] [--username u]` | 登录并保存凭据；token 过期后自动用保存的密码续登 |
| `dsync whoami` / `dsync logout` | 查看当前用户 / 登出并清除本地凭据 |
| `dsync acr list` | 所有镜像仓库 + 仓库配额用量（TYPE 列区分 ACR/SWR，`*` 为默认仓库；SWR 配额显示「不限」） |
| `dsync repo list [--acr ns] [--filter kw]` | 镜像仓库列表（数据源：平台本地库） |
| `dsync tag list <repo> [--acr ns]` | 某仓库全部 Tag（实时查 ACR） |
| `dsync search <kw> [--acr ns] [--refresh]` | 跨 ACR 搜索仓库名与 Tag |
| `dsync sync <源镜像> [flags]` | 提交同步并等待完成 |
| `dsync batch -f images.txt [flags]` | 批量提交同步 |
| `dsync task list` / `dsync task status <id> [--watch]` | 任务历史 / 任务详情 |
| `dsync retry <record-id>` / `dsync retry --task <id>` | 重试失败记录 |
| `dsync check <镜像>... [--acr ns]` | 检查镜像与 ACR 归属冲突 |
| `dsync suggest <镜像>` | 推荐目标 ACR 及理由 |

所有列表/查询命令均支持 `--json` 输出，便于脚本处理，例如：

```bash
dsync sync nginx:1.25 --no-wait --json | jq -r .task_id
```

## 同步命令详解

```bash
dsync sync nginx:1.25
dsync sync ghcr.io/prometheus/prometheus:v2.50.0 --acr my-ns
dsync sync k8s.gcr.io/pause:3.9 --target-tag 3.9-amd64 --arch amd64
dsync sync nginx:1.25 --no-wait       # 只拿 task-id，不等待
```

- `--acr <alias>`：指定目标镜像仓库（别名优先，兼容 namespace）。**不指定时按服务端亲和性逻辑自动选择**（与 Web 端一致），CLI 会打印所选 ACR 与理由。
- `--target-tag`：目标 Tag，默认沿用源镜像 Tag。
- `--arch`：目标架构（amd64/arm64）。
- `--force`：跳过提交前查重拦截。
- `--no-wait`：提交后立即返回 task-id。
- 默认每 5 秒轮询一次进度，`Ctrl+C` 中断等待不影响服务端任务，可用 `dsync task status <id>` 继续查看。

### 提交前自动查重

`dsync sync` 提交前自动执行两级检查（`--force` 跳过）：

1. **仓库归属**：仓库已归属其他 ACR 时拦截（避免跨 ACR 重复建仓）；
2. **Tag 级查重**：目标 ACR 已存在相同 `仓库:Tag` 时拦截（纯重复同步）。

仓库已存在但 Tag 是新的会正常放行（追加 Tag），并以提示说明。

### 限流说明

平台对 `/sync/submit`、`/sync/batch` 有独立限流：**每 IP 每 12 秒仅允许 1 次提交**。CLI 遇到 429 会自动等待 13 秒重试（最多 3 次）。

## 批量同步

文件格式（每行一个镜像，`#` 为注释）：

```
# 每行: 源镜像[:tag][ 空格 目标Tag]
nginx:1.25
redis:7-alpine  7-alpine-amd64
k8s.gcr.io/pause:3.9  3.9
```

```bash
dsync batch -f images.txt --acr my-ns --auto-retry
```

> **注意**：批量同步多于 1 个镜像时，服务端会按仓库亲和性自动分配 ACR，`--acr` 仅作为无归属仓库的首选，并非强制指定。

## 搜索与 Tag 缓存

- **仓库名搜索**来自平台本地库，即时返回；平台本地库与 ACR 实际状态可能有漂移，可在 Web 端执行"从同步记录导入/清理无效镜像"对账。
- **Tag 搜索**来自本地缓存（`~/.config/dsync/tag-cache.json`），超过 24 小时的条目在下次搜索时自动增量刷新；`--refresh` 强制全量刷新。首次搜索会拉取全部仓库的 Tag，视仓库数量可能耗时几十秒。

## 凭据与安全

- 配置文件 `~/.config/dsync/config.json` 权限 0600，包含服务器地址、用户名、密码与 token；
- 密码输入不回显；token 过期后自动用保存的密码重新登录；
- `dsync logout` 同时清除本地凭据并使服务端 token 失效；
- CLI 使用的账号角色需要平台 `sync` 与 `images` 权限，否则会提示 403。

## 常见问题

**Q: 提示"未找到 namespace 为 xxx 的 ACR"？**
先执行 `dsync acr list` 查看可用 namespace，`--acr` 参数使用 NAMESPACE 列的值（不是 registry 地址）。

**Q: 同步失败怎么办？**
`dsync task status <task-id>` 查看错误信息与 GitHub Actions 链接；修复后 `dsync retry <record-id>` 重试。

**Q: 搜索结果不全？**
Tag 搜索依赖缓存，用 `dsync search <kw> --refresh` 强制刷新；仓库搜索依赖平台本地库，需在 Web 端对账后才会包含最新镜像。
