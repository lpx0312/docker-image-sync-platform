# Docker 镜像同步平台 — E2E 功能测试指南

> **版本**：v1.0  
> **更新日期**：2026-04-03  
> **测试账号**：`<测试用户名> / <测试密码>`（管理员角色，请使用本地环境的测试账号，勿写入文档）

---

## 一、前置要求

### 1.1 必须使用 `chrome-devtools-cli` Skill

**本测试流程强制要求使用项目内置的 `chrome-devtools-cli` Skill 进行自动化测试。**

Skill 路径：`.cursor/skills/chrome-devtools-cli/SKILL.md`

使用前请先读取该 Skill，然后按照 Skill 中的 AI Workflow 执行。Skill 核心工作流如下：

1. **执行**：直接运行 `chrome-devtools <tool>` 命令，后台服务会隐式启动，**不要**在每次使用前手动运行 `start`/`status`/`stop`。
2. **审查**：使用 `take_snapshot` 获取元素 `<uid>`。
3. **操作**：使用 `click`、`fill` 等命令，状态在命令间持久保持。

### 1.2 环境准备

| 服务 | 地址 | 启动命令 |
|------|------|----------|
| MySQL | `127.0.0.1:3306` | `sudo docker start docker-sync-mysql-dev` |
| 后端 Go | `http://localhost:8080` | `go run main.go` |
| 前端 Vue | `http://localhost:3000` | `cd web && npm run dev` |

启动前检查端口：

```bash
ss -tlnp | grep -E ':3000|:8080|:3306'
```

---

## 二、测试顺序总览

| 序号 | 模块 | 测试点 |
|------|------|--------|
| T0 | 后端健康与鉴权 | `/health` 接口、无 Token 访问 401 |
| T1 | 登录 | 空提交校验、错误密码、正确登录 |
| T2 | 单个镜像同步 | 空提交校验、架构切换、字数计数、重置 |
| T3 | 批量镜像同步 | Tab 切换、手动输入/文件导入 Radio、表单填写 |
| T4 | 镜像列表 | 搜索、状态/架构筛选、去重勾选、批量检测、复制、详情弹层 |
| T5 | 系统配置 - Git | 字段完整性、仓库类型切换（Gitee ↔ GitHub） |
| T6 | 系统配置 - 镜像仓库 | 字段完整性、密码填写后保存按钮激活 |
| T7 | 系统配置 - 系统设置 | 同步间隔/最大并发 spinbutton 调整 |
| T8 | GitHub Actions | 检查 API 限制、状态筛选、运行详情弹层 |
| T9 | 用户管理 | 新建用户弹层校验、修改密码校验、登录日志筛选 |
| T10 | 全局质量检查 | Console error/warn、4xx/5xx 网络请求汇总 |

---

## 三、详细测试步骤

### T0 — 后端健康与鉴权

```bash
# 1. 健康检查
curl -s http://localhost:8080/api/v1/health

# 预期：{"status":"ok","timestamp":...,"version":"1.0.0"}

# 2. 鉴权守卫（无 token 访问受保护接口）
curl -s -o /dev/null -w "HTTP %{http_code}" http://localhost:8080/api/v1/images/list

# 预期：HTTP 401
```

**通过标准**：health 返回 200，无 token 返回 401。

---

### T1 — 登录

```bash
# 打开登录页
chrome-devtools navigate_page --url "http://localhost:3000/login" --timeout 12000

# 获取元素 UID
chrome-devtools take_snapshot
```

**T1-a：空提交校验**

```bash
chrome-devtools click "<登录按钮UID>"
# 预期：出现「请输入用户名」「请输入密码」提示
```

**T1-b：错误密码**

```bash
chrome-devtools fill "<用户名输入框UID>" "<测试用户名>"
chrome-devtools fill "<密码输入框UID>" "wrongpass"
chrome-devtools click "<登录按钮UID>"
# 预期：Toast「用户名或密码错误」
```

**T1-c：正确登录**

```bash
chrome-devtools fill "<用户名输入框UID>" "<测试用户名>"
chrome-devtools fill "<密码输入框UID>" "<测试密码>"
chrome-devtools click "<登录按钮UID>"
# 预期：跳转 /sync，Toast「登录成功」，导航栏显示「<测试用户名首字母> <测试用户名>」
```

**通过标准**：三种场景结果均符合预期。

---

### T2 — 单个镜像同步

> 前提：已登录，当前页为 `/sync`，单个同步 Tab 已选中。

**T2-a：空提交校验**

```bash
chrome-devtools click "<开始同步按钮UID>"
# 预期：「请输入源镜像地址」校验提示
```

**T2-b：架构切换**

```bash
# 通过 JS 选择 arm64
chrome-devtools evaluate_script "() => {
  const items = document.querySelectorAll('.el-select-dropdown__item');
  for(const i of items) {
    if(i.textContent.trim()==='arm64') { i.click(); return 'ok'; }
  }
}"
# 预期：架构下拉显示 arm64
```

**T2-c：同步说明字数计数**

```bash
chrome-devtools fill "<同步说明textboxUID>" "测试说明内容"
# 预期：「X / 500」计数器随输入更新
```

**T2-d：重置**

```bash
chrome-devtools click "<重置按钮UID>"
# 预期：说明清空，计数归 0 / 500，架构恢复 amd64
```

**通过标准**：四个场景全部符合预期。

---

### T3 — 批量镜像同步

**T3-a：Tab 切换**

```bash
chrome-devtools click "<批量同步TabUID>"
# 预期：页面切换，出现「手动输入」「文件导入」Radio
```

**T3-b：文件导入 Radio**

```bash
chrome-devtools click "<文件导入RadioUID>"
# 预期：出现「选择文件」按钮，提示支持 .txt/.csv
```

**T3-c：手动输入表单**

```bash
chrome-devtools click "<手动输入RadioUID>"
chrome-devtools fill "<镜像列表textareaUID>" "redis:7.0"
chrome-devtools click "<增加数值按钮UID>"   # 并发 +1
chrome-devtools click "<仅ARM64RadioUID>"
# 预期：textarea 有值，并发 3→4，架构显示「仅 ARM64」
```

**通过标准**：文件/手动 Radio 切换正常，表单交互有效。

---

### T4 — 镜像列表

> 位于 `/sync` 页下方，单个同步 Tab 下可见。

**T4-a：搜索**

```bash
chrome-devtools fill "<搜索框UID>" "nginx"
# 预期：列表实时过滤，API 参数 search=nginx
```

**T4-b：状态筛选**

```bash
chrome-devtools click "<状态下拉UID>"
# 通过 JS 选择「成功」
chrome-devtools evaluate_script "() => {
  const items = document.querySelectorAll('.el-select-dropdown__item');
  for(const i of items) {
    if(i.textContent.trim()==='成功') { i.click(); return 'ok'; }
  }
}"
# 预期：API 参数 status=success，仅显示成功记录
```

**T4-c：架构筛选**

```bash
chrome-devtools click "<架构下拉UID>"
# JS 选择「arm64」
# 预期：列表仅显示 arm64 架构记录
```

**T4-d：清除筛选**

```bash
chrome-devtools click "<清除筛选按钮UID>"
# 预期：恢复全量数据
```

**T4-e：去重勾选**

```bash
# 通过 JS 点击 checkbox
chrome-devtools evaluate_script "() => {
  document.querySelector('.el-checkbox__input').click();
  return 'ok';
}"
# 预期：总条数变化（有重复镜像时去重前 > 去重后）
```

**T4-f：复制**

```bash
chrome-devtools click "<复制按钮UID>"
# 预期：Toast「已复制到剪贴板」
```

**T4-g：单条 ACR 检测**

```bash
chrome-devtools click "<检测按钮UID>"
# 预期：POST /images/:id/check 返回 200，contains exists 字段
```

验证 API 响应：

```bash
chrome-devtools list_network_requests --resourceTypes xhr --pageSize 20
chrome-devtools get_network_request --reqid <reqid>
# 预期响应体包含 {"exists":true/false,"message":"...","target_image":"..."}
```

**T4-h：详情弹层**

```bash
chrome-devtools click "<详情按钮UID>"
# 预期：弹出「镜像详情」对话框，包含 ID/源镜像/目标镜像/状态/任务ID/时间
chrome-devtools click "<关闭按钮UID>"
```

**T4-i：批量检测**

```bash
chrome-devtools click "<批量检测按钮UID>"
# 预期：按钮短暂变 disabled（请求中），完成后恢复可用
```

**通过标准**：9 个交互点全部符合预期，API 返回 200。

---

### T5 — 系统配置 Git

```bash
chrome-devtools navigate_page --url "http://localhost:3000/config" --timeout 12000
chrome-devtools click "<Git配置TabUID>"
```

**T5-a：仓库类型切换**

```bash
chrome-devtools click "<Gitee RadioUID>"
# 预期：Toast「Git仓库类型已更新为 Gitee」

chrome-devtools click "<GitHub RadioUID>"
# 预期：Toast「Git仓库类型已更新为 GitHub」
```

**T5-b：保存按钮激活逻辑**

```bash
# GitHub 访问令牌字段为必填，空时保存按钮 disabled
chrome-devtools fill "<访问令牌textboxUID>" "test-token"
# 预期：「保存并测试配置」按钮变为可用

# Gitee 密码字段同理
chrome-devtools fill "<Gitee密码textboxUID>" "test-pass"
# 预期：Gitee「保存并测试配置」按钮变为可用
```

**通过标准**：切换有 Toast 反馈，密码/Token 填写后按钮激活。

---

### T6 — 系统配置镜像仓库

```bash
chrome-devtools click "<镜像仓库配置TabUID>"
```

**T6-a：字段完整性**

```bash
chrome-devtools take_snapshot
# 预期：存在「镜像仓库URL」「命名空间」「用户名」「密码」四个字段
```

**T6-b：密码填写触发保存按钮**

```bash
chrome-devtools fill "<密码textboxUID>" "TestPass"
# 预期：「保存并测试配置」按钮由 disabled 变为可用
```

**通过标准**：四个字段均存在且有预设值，密码填写后保存按钮激活。

---

### T7 — 系统配置系统设置

```bash
chrome-devtools click "<系统设置TabUID>"
chrome-devtools take_snapshot
# 预期：存在「同步间隔」「最大并发数」两个 spinbutton
```

**T7-a：同步间隔调整**

```bash
chrome-devtools click "<同步间隔增加按钮UID>"
# 预期：值 +1（如 60 → 61），范围 [1, 1440]
```

**T7-b：最大并发调整**

```bash
chrome-devtools click "<最大并发增加按钮UID>"
# 预期：值 +1（如 3 → 4）

chrome-devtools click "<最大并发减少按钮UID>"
# 预期：值 -1（如 4 → 3），范围 [1, 10]
```

**通过标准**：spinbutton 增减正常，边界值符合范围限制。

---

### T8 — GitHub Actions

```bash
chrome-devtools navigate_page --url "http://localhost:3000/github" --timeout 12000
```

**T8-a：检查 API 限制**

```bash
chrome-devtools click "<检查限制按钮UID>"
# 预期：显示「剩余请求 / 总请求限制 / 重置时间 / 使用率」
```

**T8-b：状态筛选**

```bash
chrome-devtools click "<状态下拉UID>"
# JS 选择「已完成」
chrome-devtools evaluate_script "() => {
  const items = document.querySelectorAll('.el-select-dropdown__item');
  for(const i of items) {
    if(i.textContent.trim()==='已完成') { i.click(); return 'ok'; }
  }
}"
# 预期：列表仅显示已完成记录

chrome-devtools click "<清除筛选按钮UID>"
# 预期：恢复全量记录
```

**T8-c：运行详情弹层**

```bash
chrome-devtools click "<第一条详情按钮UID>"
# 预期：弹出「GitHub Actions 运行详情」对话框，包含：
#   - 运行 ID
#   - 状态 / 结果
#   - 提交 SHA（完整 hash）
#   - 创建/更新时间
#   - 持续时间
#   - GitHub 链接（可跳转）

chrome-devtools click "<关闭按钮UID>"
```

**通过标准**：限制数据有值，筛选生效，详情弹层字段完整。

---

### T9 — 用户管理

```bash
chrome-devtools navigate_page --url "http://localhost:3000/users" --timeout 10000
```

**T9-a：新建用户弹层**

```bash
chrome-devtools click "<新建用户按钮UID>"
# 预期：弹出「新建用户」对话框，包含用户名/密码/邮箱/角色四个字段

# 验证角色下拉选项
chrome-devtools click "<角色下拉UID>"
chrome-devtools evaluate_script "() => Array.from(document.querySelectorAll('.el-select-dropdown__item')).map(o=>o.textContent.trim())"
# 预期：["普通用户", "运维员", "管理员"]

# 空提交校验
chrome-devtools click "<创建按钮UID>"
# 预期：「请输入用户名」「请输入密码」提示

chrome-devtools click "<取消按钮UID>"
```

**T9-b：修改密码弹层**

```bash
# 从右上角用户菜单进入
chrome-devtools click "<用户菜单按钮UID>"
chrome-devtools click "<修改密码menuitemUID>"
# 预期：弹出「修改密码」对话框，含原密码/新密码/确认密码

# 两次密码不一致校验
chrome-devtools fill "<原密码UID>" "<测试密码>"
chrome-devtools fill "<新密码UID>" "NewPass123!"
chrome-devtools fill "<确认密码UID>" "DifferentPass!"
chrome-devtools click "<确认修改按钮UID>"
# 预期：「两次输入的密码不一致」提示

chrome-devtools click "<取消按钮UID>"
```

**T9-c：登录日志筛选**

```bash
chrome-devtools click "<登录日志TabUID>"
chrome-devtools fill "<搜索用户名textboxUID>" "<测试用户名>"
chrome-devtools press_key "Enter"
# 预期：结果条数减少（仅显示该用户的日志）
```

**通过标准**：新建弹层字段完整、角色选项齐全、校验提示正确，密码不一致报错，日志搜索生效。

---

### T10 — 全局质量检查

```bash
# 检查 Console 错误
chrome-devtools list_console_messages --types error --pageSize 20
# 预期：<no console messages found>

# 检查 Console 警告
chrome-devtools list_console_messages --types warn --pageSize 20
# 预期：<no console messages found>

# 检查网络 4xx/5xx（排除登录前预期的 401）
chrome-devtools list_network_requests --resourceTypes xhr --pageSize 50
# 预期：会话内除登出前 401 外，无其他 4xx/5xx 错误
```

**通过标准**：Console 无 error/warn，API 请求无非预期错误。

---

## 四、退出登录

```bash
chrome-devtools click "<用户菜单按钮UID>"
chrome-devtools click "<退出登录menuitemUID>"
# 出现确认弹层
chrome-devtools click "<确定按钮UID>"
# 预期：跳转回 /login 页
```

---

## 五、常用 chrome-devtools-cli 命令速查

```bash
# 导航
chrome-devtools navigate_page --url "http://localhost:3000" --timeout 12000
chrome-devtools navigate_page --type "reload" --ignoreCache true

# 快照（获取 UID）
chrome-devtools take_snapshot
chrome-devtools take_snapshot --verbose true

# 交互
chrome-devtools click "<uid>"
chrome-devtools fill "<uid>" "文本内容"
chrome-devtools press_key "Enter"
chrome-devtools press_key "Escape"

# JS 执行（处理 Element Plus 下拉等特殊组件）
chrome-devtools evaluate_script "() => { ... }"

# 网络请求
chrome-devtools list_network_requests --resourceTypes xhr --pageSize 20
chrome-devtools get_network_request --reqid <reqid>

# 控制台
chrome-devtools list_console_messages --types error
chrome-devtools list_console_messages --types warn

# 截图
chrome-devtools take_screenshot
chrome-devtools take_screenshot --fullPage true
```

---

## 六、测试结果记录模板

| 测试点 | 预期结果 | 实际结果 | 状态 | 备注 |
|--------|----------|----------|------|------|
| T0 - /health | 200, status:ok | | ⬜ | |
| T0 - 无 token | 401 | | ⬜ | |
| T1 - 空提交 | 校验提示 | | ⬜ | |
| T1 - 错误密码 | 错误 Toast | | ⬜ | |
| T1 - 正确登录 | 跳转 /sync | | ⬜ | |
| T2 - 空提交 | 校验提示 | | ⬜ | |
| T2 - 架构切换 | arm64 选中 | | ⬜ | |
| T2 - 字数计数 | X / 500 | | ⬜ | |
| T2 - 重置 | 表单清空 | | ⬜ | |
| T3 - Tab 切换 | 批量表单出现 | | ⬜ | |
| T3 - 文件导入 | 选择文件按钮 | | ⬜ | |
| T3 - 手动输入 | 表单可填写 | | ⬜ | |
| T4 - 搜索 | 实时过滤 | | ⬜ | |
| T4 - 状态筛选 | 按状态过滤 | | ⬜ | |
| T4 - 架构筛选 | 按架构过滤 | | ⬜ | |
| T4 - 清除筛选 | 全量恢复 | | ⬜ | |
| T4 - 去重勾选 | 条数变化 | | ⬜ | |
| T4 - 复制 | 剪贴板 Toast | | ⬜ | |
| T4 - 单条检测 | check API 200 | | ⬜ | |
| T4 - 详情弹层 | 字段完整 | | ⬜ | |
| T4 - 批量检测 | 按钮 loading | | ⬜ | |
| T5 - 仓库切换 | Toast 反馈 | | ⬜ | |
| T5 - 保存激活 | 按钮可用 | | ⬜ | |
| T6 - 字段完整 | 4 字段存在 | | ⬜ | |
| T6 - 保存激活 | 按钮可用 | | ⬜ | |
| T7 - 同步间隔 | +1 正确 | | ⬜ | |
| T7 - 最大并发 | +1/-1 正确 | | ⬜ | |
| T8 - API 限制 | 数值显示 | | ⬜ | |
| T8 - 状态筛选 | 过滤生效 | | ⬜ | |
| T8 - 运行详情 | 弹层字段完整 | | ⬜ | |
| T9 - 新建弹层 | 字段+角色完整 | | ⬜ | |
| T9 - 空提交校验 | 提示显示 | | ⬜ | |
| T9 - 密码不一致 | 校验提示 | | ⬜ | |
| T9 - 日志筛选 | 条数减少 | | ⬜ | |
| T10 - Console | 0 error/warn | | ⬜ | |
| T10 - 网络请求 | 无非预期错误 | | ⬜ | |

> 状态：✅ 通过 / ❌ 失败 / ⚠️ 部分通过 / ⬜ 未测试
