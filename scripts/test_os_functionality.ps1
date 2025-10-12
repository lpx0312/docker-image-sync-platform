# 测试 OS 功能的脚本
. .\scripts\docker_hub_search.ps1

# 创建模拟数据来测试 OS 前缀功能
function Test-OSPrefixLogic {
    param(
        [bool]$ShowOS
    )
    
    Write-Host "测试 ShowOS = $ShowOS" -ForegroundColor Cyan
    
    # 模拟镜像数据
    $mockImages = @(
        @{ architecture = "amd64"; os = "linux"; variant = $null },
        @{ architecture = "amd64"; os = "windows"; variant = $null },
        @{ architecture = "arm64"; os = "linux"; variant = "v8" },
        @{ architecture = "arm"; os = "linux"; variant = "v7" },
        @{ architecture = "386"; os = "linux"; variant = $null }
    )
    
    $archList = @()
    $linuxArchList = @()
    
    foreach ($image in $mockImages) {
        $arch = $image.architecture
        if ($image.variant) {
            $arch += "/$($image.variant)"
        }
        
        $os = if ($image.os) { $image.os } else { "linux" }
        
        if ($ShowOS) {
            # 显示OS时，非Linux的架构需要加OS前缀
            if ($os -ne "linux") {
                $archWithOS = "$os/$arch"
            } else {
                $archWithOS = $arch
            }
            $archList += $archWithOS
        } else {
            # 不显示OS时，只收集Linux架构
            if ($os -eq "linux") {
                $linuxArchList += $arch
            }
        }
    }
    
    # 根据ShowOS参数决定使用哪个架构列表
    $finalArchList = if ($ShowOS) { $archList } else { $linuxArchList }
    
    Write-Host "架构列表: $($finalArchList -join ', ')" -ForegroundColor Green
    Write-Host ""
}

# 测试两种模式
Test-OSPrefixLogic -ShowOS $false
Test-OSPrefixLogic -ShowOS $true