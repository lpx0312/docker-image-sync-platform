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
# MySQL就绪检查函数
# ============================================================================
check_mysql_ready() {
    local max_attempts=30
    local attempt=0
    local mysql_host="127.0.0.1"
    local mysql_port="3306"

    echo "   🔄 检查MySQL服务就绪状态..."

    while [ $attempt -lt $max_attempts ]; do
        attempt=$((attempt + 1))

        # 检查端口3306是否可用（最简单通用的方法）
        if command -v docker >/dev/null 2>&1; then
            # 如果容器存在，检查容器状态
            if docker ps | grep -q "docker-sync-mysql-dev"; then
                # 容器正在运行，等待一段时间让MySQL启动
                sleep 3
                echo "   ✅ MySQL容器运行中"
                return 0
            fi
        fi

        # 检查端口是否开放（使用curl尝试连接）
        if command -v curl >/dev/null 2>&1; then
            # 简单的TCP连接测试
            if timeout 3 bash -c "</dev/tcp/$mysql_host/$mysql_port" 2>/dev/null; then
                echo "   ✅ MySQL端口可访问"
                return 0
            fi
        fi

        # 使用PowerShell测试连接（Windows环境）
        if command -v powershell >/dev/null 2>&1; then
            if powershell -Command "
                try {
                    \$tcpClient = New-Object System.Net.Sockets.TcpClient
                    \$tcpClient.Connect('$mysql_host', $mysql_port)
                    \$tcpClient.Close()
                    exit 0
                } catch {
                    exit 1
                }
            " 2>/dev/null; then
                echo "   ✅ MySQL端口可访问"
                return 0
            fi
        fi

        # 如果是Windows环境，给Docker容器更多启动时间
        if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]]; then
            echo "   ⏳ 等待MySQL容器启动... (尝试 $attempt/$max_attempts)"
            sleep 3
        else
            echo "   ⏳ 等待MySQL启动... (尝试 $attempt/$max_attempts)"
            sleep 2
        fi
    done

    echo "   ⚠️  MySQL服务检查超时，但继续执行..."
    return 1
}

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
    echo "   ✅ Docker已安装，将使用Docker启动MySQL（如需要）"
else
    echo "   ⚠️  Docker未安装，将使用本地MySQL数据库"
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
if [ ! -d logs ]; then
    mkdir -p logs
fi

# ============================================================================
# 第四步：数据库启动
# ============================================================================

echo "🗄️  启动MySQL数据库..."

# 如果Docker可用，使用Docker启动MySQL
if command -v docker &> /dev/null; then
    # 检查MySQL容器是否存在
    if docker ps | grep -q "docker-sync-mysql-dev"; then
        echo "   ✅ MySQL容器已在运行"
        # 检查MySQL服务是否就绪
        check_mysql_ready
    else
        echo "   🐳 启动MySQL容器..."
        docker run -d \
            --name docker-sync-mysql-dev \
            -p 3306:3306 \
            -e MYSQL_ROOT_PASSWORD=123456 \
            -e MYSQL_DATABASE=docker_sync \
            -e MYSQL_USER=docker_sync \
            -e MYSQL_PASSWORD=123456 \
            mysql:8.0

        # 检查MySQL服务是否就绪
        check_mysql_ready
    fi
else
    echo "   ⚠️  Docker未安装，请确保本地MySQL服务已启动，数据库名为: docker_sync"
    # 检查本地MySQL是否就绪
    check_mysql_ready
fi

# ============================================================================
# 第五步：后端依赖安装
# ============================================================================

echo "📦 安装Go依赖..."
go env -w GOPROXY=https://goproxy.cn,https://goproxy.io,https://mirrors.aliyun.com/goproxy/,direct
# 清理旧缓存
# go clean -modcache
# 强制校验依赖
# go mod verify
# 下载依赖
# go mod download
# 强制刷新依赖
go mod tidy -v
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
npm install --registry=--registry=https://registry.npmmirror.com/

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