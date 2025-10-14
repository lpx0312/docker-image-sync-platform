#!/bin/bash

echo "================================"
echo "构建 Docker 镜像同步平台 (完整版)"
echo "================================"

echo
echo "[1/4] 清理旧的构建缓存..."
docker builder prune -f

echo
echo "[2/4] 构建 Docker 镜像..."
docker build -f Dockerfile-all -t docker-image-sync-platform:all ../../

if [ $? -ne 0 ]; then
    echo
    echo "❌ 构建失败！"
    exit 1
fi

echo
echo "[3/4] 查看镜像信息..."
docker images docker-image-sync-platform:all

echo
echo "[4/4] 构建完成！"
echo
echo "🎉 镜像构建成功！"
echo
echo "使用以下命令启动服务："
echo "    只启动 app-all 服务(不部署Mysql)"
echo "        docker-compose -f docker-compose-all.yml up -d"
echo "    启动 app-all 服务并部署Mysql"
echo "        docker-compose -f docker-compose-all-mysql.yml  up -d"
echo 
echo "或者直接运行镜像："
echo "  docker run -d -p 80:80 -p 8080:8080 --name docker-sync-all docker-image-sync-platform:all"
echo
echo "访问地址："
echo "  前端界面: http://localhost"
echo "  后端API:  http://localhost:8080/api"
echo "  健康检查: http://localhost/health"
echo