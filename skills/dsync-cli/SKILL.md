---
name: dsync-cli
description: >-
  使用 dsync CLI 操作 Docker 镜像同步平台：将海外镜像（Docker Hub / ghcr.io / k8s.gcr.io / quay.io 等）
  同步到本平台绑定的镜像仓库（阿里云 ACR / 华为云 SWR / 腾讯云 CCR / Harbor / 通用 Registry）并获取国内可直接拉取的地址；查询各仓库的镜像、Tag 与配额用量；
  跨仓库搜索镜像、确认某镜像是否已同步过；查看同步任务进度、排查并重试失败任务；
  把 k8s 部署文件里的海外镜像整体换成国内地址。

  触发场景：用户想同步/搬运/转存镜像到国内、需要国内拉取地址、问"仓库里有哪些镜像/Tag/配额"、
  搜索或确认镜像是否同步过、查询同步任务是否完成、部署文件换国内源；
  以及症状式表达——docker pull 超时/失败/卡住、k8s ImagePullBackOff/ErrImagePull 等海外源导致的拉取问题
  （即使没提"同步"或"国内"也应触发）。

  不适用：镜像构建与 Dockerfile 编写、在仓库建仓/删仓、仓库凭证管理、同步到平台未支持类型的仓库。
---

# dsync CLI · Docker 镜像同步平台操作

通过 `dsync` 命令行客户端操作镜像同步平台。平台负责把海外镜像经 GitHub Actions 同步到绑定的镜像仓库（阿里云 ACR / 华为云 SWR / 腾讯云 CCR / Harbor / 通用 Registry，`acr list` 的 TYPE 列区分类型），返回国内可直接 `docker pull` 的地址。仓库以**别名（ALIAS）**标识展示与选择（各厂商命名空间可能同名），`--acr` 优先按别名匹配、兼容 namespace。

## 前置检查与登录态

**执行任何 dsync 命令前，先确认二进制可用**。按顺序检查，取第一个可用的：

```bash
command -v dsync    # ① 全局安装的 dsync（make cli-install 或 Release 安装）
ls bin/dsync        # ② 仓库内构建产物
```

两者都没有时**必须先安装，不要直接执行 dsync 命令**。在仓库根目录任选一种方式：

```bash
# 方式一：仓库内构建（需要 Go 1.21+，无需 root，产物 bin/dsync）
make cli

# 方式二：构建并安装全局命令（需要 root，之后可直接用 dsync）
make cli && sudo make cli-install

# 方式三：下载预编译二进制（无 Go 环境时；linux/darwin × amd64/arm64，按平台替换文件名）
curl -LO https://github.com/lpx0312/docker-image-sync-platform/releases/latest/download/dsync-linux-amd64.tar.gz
tar -xzf dsync-linux-amd64.tar.gz && sudo install -m 0755 dsync /usr/local/bin/dsync

# GitHub 直连超时/失败时，在原 URL 前加代理前缀 https://gh.1102345.xyz/ 重试：
curl -LO https://gh.1102345.xyz/https://github.com/lpx0312/docker-image-sync-platform/releases/latest/download/dsync-linux-amd64.tar.gz
```

安装后用 `dsync --version`（或 `bin/dsync --version`）验证。本文后续命令统一写作 `bin/dsync`；若①/方式二/方式三已全局可用，把 `bin/dsync` 换成 `dsync` 即可。

**登录态**：

- 本机通常已在 `~/.config/dsync/config.json` 保存登录态（服务器 `https://sync.sktill.top:7000`，token 过期会自动续登）。
- 遇到 401/未登录：让用户提供密码并通过环境变量登录（密码不落命令行参数）：
  ```bash
  DSYNC_PASSWORD='<密码>' bin/dsync login --server https://sync.sktill.top:7000 --username <用户名>
  ```
- 验证登录态：`bin/dsync whoami`。

## 命令速查

| 需求 | 命令 |
|---|---|
| 同步单个镜像并等待完成 | `bin/dsync sync <镜像>[:tag]` |
| 同步到指定仓库 | `bin/dsync sync nginx:1.25 --acr <别名>` |
| 批量同步 | `bin/dsync batch -f images.txt [--acr ns]` |
| 只提交不等待（脚本场景） | `bin/dsync sync <镜像> --no-wait` |
| 查任务状态 | `bin/dsync task status <task-id>` / `task list [--status failed]` |
| 重试失败记录 | `bin/dsync retry --task <task-id>` |
| 查镜像同步记录（单条维度，支持过滤/搜索/去重） | `bin/dsync image list [--status failed] [--search kw]` |
| 查镜像同步状态统计 | `bin/dsync image stats` |
| 删除镜像同步记录（仅删平台记录，不动远程） | `bin/dsync image delete <id> --yes` |
| 校验镜像在远程仓库是否存在并更新状态 | `bin/dsync image check <id>` |
| 列出镜像仓库、配额用量（TYPE 列区分 5 种类型，ALIAS 列为引用标识；除 ACR 外配额「不限」） | `bin/dsync acr list` |
| 测试仓库配置连通性（登录凭证 + SWR/CCR 管理面凭证） | `bin/dsync acr test [别名]` |
| 查某仓库的镜像台账 | `bin/dsync repo list [--acr ns] [--filter kw]` |
| 从远程仓库导入镜像列表 | `bin/dsync repo import --acr <别名>` |
| 查某仓库的全部 Tag | `bin/dsync tag list <仓库名> [--acr ns]` |
| 查某 Tag 的详情（架构/digest/大小/推送时间） | `bin/dsync tag detail <仓库名> <tag> [--acr ns]` |
| 跨仓库搜仓库和 Tag | `bin/dsync search <关键词>` |
| 查镜像归属/推荐目标仓库 | `bin/dsync check <镜像>` / `bin/dsync suggest <镜像>` |
| 修改当前用户密码 | `bin/dsync passwd` |

所有命令支持 `--json`（脚本处理时用它，配合 jq）。

## 完整拉取地址（交付给用户前必须确认）

给用户的地址必须是**可直接 `docker pull` 的完整地址**：`<registry域名>/<namespace>/<仓库>:<tag>`。

- `sync` 成功输出和**查重拦截消息**中的地址已含完整域名，可直接转述。
- 查询命令（`tag list` / `search` / `check`）的输出**不含 registry 域名**，拼接方法：`acr list` 查 REGISTRY 列，按 `REGISTRY/namespace/仓库:tag` 拼出。

## 同步：核心用法与关键行为

**先查再提交**。用户只要"能拉的地址"时，先 `check <镜像>` + `tag list <仓库>` 确认是否已同步过：已存在就直接给完整地址，**跳过提交**；未同步且用户没有明确要求同步时，报告查询结果并等待指示，**不要自动提交**。

**同步是异步任务**。`dsync sync` 默认阻塞轮询直到完成（通常 1~5 分钟），成功后打印 `✓ 源镜像 -> 目标仓库完整地址`——这是交付物，直接转述给用户。脚本/批量场景加 `--no-wait` 拿 task-id，之后 `task status <id>` 查询。

**查重拦截是特性，不是错误**。提交前 CLI 自动检查，遇到以下报错说明镜像早已同步过，**把拦截消息里的完整地址直接给用户即可，不要加 --force 重试**：

```
Error: 已拦截: 镜像已存在于目标仓库（registry.cn-hangzhou.aliyuncs.com/lpx03/nginx:1.25.1），无需重复同步；确要重试请加 --force
```

只有用户明确说要"重新同步/覆盖"时才用 `--force`。

**目标仓库用别名引用**（`acr list` 的 ALIAS 列，兼容直接传 namespace；namespace 同名冲突时报错要求改用别名），不是 registry 地址。不指定时 CLI 自动按亲和性选择并打印所选仓库——向用户复述这一行即可。

**执行确认策略**：用户明确说"同步 xxx"→ 直接执行；AI 自己发起的批量同步 → 先列清单确认（模板见下），确认后执行。

**提交限流**：平台每 12 秒只允许 1 次提交，CLI 遇 429 自动退避重试。看到"限流重试"提示时等待即可，不要手动反复重试。

## 批量同步与"换源"完整闭环

用户的"把部署文件里的海外镜像换成国内地址"包含两半：**同步 + 写回文件**，两半都要完成。

1. 解析文件提取镜像列表（如 `grep image:` deployment.yaml）。
2. 逐个 `check` 查归属，`tag list` 实时确认哪些已存在（已存在的直接复用地址，无需同步）。
3. **向用户确认**，模板：

   > 在 `<文件>` 中找到 N 个海外镜像，其中 X 个已同步过可直接使用，Y 个需要新建同步：
   >
   > | 源镜像 | 目标 ACR | 依据 |
   > |---|---|---|
   > | k8s.gcr.io/pause:3.9 | lpx0312 | 新仓库，平台默认 |
   >
   > 将执行 `bin/dsync batch -f <临时文件> [--acr ns]`，是否确认？

4. 确认后写入临时文件执行 `batch`，等待完成。
5. **写回文件**：用编辑工具原位替换每个 `image:` 为完整国内地址，不改动其他内容，最后把修改结果展示给用户。

**batch 的 --acr 只影响无归属的新仓库**：已有归属的镜像永远进其归属 ACR（亲和性），所以混合目标的批量一次 `batch` 即可完成，不要拆成多次。批量文件格式：每行 `源镜像[:tag]`，可加空格分隔的目标 Tag，`#` 为注释。

## 查询：数据源差异

- `image list` / `task list` 查平台数据库的同步记录：`task list` 是任务维度（一个任务含多条镜像），`image list` 是单条镜像记录维度（支持状态/架构过滤、关键词搜索、去重）。排查具体某条镜像用 `image list`，看整体任务用 `task list`。
- `repo list` / `search` 的**仓库匹配**查平台本地库（即时）；本地库与远程仓库实际状态可能漂移，查不到不代表远程没有。对账用 `repo import`（远程拉取补齐）/ `repo sync-records`（同步记录补齐）/ `repo clean --yes`（清理失效记录）。
- `tag list` 是**实时**查目标仓库（按类型自动适配认证），最准确，查重判断以它为准。核对某个具体 Tag 是否存在时用 `--json` 精确匹配（如 `tag list <repo> --json | jq -r '.[].Tags[]' | grep -x <tag>`），勿凭多列排版目测——相近 Tag（如 `7.2.12` 与 `7.2.12-alpine`）肉眼极易混淆。
- `tag list` 传裸仓库名即可（自动定位）；仓库在多个 ACR 存在时全部展示；本地库漂移导致定位失败时加 `--acr` 指定。
- `search` 的 Tag 匹配来自本地缓存：首次搜索会拉全部仓库 Tag 建缓存（可能几十秒），之后秒出；出现"N 个仓库 Tag 拉取失败"警告时，对关键镜像用 `tag list` 实时验证，勿只信缓存。
- Tag 里的 `-amd64-tmp` / `-arm64-tmp` 后缀是平台同步多架构镜像的中间产物，**给用户无后缀的主 Tag 即可**（指向多架构 manifest）。

## 失败处理

```bash
bin/dsync task status <task-id>   # 看错误信息 + GitHub Actions 链接
bin/dsync task list --status failed   # 最近的失败任务
bin/dsync image list --status failed  # 失败的镜像记录（单条维度）
bin/dsync retry --task <task-id>  # 重置该任务全部失败记录
bin/dsync image check <id>        # 校验某条记录在远程是否真实存在，状态不一致时自动修正
```

常见失败原因：源镜像地址/Tag 不存在、GitHub Actions 配额、网络问题。重试后任务由平台重新调度，用 `task status` 跟踪。`image check` 用于怀疑状态与远程实际不一致时（如手动删过远程镜像、Actions 异常），它会按远程真实存在性自动更新记录状态。

## 403 权限错误

当前账号角色缺少 sync/images 权限，让用户找管理员调整，不要绕过。
