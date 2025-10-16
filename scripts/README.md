# 配置迁移脚本使用说明

## 概述

本目录包含了将 `config.yaml` 配置文件迁移到数据库的脚本工具。迁移完成后，系统将从数据库读取配置，支持通过Web界面动态修改配置。

## 文件说明

- `migrate_config.go` - 配置迁移的核心逻辑
- `run_migration.sh` - 迁移执行脚本（推荐使用）
- `README.md` - 本说明文档

## 迁移内容

脚本将迁移以下配置项到数据库：

### Git配置
- `git_repository_type` - Git仓库类型（gitee/github）
- `gitee_repo_url` - Gitee仓库URL
- `gitee_username` - Gitee用户名
- `gitee_password` - Gitee密码（加密存储）
- `gitee_email` - Gitee邮箱
- `github_repo_url` - GitHub仓库URL
- `github_username` - GitHub用户名
- `github_token` - GitHub访问令牌（加密存储）
- `gitee_local_path` - Gitee本地仓库路径
- `github_local_path` - GitHub本地仓库路径

### 阿里云配置
- `aliyun_registry` - 阿里云镜像仓库地址
- `aliyun_namespace` - 阿里云命名空间
- `aliyun_username` - 阿里云用户名
- `aliyun_password` - 阿里云密码（加密存储）

### 系统配置
- `server_port` - 服务器端口
- `server_mode` - 运行模式
- `server_host` - 服务器地址
- `sync_timeout_minutes` - 同步超时时间
- `sync_max_concurrent_jobs` - 最大并发任务数
- `sync_max_retry_count` - 最大重试次数
- `sync_retry_interval_minutes` - 重试间隔
- `github_actions_workflow_file` - GitHub Actions工作流文件
- `github_actions_api_timeout_seconds` - API超时时间
- `github_actions_status_check_interval_seconds` - 状态检查间隔

## 使用方法

### 方法一：使用执行脚本（推荐）

```bash
# 1. 确保在项目根目录
cd /path/to/docker-image-sync-platform

# 2. 执行迁移脚本
./scripts/run_migration.sh
```

### 方法二：手动执行

```bash
# 1. 确保在项目根目录
cd /path/to/docker-image-sync-platform

# 2. 编译迁移程序
go build -o tmp/migrate_config scripts/migrate_config.go

# 3. 执行迁移
./tmp/migrate_config
```

## 前置条件

1. **Go环境** - 确保已安装Go 1.19+
2. **数据库连接** - 确保数据库服务正常运行且连接配置正确
3. **配置文件** - 确保 `config.yaml` 文件存在且格式正确
4. **依赖包** - 确保已下载所有Go模块依赖

## 安全说明

- **敏感信息加密**: 密码、令牌等敏感信息会自动加密存储
- **备份建议**: 迁移前建议备份数据库
- **权限检查**: 确保数据库用户有足够的权限创建和修改表

## 迁移行为

- **幂等操作**: 可以重复执行，已存在的配置会被更新
- **空值跳过**: 空值配置项会被跳过，不会写入数据库
- **分组管理**: 配置按功能分组（git、aliyun、server等）
- **顺序保持**: 配置项会保持合理的显示顺序

## 故障排除

### 常见错误

1. **数据库连接失败**
   ```
   连接数据库失败: dial tcp xxx:3306: connect: connection refused
   ```
   - 检查数据库服务是否启动
   - 验证 `config.yaml` 中的数据库配置

2. **配置文件不存在**
   ```
   配置文件不存在: /path/to/config.yaml
   ```
   - 确保在正确的项目根目录执行
   - 检查 `config.yaml` 文件是否存在

3. **编译失败**
   ```
   go build: cannot find module
   ```
   - 执行 `go mod download` 下载依赖
   - 检查网络连接和Go代理设置

### 调试模式

如需查看详细的迁移过程，可以直接运行Go程序：

```bash
go run scripts/migrate_config.go
```

## 迁移后验证

迁移完成后，可以通过以下方式验证：

1. **数据库检查**
   ```sql
   SELECT * FROM system_configs ORDER BY config_group, display_order;
   ```

2. **Web界面** - 访问系统配置页面查看配置项

3. **API测试** - 调用配置相关API接口验证

## 注意事项

- 迁移不会删除原有的 `config.yaml` 文件
- 迁移后系统优先从数据库读取配置
- 如需回退，可以删除数据库中的配置记录
- 建议在测试环境先验证迁移效果