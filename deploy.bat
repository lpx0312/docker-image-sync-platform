@echo off
setlocal enabledelayedexpansion

echo 开始部署Docker镜像同步平台...

REM 检查Docker是否安装
docker --version >nul 2>&1
if errorlevel 1 (
    echo 错误: Docker未安装，请先安装Docker Desktop
    pause
    exit /b 1
)

REM 检查Docker Compose是否安装
docker-compose --version >nul 2>&1
if errorlevel 1 (
    echo 错误: Docker Compose未安装，请先安装Docker Compose
    pause
    exit /b 1
)

REM 检查环境变量文件
if not exist .env (
    if exist .env.example (
        echo 复制环境变量示例文件...
        copy .env.example .env
        echo 请编辑 .env 文件配置您的环境变量
        echo 配置完成后重新运行此脚本
        pause
        exit /b 1
    ) else (
        echo 错误: 未找到环境变量文件
        pause
        exit /b 1
    )
)

REM 创建必要的目录
echo 创建必要的目录...
if not exist logs mkdir logs
if not exist git_repo mkdir git_repo

REM 构建前端
echo 构建前端...
cd web
if not exist node_modules (
    echo 安装前端依赖...
    npm install
    if errorlevel 1 (
        echo 错误: 前端依赖安装失败
        cd ..
        pause
        exit /b 1
    )
)
echo 构建前端项目...
npm run build
if errorlevel 1 (
    echo 错误: 前端构建失败
    cd ..
    pause
    exit /b 1
)
cd ..

REM 停止现有服务
echo 停止现有服务...
docker-compose down

REM 构建并启动服务
echo 构建并启动服务...
docker-compose up --build -d
if errorlevel 1 (
    echo 错误: 服务启动失败
    pause
    exit /b 1
)

REM 等待服务启动
echo 等待服务启动...
timeout /t 30 /nobreak >nul

REM 检查服务状态
echo 检查服务状态...
docker-compose ps | findstr "Up" >nul
if errorlevel 1 (
    echo ❌ 服务启动失败，请检查日志:
    docker-compose logs
    pause
    exit /b 1
) else (
    echo ✅ 服务启动成功！
    echo.
    echo 访问地址:
    echo   前端: http://localhost
    echo   API: http://localhost:8080/api/v1
    echo   健康检查: http://localhost:8080/api/v1/health
    echo.
    echo 查看日志:
    echo   docker-compose logs -f
    echo.
    echo 停止服务:
    echo   docker-compose down
    echo.
    pause
)