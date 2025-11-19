# Windows PowerShell 停止端口进程脚本
# 停止占用端口 3000 和 8080 的进程

Write-Host "正在查找占用端口 3000 和 8080 的进程..." -ForegroundColor Yellow

# 方法1: 使用 Get-NetTCPConnection 查找进程ID并停止
Write-Host "`n=== 方法1: 使用 Get-NetTCPConnection ===" -ForegroundColor Cyan

# 查找端口3000
$port3000 = Get-NetTCPConnection -LocalPort 3000 -ErrorAction SilentlyContinue
if ($port3000) {
    $pid3000 = $port3000.OwningProcess
    Write-Host "发现端口 3000 被进程 PID: $pid3000 占用" -ForegroundColor Green
    $process3000 = Get-Process -Id $pid3000 -ErrorAction SilentlyContinue
    if ($process3000) {
        Write-Host "进程名称: $($process3000.ProcessName)" -ForegroundColor Green
        Write-Host "正在停止进程..." -ForegroundColor Yellow
        Stop-Process -Id $pid3000 -Force
        Write-Host "端口 3000 进程已停止" -ForegroundColor Green
    }
} else {
    Write-Host "端口 3000 未被占用" -ForegroundColor Gray
}

# 查找端口8080
$port8080 = Get-NetTCPConnection -LocalPort 8080 -ErrorAction SilentlyContinue
if ($port8080) {
    $pid8080 = $port8080.OwningProcess
    Write-Host "发现端口 8080 被进程 PID: $pid8080 占用" -ForegroundColor Green
    $process8080 = Get-Process -Id $pid8080 -ErrorAction SilentlyContinue
    if ($process8080) {
        Write-Host "进程名称: $($process8080.ProcessName)" -ForegroundColor Green
        Write-Host "正在停止进程..." -ForegroundColor Yellow
        Stop-Process -Id $pid8080 -Force
        Write-Host "端口 8080 进程已停止" -ForegroundColor Green
    }
} else {
    Write-Host "端口 8080 未被占用" -ForegroundColor Gray
}

Write-Host "`n=== 方法2: 使用 netstat (备选方案) ===" -ForegroundColor Cyan

# 方法2: 使用 netstat 查找并停止
$netstat3000 = netstat -ano | findstr ":3000"
if ($netstat3000) {
    Write-Host "发现端口 3000 连接:" -ForegroundColor Yellow
    Write-Host $netstat3000 -ForegroundColor Gray
    # 提取PID并停止
    $netstat3000 -match '\s+(\d+)$' | Out-Null
    $pid = $matches[1]
    if ($pid) {
        Write-Host "停止 PID: $pid" -ForegroundColor Yellow
        Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
    }
}

$netstat8080 = netstat -ano | findstr ":8080"
if ($netstat8080) {
    Write-Host "发现端口 8080 连接:" -ForegroundColor Yellow
    Write-Host $netstat8080 -ForegroundColor Gray
    # 提取PID并停止
    $netstat8080 -match '\s+(\d+)$' | Out-Null
    $pid = $matches[1]
    if ($pid) {
        Write-Host "停止 PID: $pid" -ForegroundColor Yellow
        Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
    }
}

Write-Host "`n=== 验证端口状态 ===" -ForegroundColor Cyan

# 验证端口是否已释放
Start-Sleep -Seconds 2
$check3000 = Get-NetTCPConnection -LocalPort 3000 -ErrorAction SilentlyContinue
$check8080 = Get-NetTCPConnection -LocalPort 8080 -ErrorAction SilentlyContinue

if (-not $check3000) {
    Write-Host "✓ 端口 3000 已释放" -ForegroundColor Green
} else {
    Write-Host "✗ 端口 3000 仍被占用" -ForegroundColor Red
}

if (-not $check8080) {
    Write-Host "✓ 端口 8080 已释放" -ForegroundColor Green
} else {
    Write-Host "✗ 端口 8080 仍被占用" -ForegroundColor Red
}

Write-Host "`n端口清理完成!" -ForegroundColor Green