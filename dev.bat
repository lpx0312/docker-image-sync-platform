@echo off
setlocal enabledelayedexpansion

echo 启动开发环境...

REM 检查Go是否安装
go version >nul 2>&1
if errorlevel 1 (
    echo 错误: Go未安装，请先安装Go
    pause
    exit /b 1
)

REM 检查Node.js是否安装
node --version >nul 2>&1
if errorlevel 1 (
    echo 错误: Node.js未安装，请先安装Node.js
    pause
    exit /b 1
)

REM 检查配置文件
if not exist config.yaml (
    echo 错误: 未找到配置文件 config.yaml
    pause
    exit /b 1
)

REM 创建必要的目录
if not exist logs mkdir logs
if not exist git_repo mkdir git_repo

REM 启动MySQL（如果使用Docker）
echo 启动MySQL数据库...
docker --version >nul 2>&1
if not errorlevel 1 (
    docker run -d --name docker-sync-mysql-dev -p 3306:3306 -e MYSQL_ROOT_PASSWORD=root123456 -e MYSQL_DATABASE=docker_sync -e MYSQL_USER=docker_sync -e MYSQL_PASSWORD=sync123456 -v %cd%/sql/init.sql:/docker-entrypoint-initdb.d/init.sql mysql:8.0 >nul 2>&1
    echo 等待MySQL启动...
    timeout /t 10 /nobreak >nul
)

REM 安装Go依赖
echo 安装Go依赖...
go mod tidy

REM 启动后端服务
echo 启动后端服务...
start "Backend Server" cmd /c "go run main.go"

REM 等待后端启动
timeout /t 5 /nobreak >nul

REM 启动前端开发服务器
echo 启动前端开发服务器...
cd web

REM 安装前端依赖
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

REM 启动前端
start "Frontend Server" cmd /c "npm run dev"

cd ..

echo.
echo ✅ 开发环境启动成功！
echo.
echo 访问地址:
echo   前端: http://localhost:3000
echo   后端API: http://localhost:8080/api/v1
echo   健康检查: http://localhost:8080/api/v1/health
echo.
echo 按任意键退出...
pause >nul