#!/bin/bash

# 开发环境启动脚本

set -e

echo "启动开发环境..."

# 检查Go是否安装
if ! command -v go &> /dev/null; then
    echo "错误: Go未安装，请先安装Go"
    exit 1
fi

# 检查Node.js是否安装
if ! command -v node &> /dev/null; then
    echo "错误: Node.js未安装，请先安装Node.js"
    exit 1
fi

# 检查配置文件
if [ ! -f config.yaml ]; then
    echo "错误: 未找到配置文件 config.yaml"
    exit 1
fi

# 创建必要的目录
mkdir -p logs
mkdir -p git_repo

# 启动MySQL（如果使用Docker）
echo "启动MySQL数据库..."
if command -v docker &> /dev/null; then
    docker run -d \
        --name docker-sync-mysql-dev \
        -p 3306:3306 \
        -e MYSQL_ROOT_PASSWORD=root123456 \
        -e MYSQL_DATABASE=docker_sync \
        -e MYSQL_USER=docker_sync \
        -e MYSQL_PASSWORD=sync123456 \
        -v $(pwd)/sql/init.sql:/docker-entrypoint-initdb.d/init.sql \
        mysql:8.0 2>/dev/null || echo "MySQL容器已存在或启动失败"
    
    echo "等待MySQL启动..."
    sleep 10
fi

# 安装Go依赖
echo "安装Go依赖..."
go mod tidy

# 启动后端服务
echo "启动后端服务..."
go run main.go &
BACKEND_PID=$!

# 等待后端启动
sleep 5

# 启动前端开发服务器
echo "启动前端开发服务器..."
cd web

# 安装前端依赖
if [ ! -d node_modules ]; then
    echo "安装前端依赖..."
    npm install
fi

# 启动前端
npm run dev &
FRONTEND_PID=$!

cd ..

echo ""
echo "✅ 开发环境启动成功！"
echo ""
echo "访问地址:"
echo "  前端: http://localhost:3000"
echo "  后端API: http://localhost:8080/api/v1"
echo "  健康检查: http://localhost:8080/api/v1/health"
echo ""
echo "按 Ctrl+C 停止服务"

# 等待中断信号
trap 'echo "正在停止服务..."; kill $BACKEND_PID $FRONTEND_PID 2>/dev/null; exit 0' INT

wait