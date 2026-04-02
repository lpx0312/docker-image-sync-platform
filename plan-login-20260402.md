# 登录功能实现计划（2026-04-02）

## 需求摘要

| 项目 | 选型 |
|------|------|
| 认证方式 | JWT Token（无状态） |
| 用户管理 | 多用户，数据库存储，支持注册/管理 |
| 登录页风格 | 简洁居中卡片（白色卡片 + 品牌色渐变背景） |
| 保护范围 | 全部页面和接口 |
| 附加功能 | 修改密码、记住登录状态、超时自动登出、登录日志 |

---

## 一、后端实现计划

### 阶段 1：数据模型（预计 30 分钟）

#### 1.1 用户表 `users`

```go
type User struct {
    ID           uint           `gorm:"primarykey" json:"id"`
    Username     string         `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
    PasswordHash string         `gorm:"type:varchar(255);not null" json:"-"`
    Email        string         `gorm:"type:varchar(100)" json:"email"`
    Role         string         `gorm:"type:enum('admin','user');default:'user'" json:"role"`
    Status       string         `gorm:"type:enum('active','disabled');default:'active'" json:"status"`
    LastLoginAt  *time.Time     `json:"last_login_at"`
    CreatedAt    time.Time      `json:"created_at"`
    UpdatedAt    time.Time      `json:"updated_at"`
    DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}
```

#### 1.2 登录日志表 `login_logs`

```go
type LoginLog struct {
    ID        uint      `gorm:"primarykey" json:"id"`
    UserID    uint      `gorm:"index;not null" json:"user_id"`
    Username  string    `gorm:"type:varchar(50);not null" json:"username"`
    IP        string    `gorm:"type:varchar(45)" json:"ip"`
    UserAgent string    `gorm:"type:varchar(500)" json:"user_agent"`
    Status    string    `gorm:"type:enum('success','failed');not null" json:"status"`
    Message   string    `gorm:"type:varchar(255)" json:"message"`
    CreatedAt time.Time `gorm:"index" json:"created_at"`
}
```

#### 1.3 数据库迁移
- 在 `database.go` 的 `AutoMigrate` 中增加 `User` 和 `LoginLog`
- 首次启动时自动创建默认管理员账号（admin / admin123），并在日志中提示修改默认密码

**涉及文件**：
- `internal/models/models.go` — 新增模型
- `internal/database/database.go` — 注册迁移 + 初始化默认用户

---

### 阶段 2：配置项（预计 10 分钟）

在 `config.yaml` 中增加 JWT 相关配置：

```yaml
auth:
  jwt_secret: "change-me-to-a-random-string"    # JWT 签名密钥
  token_expiry: 24h                              # Token 过期时间
  remember_me_expiry: 168h                       # 记住我时 Token 过期时间（7天）
  auto_logout_minutes: 30                        # 前端无操作超时（分钟）
  default_admin_username: "admin"                # 默认管理员用户名
  default_admin_password: "admin123"             # 默认管理员密码（首次初始化用）
```

**涉及文件**：
- `config.yaml` — 增加 auth 段
- `internal/config/config.go` — 增加 `AuthConfig` 结构体并映射

---

### 阶段 3：JWT 服务（预计 30 分钟）

新建 `internal/services/auth.go`：

- `GenerateToken(userID, username, role, rememberMe) (string, error)` — 生成 JWT
- `ValidateToken(tokenString) (*Claims, error)` — 验证并解析 Token
- `HashPassword(password) (string, error)` — bcrypt 加密
- `CheckPassword(hash, password) bool` — 校验密码

JWT Claims 结构：
```go
type Claims struct {
    UserID   uint   `json:"user_id"`
    Username string `json:"username"`
    Role     string `json:"role"`
    jwt.RegisteredClaims
}
```

**依赖**：`golang-jwt/jwt/v5`、`golang.org/x/crypto/bcrypt`

---

### 阶段 4：用户服务（预计 30 分钟）

新建 `internal/services/user.go`：

- `CreateUser(username, password, email, role) (*User, error)`
- `GetUserByUsername(username) (*User, error)`
- `GetUserByID(id) (*User, error)`
- `ListUsers(page, pageSize) ([]User, int64, error)`
- `UpdateUserStatus(id, status) error`
- `ChangePassword(userID, oldPassword, newPassword) error`
- `DeleteUser(id) error`
- `RecordLoginLog(userID, username, ip, userAgent, status, message) error`
- `GetLoginLogs(page, pageSize, username) ([]LoginLog, int64, error)`

---

### 阶段 5：认证中间件（预计 20 分钟）

新建 `internal/middleware/auth.go`：

- `AuthRequired()` — 校验 JWT，从 `Authorization: Bearer <token>` 提取
  - 成功：将 `userID`、`username`、`role` 写入 `gin.Context`
  - 失败：返回 `401 Unauthorized`
- `AdminRequired()` — 在 `AuthRequired` 基础上检查 `role == "admin"`
  - 失败：返回 `403 Forbidden`

---

### 阶段 6：认证 Handler（预计 40 分钟）

新建 `internal/handlers/auth.go`：

| 方法 | 路由 | 说明 | 权限 |
|------|------|------|------|
| `Login` | `POST /api/v1/auth/login` | 登录，返回 JWT | 公开 |
| `GetCurrentUser` | `GET /api/v1/auth/me` | 获取当前用户信息 | 登录 |
| `ChangePassword` | `PUT /api/v1/auth/password` | 修改自己的密码 | 登录 |
| `Logout` | `POST /api/v1/auth/logout` | 登出（前端清 Token，后端记录日志） | 登录 |
| `GetLoginLogs` | `GET /api/v1/auth/login-logs` | 查看登录日志 | 管理员 |
| `ListUsers` | `GET /api/v1/auth/users` | 用户列表 | 管理员 |
| `CreateUser` | `POST /api/v1/auth/users` | 创建用户 | 管理员 |
| `UpdateUserStatus` | `PUT /api/v1/auth/users/:id/status` | 启用/禁用用户 | 管理员 |
| `DeleteUser` | `DELETE /api/v1/auth/users/:id` | 删除用户 | 管理员 |
| `ResetUserPassword` | `PUT /api/v1/auth/users/:id/password` | 管理员重置用户密码 | 管理员 |

登录请求体：
```json
{
  "username": "admin",
  "password": "admin123",
  "remember_me": true
}
```

登录响应：
```json
{
  "token": "eyJhbGciOiJIUzI1NiI...",
  "user": {
    "id": 1,
    "username": "admin",
    "role": "admin",
    "email": "admin@example.com"
  },
  "expires_at": "2026-04-03T10:00:00Z"
}
```

---

### 阶段 7：路由改造（预计 20 分钟）

`main.go` 路由结构调整：

```
/api/v1/
├── auth/
│   ├── POST   /login              (公开)
│   ├── GET    /me                  (登录)
│   ├── PUT    /password            (登录)
│   ├── POST   /logout              (登录)
│   ├── GET    /login-logs          (管理员)
│   ├── GET    /users               (管理员)
│   ├── POST   /users               (管理员)
│   ├── PUT    /users/:id/status    (管理员)
│   ├── DELETE /users/:id           (管理员)
│   └── PUT    /users/:id/password  (管理员)
├── health                          (公开)
└── [以下全部需要 AuthRequired 中间件]
    ├── sync/...
    ├── images/...
    ├── config/...
    └── github/...
```

**涉及文件**：
- `main.go` — 重组路由，为受保护路由组增加 `middleware.AuthRequired()`

---

## 二、前端实现计划

### 阶段 8：API 层（预计 15 分钟）

在 `web/src/api/index.js` 中新增：

```javascript
export const authAPI = {
  login(data) {},           // POST /auth/login
  getCurrentUser() {},      // GET /auth/me
  changePassword(data) {},  // PUT /auth/password
  logout() {},              // POST /auth/logout
  getLoginLogs(params) {},  // GET /auth/login-logs
  listUsers(params) {},     // GET /auth/users
  createUser(data) {},      // POST /auth/users
  updateUserStatus(id, data) {},  // PUT /auth/users/:id/status
  deleteUser(id) {},        // DELETE /auth/users/:id
  resetUserPassword(id, data) {}, // PUT /auth/users/:id/password
}
```

改造 Axios 拦截器：
- **请求拦截器**：从 `localStorage`/`sessionStorage` 读取 Token，添加 `Authorization: Bearer <token>`
- **响应拦截器**：收到 `401` 时清除 Token 并跳转登录页

---

### 阶段 9：Auth Store（预计 20 分钟）

新建 `web/src/stores/auth.js`（Pinia）：

```
state:
  - token: string
  - user: { id, username, role, email }
  - isLoggedIn: boolean
  - lastActivityTime: number

actions:
  - login(username, password, rememberMe)
  - logout()
  - fetchCurrentUser()
  - changePassword(oldPassword, newPassword)
  - checkAutoLogout()        // 检查无操作超时
  - updateActivityTime()     // 更新最后活跃时间

getters:
  - isAdmin: boolean
  - tokenExpired: boolean
```

Token 存储策略：
- **记住我** → `localStorage`（浏览器关闭后保留）
- **不记住** → `sessionStorage`（浏览器关闭后清除）

---

### 阶段 10：登录页面（预计 40 分钟）

新建 `web/src/views/LoginView.vue`：

设计方案：
- 全屏渐变背景（蓝紫品牌色渐变）
- 居中白色卡片，圆角 + 阴影
- 顶部：Docker 图标 + 平台名称
- 表单：用户名、密码输入框（带图标前缀）
- "记住我" 复选框
- 登录按钮（品牌色，loading 状态）
- 底部：版权信息

交互：
- Enter 键提交
- 表单校验（用户名和密码必填，密码最少 6 位）
- 登录失败显示错误信息（密码错误、账号被禁用等）
- 登录成功跳转到之前尝试访问的页面或首页

---

### 阶段 11：路由守卫（预计 15 分钟）

改造 `web/src/router/index.js`：

- 新增 `/login` 路由，指向 `LoginView.vue`
- `router.beforeEach` 全局守卫：
  - 未登录访问非 `/login` → 重定向到 `/login`，并保存原始路由到 `query.redirect`
  - 已登录访问 `/login` → 重定向到首页
  - 已登录正常放行

---

### 阶段 12：全局布局改造（预计 15 分钟）

改造 `web/src/App.vue`：

- 登录页不显示顶部导航和侧边栏（`v-if="isLoggedIn"` 控制）
- 导航栏右侧增加用户信息区域：
  - 显示用户名
  - 下拉菜单：修改密码、退出登录
  - 管理员额外显示：用户管理入口
- 超时自动登出逻辑：
  - 监听鼠标移动/键盘/点击事件更新活跃时间
  - 定时器每分钟检查是否超过配置的超时时间
  - 超时弹窗提示 → 确认后跳转登录页

---

### 阶段 13：用户管理页面（预计 40 分钟）

新建 `web/src/views/UserManageView.vue`（仅管理员可见）：

- 用户列表表格：ID、用户名、邮箱、角色、状态、最后登录时间、操作
- 新建用户对话框：用户名、密码、邮箱、角色
- 操作按钮：启用/禁用、重置密码、删除（带确认）
- 登录日志 Tab：时间、用户名、IP、状态、User-Agent

---

### 阶段 14：修改密码对话框（预计 15 分钟）

新建 `web/src/components/ChangePasswordDialog.vue`：

- 当前密码、新密码、确认新密码
- 校验：新密码 ≥ 6 位，两次输入一致，新旧不同
- 修改成功后清除 Token 并跳转登录页重新登录

---

## 三、涉及文件清单

### 新增文件

| 文件 | 说明 |
|------|------|
| `internal/services/auth.go` | JWT 生成/验证、密码加密 |
| `internal/services/user.go` | 用户 CRUD、登录日志 |
| `internal/handlers/auth.go` | 认证相关 HTTP Handler |
| `internal/middleware/auth.go` | JWT 认证中间件 |
| `web/src/views/LoginView.vue` | 登录页 |
| `web/src/views/UserManageView.vue` | 用户管理页 |
| `web/src/stores/auth.js` | 认证状态管理 |
| `web/src/components/ChangePasswordDialog.vue` | 修改密码对话框 |

### 修改文件

| 文件 | 改动内容 |
|------|----------|
| `internal/models/models.go` | 新增 User、LoginLog 模型 |
| `internal/database/database.go` | AutoMigrate + 默认管理员初始化 |
| `internal/config/config.go` | 新增 AuthConfig 结构体 |
| `config.yaml` | 新增 auth 配置段 |
| `main.go` | 重组路由，添加认证中间件 |
| `go.mod` | 新增 jwt、bcrypt 依赖 |
| `web/src/api/index.js` | 新增 authAPI + 拦截器改造 |
| `web/src/router/index.js` | 新增登录路由 + 全局守卫 |
| `web/src/App.vue` | 布局改造（条件渲染 + 用户菜单 + 超时检测） |

---

## 四、实施顺序与预估

| 序号 | 阶段 | 预估时间 | 依赖 |
|------|------|----------|------|
| 1 | 数据模型 | 30 min | 无 |
| 2 | 配置项 | 10 min | 无 |
| 3 | JWT 服务 | 30 min | 阶段 1, 2 |
| 4 | 用户服务 | 30 min | 阶段 1 |
| 5 | 认证中间件 | 20 min | 阶段 3 |
| 6 | 认证 Handler | 40 min | 阶段 3, 4, 5 |
| 7 | 路由改造 | 20 min | 阶段 5, 6 |
| 8 | 前端 API 层 | 15 min | 阶段 6 |
| 9 | Auth Store | 20 min | 阶段 8 |
| 10 | 登录页面 | 40 min | 阶段 9 |
| 11 | 路由守卫 | 15 min | 阶段 9 |
| 12 | 全局布局改造 | 15 min | 阶段 9, 11 |
| 13 | 用户管理页 | 40 min | 阶段 8, 9 |
| 14 | 修改密码对话框 | 15 min | 阶段 8, 9 |
| | **合计** | **约 5.5 小时** | |

---

## 五、测试要点

1. **登录流程**：正确用户名密码 → 获取 Token → 跳转首页
2. **错误处理**：错误密码 → 提示错误、账号禁用 → 提示禁用
3. **Token 校验**：无 Token 访问 API → 401、Token 过期 → 401 并跳转登录
4. **记住我**：勾选后关闭浏览器重开 → 仍保持登录
5. **超时登出**：无操作超过配置时间 → 弹窗提示 → 跳转登录
6. **修改密码**：旧密码校验 → 修改成功 → 强制重新登录
7. **用户管理**：管理员创建/禁用/删除用户、重置密码
8. **权限控制**：普通用户无法访问用户管理页和管理接口
9. **登录日志**：每次登录/失败均有记录，管理员可查看

---

## 六、状态跟踪

| 阶段 | 状态 | 备注 |
|------|------|------|
| 1. 数据模型 | ⬜ 待开始 | |
| 2. 配置项 | ⬜ 待开始 | |
| 3. JWT 服务 | ⬜ 待开始 | |
| 4. 用户服务 | ⬜ 待开始 | |
| 5. 认证中间件 | ⬜ 待开始 | |
| 6. 认证 Handler | ⬜ 待开始 | |
| 7. 路由改造 | ⬜ 待开始 | |
| 8. 前端 API 层 | ⬜ 待开始 | |
| 9. Auth Store | ⬜ 待开始 | |
| 10. 登录页面 | ⬜ 待开始 | |
| 11. 路由守卫 | ⬜ 待开始 | |
| 12. 全局布局改造 | ⬜ 待开始 | |
| 13. 用户管理页 | ⬜ 待开始 | |
| 14. 修改密码对话框 | ⬜ 待开始 | |
