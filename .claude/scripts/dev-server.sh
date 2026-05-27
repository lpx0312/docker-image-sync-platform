#!/bin/bash
# 开发环境管理脚本
# 用法: ./dev-server.sh [start|stop|restart|status]

PROJECT_DIR="/home/lipanxiang/code-workspace/docker-image-sync-platform"
BACKEND_PORT=8080
FRONTEND_PORT=3000
BACKEND_LOG="/tmp/app.log"
FRONTEND_LOG="/tmp/frontend.log"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查端口占用
check_port() {
    local port=$1
    if command -v lsof &> /dev/null; then
        lsof -ti :$port 2>/dev/null
    elif command -v fuser &> /dev/null; then
        fuser $port/tcp 2>/dev/null | tr -s ' '
    else
        ss -tlnp | grep ":$port " | awk '{print $NF}' | grep -oP '\d+' | head -1
    fi
}

# 杀掉端口占用的进程
kill_port() {
    local port=$1
    local pids=$(check_port $port)
    if [ -n "$pids" ]; then
        echo -e "${YELLOW}停止端口 $port 上的进程: $pids${NC}"
        # 尝试普通 kill，如果需要 sudo
        for pid in $pids; do
            kill $pid 2>/dev/null || sudo kill $pid 2>/dev/null
        done
        sleep 1
        # 如果还在运行，强制杀掉
        pids=$(check_port $port)
        if [ -n "$pids" ]; then
            for pid in $pids; do
                kill -9 $pid 2>/dev/null || sudo kill -9 $pid 2>/dev/null
            done
        fi
    fi
}

# 停止所有服务
stop_all() {
    echo -e "${YELLOW}停止开发服务器...${NC}"
    kill_port $BACKEND_PORT
    kill_port $FRONTEND_PORT
    # 杀掉 make dev 进程
    pkill -f "make dev" 2>/dev/null || sudo pkill -f "make dev" 2>/dev/null
    pkill -f "npm run dev" 2>/dev/null || sudo pkill -f "npm run dev" 2>/dev/null
    sleep 1
    echo -e "${GREEN}所有服务已停止${NC}"
}

# 启动后端
start_backend() {
    echo -e "${YELLOW}启动后端服务 (端口 $BACKEND_PORT)...${NC}"
    cd "$PROJECT_DIR"
    nohup make dev > "$BACKEND_LOG" 2>&1 &
    sleep 3
    if curl -s http://localhost:$BACKEND_PORT/api/v1/health > /dev/null 2>&1; then
        echo -e "${GREEN}后端服务启动成功${NC}"
    else
        echo -e "${RED}后端服务启动失败，请查看日志: $BACKEND_LOG${NC}"
        tail -20 "$BACKEND_LOG"
        return 1
    fi
}

# 启动前端
start_frontend() {
    echo -e "${YELLOW}启动前端服务 (端口 $FRONTEND_PORT)...${NC}"
    cd "$PROJECT_DIR/web"
    nohup npm run dev > "$FRONTEND_LOG" 2>&1 &
    sleep 5
    if curl -s http://localhost:$FRONTEND_PORT > /dev/null 2>&1; then
        echo -e "${GREEN}前端服务启动成功${NC}"
    else
        echo -e "${RED}前端服务启动失败，请查看日志: $FRONTEND_LOG${NC}"
        tail -20 "$FRONTEND_LOG"
        return 1
    fi
}

# 显示状态
show_status() {
    echo -e "\n${GREEN}=== 开发环境状态 ===${NC}"

    # 后端状态
    if curl -s http://localhost:$BACKEND_PORT/api/v1/health > /dev/null 2>&1; then
        echo -e "${GREEN}✓ 后端: 运行中 (http://localhost:$BACKEND_PORT)${NC}"
    else
        echo -e "${RED}✗ 后端: 未运行${NC}"
    fi

    # 前端状态
    if curl -s http://localhost:$FRONTEND_PORT > /dev/null 2>&1; then
        echo -e "${GREEN}✓ 前端: 运行中 (http://localhost:$FRONTEND_PORT)${NC}"
    else
        echo -e "${RED}✗ 前端: 未运行${NC}"
    fi

    # 端口监听情况
    echo -e "\n${YELLOW}端口监听:${NC}"
    sudo netstat -tlnp 2>/dev/null | grep -E ":$BACKEND_PORT|:$FRONTEND_PORT" || ss -tlnp | grep -E ":$BACKEND_PORT|:$FRONTEND_PORT"
    echo ""
}

# 启动所有服务
start_all() {
    start_backend
    start_frontend
    show_status
}

# 重启所有服务
restart_all() {
    stop_all
    start_all
}

# 主逻辑
case "${1:-restart}" in
    start)
        start_all
        ;;
    stop)
        stop_all
        ;;
    restart)
        restart_all
        ;;
    status)
        show_status
        ;;
    *)
        echo "用法: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
