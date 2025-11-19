@echo off
chcp 65001 >nul
echo.
echo ========================================
echo   停止 Docker镜像同步平台 开发服务器
echo ========================================
echo.

echo [1/4] 查找占用端口 3000 的进程...
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":3000" ^| findstr "LISTENING"') do (
    echo 发现端口 3000 被进程 %%a 占用
    echo 正在停止进程 %%a...
    taskkill /PID %%a /F >nul 2>&1
    if !errorlevel! equ 0 (
        echo ✓ 端口 3000 进程已停止
    ) else (
        echo - 端口 3000 进程停止失败或不存在
    )
)

echo.
echo [2/4] 查找占用端口 8080 的进程...
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":8080" ^| findstr "LISTENING"') do (
    echo 发现端口 8080 被进程 %%a 占用
    echo 正在停止进程 %%a...
    taskkill /PID %%a /F >nul 2>&1
    if !errorlevel! equ 0 (
        echo ✓ 端口 8080 进程已停止
    ) else (
        echo - 端口 8080 进程停止失败或不存在
    )
)

echo.
echo [3/4] 额外清理 - 停止可能的 Node.js 和 Go 进程...
taskkill /IM node.exe /F >nul 2>&1
taskkill /IM go.exe /F >nul 2>&1
taskkill /IM main.exe /F >nul 2>&1
echo  - 已尝试停止相关开发进程

echo.
echo [4/4] 验证端口状态...
timeout /t 2 >nul

netstat -ano | findstr ":3000" >nul
if !errorlevel! equ 1 (
    echo ✓ 端口 3000 已释放
) else (
    echo ✗ 端口 3000 仍被占用
)

netstat -ano | findstr ":8080" >nul
if !errorlevel! equ 1 (
    echo ✓ 端口 8080 已释放
) else (
    echo ✗ 端口 8080 仍被占用
)

echo.
echo ========================================
echo           端口清理完成！
echo ========================================
echo.
echo 现在可以重新启动开发服务器：
echo   后端: go run main.go
echo   前端: cd web ^&^& npm run dev
echo.
pause