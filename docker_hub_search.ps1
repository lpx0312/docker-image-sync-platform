# 重新创建完整的Docker标签查看函数
$env:HTTP_PROXY = "http://127.0.0.1:7897"
$env:HTTPS_PROXY = "http://127.0.0.1:7897"

function Get-DockerTags {
    param(
        [Parameter(Mandatory=$true)]
        [string]$ImageName,
        
        [string]$Username = "library",
        
        [string]$Search = "",
        
        [int]$Count = 10,
        
        [ValidateSet("name", "size", "date")]
        [string]$SortBy = "name",
        
        [switch]$Descending,
        
        [switch]$All
    )
    
    Write-Host "镜像: $Username/$ImageName" -ForegroundColor Cyan
    
    $allTags = @()
    $page = 1
    $pageSize = 100
    
    # 获取所有标签数据
    do {
        try {
            $uri = "https://hub.docker.com/v2/repositories/$Username/$ImageName/tags/?page=$page&page_size=$pageSize"
            $response = Invoke-WebRequest -Uri $uri -UseBasicParsing
            $data = $response.Content | ConvertFrom-Json
            
            if ($data.results.Count -eq 0) {
                break
            }
            
            foreach ($tag in $data.results) {
                # 提取架构信息
                $archList = @()
                if ($tag.images -and $tag.images.Count -gt 0) {
                    foreach ($image in $tag.images) {
                        if ($image.architecture -and $image.architecture -ne "unknown") {
                            $arch = $image.architecture
                            if ($image.variant) {
                                $arch += "/$($image.variant)"
                            }
                            $archList += $arch
                        }
                    }
                }
                
                $tagInfo = [PSCustomObject]@{
                    标签 = $tag.name
                    状态 = $tag.tag_status
                    大小 = [math]::Round($tag.full_size/1024/1024, 2)
                    架构 = ($archList | Sort-Object -Unique) -join ", "
                    更新时间 = $tag.last_updated
                }
                
                $allTags += $tagInfo
            }
            
            if (-not $data.next) {
                break
            }
            
            $page++
            Start-Sleep -Milliseconds 100
        }
        catch {
            Write-Error "获取数据失败: $_"
            break
        }
    } while ($true)
    
    # 应用搜索过滤
    if ($Search) {
        $allTags = $allTags | Where-Object { $_.标签 -like "*$Search*" }
    }
    
    # 排序
    switch ($SortBy) {
        "name" { 
            $allTags = $allTags | Sort-Object -Property 标签
            if ($Descending) { $allTags = $allTags | Sort-Object -Property 标签 -Descending }
        }
        "size" { 
            $allTags = $allTags | Sort-Object -Property 大小
            if ($Descending) { $allTags = $allTags | Sort-Object -Property 大小 -Descending }
        }
        "date" { 
            $allTags = $allTags | Sort-Object -Property 更新时间
            if ($Descending) { $allTags = $allTags | Sort-Object -Property 更新时间 -Descending }
        }
    }
    
    # 显示统计信息
    if ($All) {
        Write-Host "找到 $($allTags.Count) 个标签（显示所有）" -ForegroundColor Green
        $displayTags = $allTags
    } else {
        Write-Host "找到 $($allTags.Count) 个标签（显示前 $Count 个）" -ForegroundColor Green
        $displayTags = $allTags | Select-Object -First $Count
    }
    
    Write-Host ""
    
    # 显示结果
    $displayTags | Format-Table -AutoSize
    
    # 提示信息
    if (-not $All -and $allTags.Count -gt $Count) {
        Write-Host "`n提示: 使用 -All 参数查看所有 $($allTags.Count) 个标签" -ForegroundColor Yellow
    }
}

# 测试所有功能
Write-Host "`n=== 基础显示（前10个）===" -ForegroundColor Magenta
Get-DockerTags -ImageName "alpine" -Count 5

Write-Host "`n=== 显示所有标签（前几个）===" -ForegroundColor Magenta  
Get-DockerTags -ImageName "alpine" -All | Select-Object -First 3

Write-Host "`n=== 搜索 '2.7' 并显示所有结果 ===" -ForegroundColor Magenta
Get-DockerTags -ImageName "alpine" -Search "2.7" -All

Write-Host "`n=== 按大小排序（前5个）===" -ForegroundColor Magenta
Get-DockerTags -ImageName "alpine" -SortBy size -Descending -Count 5