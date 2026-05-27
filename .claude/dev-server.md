# 开发环境管理

## 快速命令

```bash
# 重启开发环境
.claude/scripts/dev-server.sh restart

# 启动开发环境
.claude/scripts/dev-server.sh start

# 停止开发环境
.claude/scripts/dev-server.sh stop

# 查看状态
.claude/scripts/dev-server.sh status
```

## 服务地址

- **前端**: http://localhost:3000 (监听 0.0.0.0，支持远程访问)
- **后端**: http://localhost:8080
- **Swagger UI**: http://localhost:8080/api/v1/docs.html
- **健康检查**: http://localhost:8080/api/v1/health

## 日志文件

- **后端日志**: /tmp/app.log
- **前端日志**: /tmp/frontend.log

```bash
# 查看后端日志
tail -f /tmp/app.log

# 查看前端日志
tail -f /tmp/frontend.log
```

## 手动重启

如果脚本不可用，可以手动操作：

```bash
# 1. 停止服务
lsof -ti :8080 | xargs -r kill -9
lsof -ti :3000 | xargs -r kill -9

# 2. 启动后端
cd /home/lipanxiang/code-workspace/docker-image-sync-platform
nohup make dev > /tmp/app.log 2>&1 &

# 3. 启动前端
cd web
nohup npm run dev > /tmp/frontend.log 2>&1 &
```

## 注意事项

- 前端配置了 `host: '0.0.0.0'`，支持从其他机器访问
- 后端使用 `make dev` 启动，会自动处理编译和运行
- 使用 `nohup` 确保后台运行，关闭终端不会停止服务
