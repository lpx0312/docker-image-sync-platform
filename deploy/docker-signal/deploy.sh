#!/bin/bash

# ============================================================================
# Docker镜像同步平台 - 一键部署脚本
# ============================================================================
#
# 功能说明：
# 1. 自动检查运行环境和依赖
# 2. 构建前端静态资源
# 3. 使用Docker Compose编排和启动所有服务
# 4. 验证服务启动状态
#
# 服务组件：
# - MySQL 8.0: 数据库服务
# - Go应用: 后端API服务
# - Nginx: 前端Web服务和反向代理
#
# 使用方法：
# 1. 确保已安装Docker和Docker Compose
# 2. 配置.env环境变量文件
# 3. 运行: chmod +x deploy.sh && ./deploy.sh
#
# 注意事项：
# - 首次运行需要下载Docker镜像，可能需要较长时间
# - 确保80和8080端口未被占用
# - 生产环境请修改默认密码和配置
#
# 作者: Docker镜像同步平台开发团队
# 版本: v1.0.0
# ============================================================================

# 启用严格模式：任何命令失败都会导致脚本退出
set -e

echo "🚀 开始部署Docker镜像同步平台..."

# ============================================================================
# 第一步：环境检查
# ============================================================================

echo "📋 检查运行环境..."

# 检查Docker是否安装
if ! command -v docker &> /dev/null; then
    echo "❌ 错误: Docker未安装"
    echo "请访问 https://docs.docker.com/get-docker/ 安装Docker"
    exit 1
fi

# 检查Docker Compose是否安装
if ! command -v docker-compose &> /dev/null; then
    echo "❌ 错误: Docker Compose未安装"
    echo "请访问 https://docs.docker.com/compose/install/ 安装Docker Compose"
    exit 1
fi

echo "✅ Docker和Docker Compose已安装"

# ============================================================================
# 第二步：配置文件检查
# ============================================================================

echo "📝 检查配置文件..."

# 检查环境变量文件是否存在
if [ ! -f .env ]; then
    if [ -f .env.example ]; then
        echo "📋 复制环境变量示例文件..."
        cp .env.example .env
        echo ""
        echo "⚠️  请编辑 .env 文件配置您的环境变量："
        echo "   - Git仓库配置（Gitee/GitHub用户名和密码/Token）"
        echo "   - 阿里云ACR配置（注册表地址、命名空间、用户名、密码）"
        echo "   - 数据库配置（可选，使用默认值）"
        echo ""
        echo "配置完成后重新运行此脚本"
        exit 1
    else
        echo "❌ 错误: 未找到环境变量文件 .env 或 .env.example"
        echo "请确保项目文件完整"
        exit 1
    fi
fi

echo "✅ 环境变量文件检查完成"

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
# 第四步：前端构建
# ============================================================================

echo "🔨 构建前端项目..."

# 进入前端目录
cd web

# 检查并安装前端依赖
if [ ! -d node_modules ]; then
    echo "📦 安装前端依赖..."
    npm install
    echo "   ✅ 前端依赖安装完成"
fi

# 构建前端生产版本
echo "🏗️  构建前端生产版本..."
npm run build
echo "   ✅ 前端构建完成"

# 返回项目根目录
cd ..

# ============================================================================
# 第五步：服务部署
# ============================================================================

echo "🛑 停止现有服务..."
# 停止并删除现有的容器、网络和卷
# 使用 --remove-orphans 清理孤立容器
docker-compose down --remove-orphans
echo "   ✅ 现有服务已停止"

echo "🏗️  构建并启动服务..."
# 使用 --build 强制重新构建镜像
# 使用 -d 后台运行服务
# 使用 --force-recreate 强制重新创建容器
docker-compose up --build --force-recreate -d
echo "   ✅ 服务构建和启动命令已执行"

# ============================================================================
# 第六步：服务验证
# ============================================================================

echo "⏳ 等待服务启动..."
# 等待足够时间让所有服务完全启动
# MySQL初始化、Go应用连接数据库、Nginx启动都需要时间
sleep 30
echo "   ✅ 等待完成"

echo "🔍 检查服务状态..."

# 检查所有服务是否正常运行
if docker-compose ps | grep -q "Up"; then
    echo ""
    echo "🎉 ============================================"
    echo "🎉 Docker镜像同步平台部署成功！"
    echo "🎉 ============================================"
    echo ""
    echo "📱 访问地址："
    echo "   🌐 前端界面: http://localhost"
    echo "   🔌 后端API: http://localhost:8080/api/v1"
    echo "   ❤️  健康检查: http://localhost:8080/api/v1/health"
    echo ""
    echo "🔧 管理命令："
    echo "   📋 查看服务状态: docker-compose ps"
    echo "   📄 查看实时日志: docker-compose logs -f"
    echo "   📄 查看特定服务日志: docker-compose logs -f [app|mysql|frontend]"
    echo "   🛑 停止所有服务: docker-compose down"
    echo "   🔄 重启服务: docker-compose restart"
    echo ""
    echo "📚 更多信息："
    echo "   📖 项目文档: README.md"
    echo "   ⚙️  配置说明: config.yaml"
    echo "   🐛 问题排查: docker-compose logs"
    echo ""
    echo "🚀 开始使用Docker镜像同步平台吧！"
else
    echo ""
    echo "❌ ============================================"
    echo "❌ 服务启动失败！"
    echo "❌ ============================================"
    echo ""
    echo "🔍 故障排查步骤："
    echo "   1. 查看详细日志: docker-compose logs"
    echo "   2. 检查端口占用: netstat -tlnp | grep -E '(80|8080|3306)'"
    echo "   3. 检查Docker状态: docker ps -a"
    echo "   4. 检查磁盘空间: df -h"
    echo "   5. 检查内存使用: free -h"
    echo ""
    echo "📄 详细错误日志："
    docker-compose logs
    echo ""
    echo "💡 常见问题解决："
    echo "   - 端口被占用: 修改 .env 文件中的端口配置"
    echo "   - 权限问题: 使用 sudo 运行脚本"
    echo "   - 网络问题: 检查防火墙和网络连接"
    echo "   - 资源不足: 释放磁盘空间或增加内存"
    exit 1
fi