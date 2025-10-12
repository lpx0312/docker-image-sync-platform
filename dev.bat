@echo off
setlocal enabledelayedexpansion

REM ============================================================================
REM Docker镜像同步平台 - Windows开发环境启动脚本
REM ============================================================================
REM 
REM 功能描述:
REM   自动化启动Docker镜像同步平台开发环境的Windows批处理脚本
REM   支持本地开发调试，包括热重载、实时编译等开发特性
REM 
REM 主要功能:
REM   1. 开发环境检查 (Go, Node.js, npm, Docker)
REM   2. 配置文件验证 (config.yaml)
REM   3. 目录结构准备 (logs, git_repo)
REM   4. MySQL数据库启动 (Docker容器)
REM   5. Go后端服务启动 (热重载支持)
REM   6. Vue前端开发服务器启动 (HMR热更新)
REM 
REM 开发特性:
REM   - 后端: Go原生运行，支持代码修改后手动重启
REM   - 前端: Vite开发服务器，支持热模块替换(HMR)
REM   - 数据库: Docker MySQL容器，数据持久化
REM   - 日志: 实时控制台输出，便于调试
REM 
REM 使用方法:
REM   1. 确保已安装Go、Node.js、Docker
REM   2. 配置config.yaml开发环境参数
REM   3. 双击运行dev.bat或在命令行执行
REM 
REM 注意事项:
REM   - 开发环境使用不同的端口配置
REM   - 前端开发服务器默认端口3000
REM   - 后端API服务默认端口8080
REM   - MySQL数据库默认端口3306
REM   - 使用Ctrl+C停止服务
REM 
REM 作者: Docker镜像同步平台开发团队
REM 版本: v1.0.0
REM 更新: 2024-12-19
REM ============================================================================

echo 启动开发环境...

REM ============================================================================
REM 第一步: 开发环境依赖检查
REM ============================================================================

REM 检查Go是否安装
REM Go用于运行后端API服务，支持热重载开发
echo 检查Go开发环境...
go version >nul 2>&1
if errorlevel 1 (
    echo ❌ 错误: Go未安装，请先安装Go
    echo 下载地址: https://golang.org/dl/
    echo 建议版本: Go 1.19+
    pause
    exit /b 1
)
echo ✅ Go环境已安装

REM 检查Node.js是否安装
REM Node.js用于运行前端开发服务器和构建工具
echo 检查Node.js开发环境...
node --version >nul 2>&1
if errorlevel 1 (
    echo ❌ 错误: Node.js未安装，请先安装Node.js
    echo 下载地址: https://nodejs.org/
    echo 建议版本: Node.js 16+
    pause
    exit /b 1
)
echo ✅ Node.js环境已安装

REM 检查npm是否可用
echo 检查npm包管理器...
npm --version >nul 2>&1
if errorlevel 1 (
    echo ❌ 错误: npm不可用，请检查Node.js安装
    pause
    exit /b 1
)
echo ✅ npm包管理器可用

REM ============================================================================
REM 第二步: 配置文件验证
REM ============================================================================

REM 检查配置文件
REM config.yaml包含开发环境的数据库连接、服务端口等配置
echo 检查开发环境配置文件...
if not exist config.yaml (
    echo ❌ 错误: 未找到配置文件 config.yaml
    echo 请确保项目根目录包含开发环境配置文件
    echo 可以参考config.yaml.example创建配置文件
    pause
    exit /b 1
)
echo ✅ 配置文件已存在

REM ============================================================================
REM 第三步: 目录结构准备
REM ============================================================================

REM 创建必要的目录
REM logs: 存储开发环境日志文件
REM git_repo: 存储Git仓库克隆和同步数据
echo 📁 创建开发环境目录...
if not exist logs (
    mkdir logs
    echo   ✅ 创建logs目录 (开发日志存储)
)
if not exist git_repo (
    mkdir git_repo
    echo   ✅ 创建git_repo目录 (Git仓库数据)
)

REM ============================================================================
REM 第四步: MySQL数据库启动
REM ============================================================================

REM 启动MySQL开发数据库（使用Docker容器）
REM 为开发环境提供独立的数据库实例，避免与生产环境冲突
echo 🗄️  启动MySQL开发数据库...
docker --version >nul 2>&1
if not errorlevel 1 (
    echo   📦 检查现有MySQL容器...
    docker ps -a | findstr "docker-sync-mysql-dev" >nul 2>&1
    if not errorlevel 1 (
        echo   🔄 停止并删除现有容器...
        docker stop docker-sync-mysql-dev >nul 2>&1
        docker rm docker-sync-mysql-dev >nul 2>&1
    )
    
    echo   🚀 启动新的MySQL容器...
    echo   配置信息:
    echo     - 容器名称: docker-sync-mysql-dev
    echo     - 端口映射: 3306:3306
    echo     - 数据库: docker_sync
    echo     - 用户: docker_sync
    docker run -d --name docker-sync-mysql-dev -p 3306:3306 -e MYSQL_ROOT_PASSWORD=root123456 -e MYSQL_DATABASE=docker_sync -e MYSQL_USER=docker_sync -e MYSQL_PASSWORD=sync123456 -v "%cd%/sql":/docker-entrypoint-initdb.d mysql:8.0 >nul 2>&1
    
    if errorlevel 1 (
        echo   ❌ MySQL容器启动失败
        echo   请检查Docker是否正常运行或端口3306是否被占用
    ) else (
        echo   ⏳ 等待MySQL初始化完成...
        echo   数据库初始化脚本执行中...
        timeout /t 15 /nobreak >nul
        echo   ✅ MySQL数据库已启动
    )
) else (
    echo   ⚠️  Docker未安装，跳过MySQL容器启动
    echo   请手动启动MySQL数据库服务
)

REM ============================================================================
REM 第五步: Go依赖管理
REM ============================================================================

REM 安装和更新Go依赖包
REM 确保所有后端依赖包都是最新版本
echo 📦 管理Go依赖包...
echo   🔄 下载和整理依赖包...
go mod tidy
if errorlevel 1 (
    echo   ❌ Go依赖安装失败
    echo   请检查网络连接和Go模块配置
    pause
    exit /b 1
)
echo   ✅ Go依赖包已更新

REM ============================================================================
REM 第六步: 后端服务启动
REM ============================================================================

REM 启动Go后端API服务
REM 使用go run命令直接运行，支持代码修改后手动重启
echo 🚀 启动后端API服务...
echo   📡 启动Go服务器 (端口8080)...
echo   🔧 开发模式: 支持手动重启和调试
start "Docker镜像同步平台 - 后端服务" cmd /c "echo 后端服务启动中... && go run main.go && pause"

REM 等待后端服务初始化
echo   ⏳ 等待后端服务启动...
timeout /t 8 /nobreak >nul

REM ============================================================================
REM 第七步: 前端开发服务器启动
REM ============================================================================

REM 启动Vue.js前端开发服务器
REM 使用Vite开发服务器，支持热模块替换(HMR)
echo 🎨 启动前端开发服务器...
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

REM 启动前端开发服务器
echo   🌐 启动Vite开发服务器 (端口5173)...
echo   🔥 热模块替换(HMR): 代码修改实时更新
start "Docker镜像同步平台 - 前端开发服务器" cmd /c "echo 前端开发服务器启动中... && npm run dev"

cd ..

REM ============================================================================
REM 开发环境启动完成
REM ============================================================================

echo.
echo ✅ 🎉 开发环境启动成功！
echo.
echo 🌐 访问地址:
echo   📱 前端开发界面: http://localhost:5173
echo   🔌 后端API接口: http://localhost:8080/api/v1
echo   💚 健康检查接口: http://localhost:8080/api/v1/health
echo   🗄️  数据库连接: localhost:3306 (docker_sync/sync123456)
echo.
echo 🛠️  开发特性:
echo   - 前端热更新: 修改Vue/CSS/JS文件自动刷新
echo   - 后端调试: 在终端窗口查看日志和错误信息
echo   - 数据库管理: 使用MySQL客户端连接开发数据库
echo   - API测试: 使用Postman或curl测试接口
echo.
echo 💡 开发提示:
echo   - 修改后端代码后需要手动重启后端服务
echo   - 前端代码修改会自动热更新，无需重启
echo   - 日志文件保存在 ./logs 目录
echo   - 数据库数据持久化在Docker卷中
echo.
echo 🛑 停止服务:
echo   - 关闭所有终端窗口
echo   - 或在各个服务窗口中按 Ctrl+C
echo   - 停止MySQL容器: docker stop docker-sync-mysql-dev
echo.
echo 按任意键退出此窗口...
pause >nul