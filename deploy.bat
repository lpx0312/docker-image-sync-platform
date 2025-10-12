@echo off
setlocal enabledelayedexpansion

REM ============================================================================
REM Docker镜像同步平台 - Windows部署脚本
REM ============================================================================
REM 
REM 功能描述:
REM   自动化部署Docker镜像同步平台的Windows批处理脚本
REM   支持完整的生产环境部署流程，包括环境检查、前端构建、服务启动等
REM 
REM 主要功能:
REM   1. 环境依赖检查 (Docker, Docker Compose)
REM   2. 配置文件验证 (.env环境变量)
REM   3. 目录结构准备 (logs, git_repo)
REM   4. 前端项目构建 (Vue.js应用)
REM   5. Docker服务部署 (后端API + 前端 + Nginx)
REM   6. 服务状态验证和访问信息提供
REM 
REM 服务组件:
REM   - 后端API服务 (Go + Gin框架, 端口8080)
REM   - 前端Web应用 (Vue.js + Element Plus)
REM   - Nginx反向代理 (端口80)
REM   - MySQL数据库 (端口3306)
REM 
REM 使用方法:
REM   1. 确保已安装Docker Desktop
REM   2. 配置.env环境变量文件
REM   3. 双击运行deploy.bat或在命令行执行
REM 
REM 注意事项:
REM   - 需要管理员权限运行Docker命令
REM   - 确保端口80、8080、3306未被占用
REM   - 首次运行需要下载Docker镜像，耗时较长
REM   - 生产环境建议修改默认密码和配置
REM 
REM 作者: Docker镜像同步平台开发团队
REM 版本: v1.0.0
REM 更新: 2024-12-19
REM ============================================================================

echo 开始部署Docker镜像同步平台...

REM ============================================================================
REM 第一步: 环境依赖检查
REM ============================================================================

REM 检查Docker是否安装
REM Docker是容器化部署的核心依赖，用于运行应用服务容器
echo 检查Docker安装状态...
docker --version >nul 2>&1
if errorlevel 1 (
    echo ❌ 错误: Docker未安装，请先安装Docker Desktop
    echo 下载地址: https://www.docker.com/products/docker-desktop
    pause
    exit /b 1
)
echo ✅ Docker已安装

REM 检查Docker Compose是否安装
REM Docker Compose用于管理多容器应用的编排和部署
echo 检查Docker Compose安装状态...
docker-compose --version >nul 2>&1
if errorlevel 1 (
    echo ❌ 错误: Docker Compose未安装，请先安装Docker Compose
    echo 通常Docker Desktop会自带Docker Compose
    pause
    exit /b 1
)
echo ✅ Docker Compose已安装

REM ============================================================================
REM 第二步: 配置文件验证
REM ============================================================================

REM 检查环境变量文件
REM .env文件包含数据库连接、JWT密钥等敏感配置信息
echo 检查环境变量配置文件...
if not exist .env (
    if exist .env.example (
        echo 📋 复制环境变量示例文件...
        copy .env.example .env
        echo ⚠️  请编辑 .env 文件配置您的环境变量
        echo    主要配置项:
        echo      - 数据库连接信息 (MYSQL_*)
        echo      - JWT密钥 (JWT_SECRET)
        echo      - 服务端口配置
        echo 配置完成后重新运行此脚本
        pause
        exit /b 1
    ) else (
        echo ❌ 错误: 未找到环境变量文件 (.env 或 .env.example)
        echo 请确保项目根目录包含环境变量配置文件
        pause
        exit /b 1
    )
)
echo ✅ 环境变量文件已存在

REM ============================================================================
REM 第三步: 目录结构准备
REM ============================================================================

REM 创建必要的目录
REM logs: 存储应用运行日志文件
REM git_repo: 存储Git仓库克隆和同步数据
echo 📁 创建必要的目录...
if not exist logs (
    mkdir logs
    echo   ✅ 创建logs目录 (应用日志存储)
)
if not exist git_repo (
    mkdir git_repo
    echo   ✅ 创建git_repo目录 (Git仓库数据)
)

REM ============================================================================
REM 第四步: 前端项目构建
REM ============================================================================

REM 构建前端Vue.js应用
REM 将TypeScript/Vue源码编译为生产环境的静态文件
echo 🔨 构建前端项目...
cd web

REM 检查并安装前端依赖
if not exist node_modules (
    echo   📦 安装前端依赖包...
    echo   这可能需要几分钟时间，请耐心等待...
    npm install
    if errorlevel 1 (
        echo   ❌ 错误: 前端依赖安装失败
        echo   请检查网络连接和npm配置
        cd ..
        pause
        exit /b 1
    )
    echo   ✅ 前端依赖安装完成
)

REM 执行前端构建
echo   🏗️  编译前端项目 (Vue.js + TypeScript)...
npm run build
if errorlevel 1 (
    echo   ❌ 错误: 前端构建失败
    echo   请检查前端代码语法和构建配置
    cd ..
    pause
    exit /b 1
)
echo   ✅ 前端构建完成，生成dist目录
cd ..

REM ============================================================================
REM 第五步: Docker服务部署
REM ============================================================================

REM 停止现有服务
REM 清理之前运行的容器，避免端口冲突和资源占用
echo 🛑 停止现有服务...
docker-compose down --remove-orphans
echo   ✅ 现有服务已停止

REM 构建并启动服务
REM 使用docker-compose编排多个服务容器
echo 🚀 构建并启动服务...
echo   📦 构建Docker镜像 (后端Go应用)...
echo   🔄 启动服务容器 (后端 + 前端 + 数据库)...
docker-compose up --build --force-recreate -d
if errorlevel 1 (
    echo   ❌ 错误: 服务启动失败
    echo   常见问题排查:
    echo     1. 检查端口是否被占用 (80, 8080, 3306)
    echo     2. 检查Docker Desktop是否正常运行
    echo     3. 检查.env配置文件是否正确
    echo   查看详细错误: docker-compose logs
    pause
    exit /b 1
)

REM ============================================================================
REM 第六步: 服务状态验证
REM ============================================================================

REM 等待服务启动
echo ⏳ 等待服务启动完成...
echo   数据库初始化和应用启动需要一些时间...
timeout /t 30 /nobreak >nul

REM 检查服务状态
echo 🔍 检查服务运行状态...
docker-compose ps | findstr "Up" >nul
if errorlevel 1 (
    echo ❌ 服务启动失败，请检查日志:
    echo.
    echo 🔧 故障排查步骤:
    echo   1. 查看服务状态: docker-compose ps
    echo   2. 查看详细日志: docker-compose logs
    echo   3. 查看特定服务: docker-compose logs [service_name]
    echo   4. 重新部署: docker-compose down && docker-compose up --build -d
    echo.
    docker-compose logs
    pause
    exit /b 1
) else (
    echo ✅ 🎉 部署成功！服务已启动并运行正常
    echo.
    echo 🌐 访问地址:
    echo   📱 前端界面: http://localhost
    echo   🔌 后端API: http://localhost:8080/api/v1
    echo   💚 健康检查: http://localhost:8080/api/v1/health
    echo   📊 API文档: http://localhost:8080/api/v1/docs (如果启用)
    echo.
    echo 🛠️  管理命令:
    echo   📋 查看服务状态: docker-compose ps
    echo   📝 查看实时日志: docker-compose logs -f
    echo   🔄 重启服务: docker-compose restart
    echo   🛑 停止服务: docker-compose down
    echo   🗑️  清理数据: docker-compose down -v
    echo.
    echo 📚 使用提示:
    echo   - 首次使用需要配置Docker Hub账号信息
    echo   - 支持批量镜像同步和定时任务
    echo   - 日志文件保存在 ./logs 目录
    echo   - 数据持久化存储在Docker卷中
    echo.
    pause
)