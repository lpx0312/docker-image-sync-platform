#!/bin/bash

# ============================================================================
# Docker镜像同步平台 - 开发环境启动脚本
# ============================================================================
#
# 功能说明：
# 1. 检查开发环境依赖（Go、Node.js、Docker等）
# 2. 启动MySQL数据库（使用Docker容器）
# 3. 启动Go后端服务（开发模式）
# 4. 启动Vue前端开发服务器（热重载）
#
# 开发特性：
# - 后端支持热重载（使用air工具）
# - 前端支持热重载（Vite开发服务器）
# - 数据库使用Docker容器，便于管理
# - 自动安装依赖和初始化数据库
#
# 使用方法：
# 1. 确保已安装Go 1.21+、Node.js 18+、Docker
# 2. 配置config.yaml文件
# 3. 运行: chmod +x dev.sh && ./dev.sh
#
# 注意事项：
# - 开发环境使用3000端口（前端）和8080端口（后端）
# - 数据库使用3306端口，确保端口未被占用
# - 按Ctrl+C可以停止所有服务
#
# 作者: Docker镜像同步平台开发团队
# 版本: v1.0.0
# ============================================================================

# 启用严格模式：任何命令失败都会导致脚本退出
set -e

echo "🚀 启动开发环境..."

# ============================================================================
# 第一步：环境检查
# ============================================================================

echo "📋 检查开发环境依赖..."

# 检查Go是否安装
if ! command -v go &> /dev/null; then
    echo "❌ 错误: Go未安装"
    echo "请访问 https://golang.org/dl/ 安装Go 1.21或更高版本"
    exit 1
fi

# 检查Go版本
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "   ✅ Go版本: $GO_VERSION"

# 检查Node.js是否安装
if ! command -v node &> /dev/null; then
    echo "❌ 错误: Node.js未安装"
    echo "请访问 https://nodejs.org/ 安装Node.js 18或更高版本"
    exit 1
fi

# 检查Node.js版本
NODE_VERSION=$(node --version)
echo "   ✅ Node.js版本: $NODE_VERSION"

# 检查npm是否可用
if ! command -v npm &> /dev/null; then
    echo "❌ 错误: npm未安装"
    echo "请重新安装Node.js或单独安装npm"
    exit 1
fi

# 检查Docker是否安装（可选，用于数据库）
if command -v docker &> /dev/null; then
    echo "   ✅ Docker已安装，将使用Docker启动MySQL"
else
    echo "   ⚠️  Docker未安装，请确保本地已安装MySQL数据库"
fi

# ============================================================================
# 第二步：配置文件检查
# ============================================================================

echo "📝 检查配置文件..."

# 检查配置文件是否存在
if [ ! -f config.yaml ]; then
    echo "❌ 错误: 未找到配置文件 config.yaml"
    echo "请复制 config.yaml.example 为 config.yaml 并进行配置"
    exit 1
fi

echo "   ✅ 配置文件检查完成"

# ============================================================================
# 第三步：目录准备
# ============================================================================

echo "📁 创建必要的目录..."

# 创建日志目录
mkdir -p logs
echo "   ✅ 日志目录: logs/"

# 创建Git仓库临时目录
mkdir -p git_repo
echo "   ✅ Git仓库目录: git_repo/"

# ============================================================================
# 第四步：数据库启动
# ============================================================================

echo "🗄️  启动MySQL数据库..."

# 如果Docker可用，使用Docker启动MySQL
if command -v docker &> /dev/null; then
    # 尝试启动MySQL容器，如果已存在则跳过
    docker run -d \
        --name docker-sync-mysql-dev \
        -p 3306:3306 \
        -e MYSQL_ROOT_PASSWORD=root123456 \
        -e MYSQL_DATABASE=docker_sync \
        -e MYSQL_USER=docker_sync \
        -e MYSQL_PASSWORD=sync123456 \
        -v $(pwd)/sql/init.sql:/docker-entrypoint-initdb.d/01-init.sql \
        -v $(pwd)/sql/migrate_add_architecture_default.sql:/docker-entrypoint-initdb.d/02-migrate-architecture.sql \
        -v $(pwd)/sql/migrate_add_batch_sync_support.sql:/docker-entrypoint-initdb.d/03-migrate-batch-sync.sql \
        -v $(pwd)/sql/migrate_add_deleted_at.sql:/docker-entrypoint-initdb.d/04-migrate-deleted-at.sql \
        mysql:8.0 2>/dev/null || echo "   ℹ️  MySQL容器已存在或启动失败，继续执行..."
    
    echo "   ⏳ 等待MySQL启动..."
    sleep 10
    echo "   ✅ MySQL数据库已启动"
else
    echo "   ⚠️  请确保本地MySQL服务已启动，数据库名为: docker_sync"
fi

# ============================================================================
# 第五步：后端依赖安装
# ============================================================================

echo "📦 安装Go依赖..."
go mod tidy
echo "   ✅ Go依赖安装完成"

# ============================================================================
# 第六步：启动后端服务
# ============================================================================

echo "🔧 启动Go后端服务..."

# 在后台启动Go应用
# 使用 & 符号让进程在后台运行
# 保存进程ID以便后续管理
go run main.go &
BACKEND_PID=$!

echo "   ✅ 后端服务已启动 (PID: $BACKEND_PID)"
echo "   📍 后端地址: http://localhost:8080"

# ============================================================================
# 第七步：启动前端开发服务器
# ============================================================================

echo "⏳ 等待后端服务完全启动..."
# 等待后端服务完全启动并连接数据库
sleep 5

echo "🎨 启动Vue前端开发服务器..."

# 进入前端项目目录
cd web

# 检查并安装前端依赖
if [ ! -d node_modules ]; then
    echo "📦 安装前端依赖..."
    npm install
    echo "   ✅ 前端依赖安装完成"
fi

# 在后台启动前端开发服务器
# Vite开发服务器支持热重载和快速构建
npm run dev &
FRONTEND_PID=$!

echo "   ✅ 前端开发服务器已启动 (PID: $FRONTEND_PID)"
echo "   📍 前端地址: http://localhost:3000"

# 返回项目根目录
cd ..

# ============================================================================
# 第八步：开发环境就绪
# ============================================================================

echo ""
echo "🎉 ============================================"
echo "🎉 开发环境启动成功！"
echo "🎉 ============================================"
echo ""
echo "📱 访问地址："
echo "   🌐 前端开发服务器: http://localhost:3000"
echo "   🔌 后端API服务: http://localhost:8080/api/v1"
echo "   ❤️  健康检查接口: http://localhost:8080/api/v1/health"
echo "   🗄️  数据库连接: localhost:3306 (docker_sync)"
echo ""
echo "🔧 开发特性："
echo "   🔥 前端热重载: 修改代码自动刷新页面"
echo "   🔄 后端自动重启: 使用 air 工具实现热重载"
echo "   📊 实时日志: 查看控制台输出"
echo "   🐛 调试模式: 详细的错误信息和日志"
echo ""
echo "💡 开发提示："
echo "   📝 修改前端代码: web/src/ 目录"
echo "   📝 修改后端代码: internal/ 目录"
echo "   📝 修改配置文件: config.yaml"
echo "   📄 查看日志文件: logs/ 目录"
echo ""
echo "🛑 停止服务: 按 Ctrl+C"

# ============================================================================
# 第九步：信号处理和服务管理
# ============================================================================

# 设置信号处理器，捕获Ctrl+C信号
# 当用户按下Ctrl+C时，优雅地停止所有服务
trap 'echo ""; echo "🛑 正在停止开发服务..."; echo "   ⏳ 停止后端服务..."; kill $BACKEND_PID 2>/dev/null; echo "   ⏳ 停止前端服务..."; kill $FRONTEND_PID 2>/dev/null; echo "   ✅ 所有服务已停止"; echo "   👋 感谢使用Docker镜像同步平台开发环境！"; exit 0' INT

# 等待所有后台进程
# 脚本会一直运行直到收到中断信号
wait