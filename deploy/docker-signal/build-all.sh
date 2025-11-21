#!/bin/bash

echo "================================"
echo "构建 Docker 镜像同步平台 (分离版)"
echo "================================"

echo
echo "[1/4] 构建 Backend Docker 镜像..."
docker build -f Dockerfile-backend -t docker-image-sync-platform:backend ../../

if [ $? -ne 0 ]; then
    echo
    echo "❌ Backend 镜像构建失败！"
    exit 1
fi

echo
echo "[2/4] 构建 Frontend Docker 镜像..."
docker build -f Dockerfile-frontend -t docker-image-sync-platform:frontend ../../

if [ $? -ne 0 ]; then
    echo
    echo "❌ Frontend 镜像构建失败！"
    exit 1
fi

echo
echo "[3/4] 查看镜像信息..."
echo "Backend 镜像:"
docker images docker-image-sync-platform:backend
echo
echo "Frontend 镜像:"
docker images docker-image-sync-platform:frontend

echo
echo "[4/4] 构建完成！"
echo
echo "🎉 前后端分离镜像构建成功！"
echo
echo "⚠️  部署前配置："
echo "  1. 复制配置文件：cp env-example .env"
echo "  2. 修改 .env 文件中的Git仓库配置"
echo "  3. 设置 GIT_REPOSITORY_TYPE=gitee 或 github"
echo "  4. 配置对应的Git认证信息"
echo "  5. 根据需要修改 nginx.conf 配置"
echo
echo "🚀 使用以下命令启动服务："
echo "    只启动前后端服务(不部署Mysql)"
echo "        docker-compose -f docker-compose.yml up --build --force-recreate -d"
echo
echo "    启动前后端服务并部署Mysql"
echo "        docker-compose -f docker-compose-mysql.yml up --build --force-recreate -d"
echo
echo "🌐 访问地址："
echo "  前端界面: http://localhost"
echo "  后端API:  http://localhost/api"
echo "  健康检查: http://localhost/health"
echo
echo "📝 重要提示："
echo "  - 分离部署模式，前后端独立容器"
echo "  - Nginx 反向代理配置在 nginx.conf 中"
echo "  - 支持Gitee和GitHub两种Git仓库类型"
echo "  - 默认Git仓库类型通过 GIT_REPOSITORY_TYPE 环境变量配置"
echo "  - 可通过 docker-compose.yml 调整端口映射和资源限制"
echo