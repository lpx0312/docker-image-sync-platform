#!/bin/bash

# Docker镜像同步平台部署脚本

set -e

echo "开始部署Docker镜像同步平台..."

# 检查Docker和Docker Compose是否安装
if ! command -v docker &> /dev/null; then
    echo "错误: Docker未安装，请先安装Docker"
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    echo "错误: Docker Compose未安装，请先安装Docker Compose"
    exit 1
fi

# 检查环境变量文件
if [ ! -f .env ]; then
    if [ -f .env.example ]; then
        echo "复制环境变量示例文件..."
        cp .env.example .env
        echo "请编辑 .env 文件配置您的环境变量"
        echo "配置完成后重新运行此脚本"
        exit 1
    else
        echo "错误: 未找到环境变量文件"
        exit 1
    fi
fi

# 创建必要的目录
echo "创建必要的目录..."
mkdir -p logs
mkdir -p git_repo

# 构建前端
echo "构建前端..."
cd web
if [ ! -d node_modules ]; then
    echo "安装前端依赖..."
    npm install
fi
echo "构建前端项目..."
npm run build
cd ..

# 停止现有服务
echo "停止现有服务..."
docker-compose down

# 构建并启动服务
echo "构建并启动服务..."
docker-compose up --build -d

# 等待服务启动
echo "等待服务启动..."
sleep 30

# 检查服务状态
echo "检查服务状态..."
if docker-compose ps | grep -q "Up"; then
    echo "✅ 服务启动成功！"
    echo ""
    echo "访问地址:"
    echo "  前端: http://localhost"
    echo "  API: http://localhost:8080/api/v1"
    echo "  健康检查: http://localhost:8080/api/v1/health"
    echo ""
    echo "查看日志:"
    echo "  docker-compose logs -f"
    echo ""
    echo "停止服务:"
    echo "  docker-compose down"
else
    echo "❌ 服务启动失败，请检查日志:"
    docker-compose logs
    exit 1
fi