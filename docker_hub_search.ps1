# Docker Hub Search Script with Architecture Extraction
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
            $uri = "https://hub.docker.com/v2/repositories/$Username/$ImageName/tags/?page=$page" + "`&page_size=$pageSize"
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
                    Tag = $tag.name
                    Status = $tag.tag_status
                    Size = [math]::Round($tag.full_size/1024/1024, 2)
                    Architecture = ($archList | Sort-Object -Unique) -join ", "
                    LastUpdated = $tag.last_updated
                }
                
                $allTags += $tagInfo
            }
            
            $page++
            Start-Sleep -Milliseconds 500
        }
        catch {
            Write-Error "获取数据失败: $_"
            break
        }
    } while ($data.next)
    
    # 应用搜索过滤
    if ($Search) {
        $allTags = $allTags | Where-Object { $_.Tag -like "*$Search*" }
    }
    
    # 排序
    switch ($SortBy) {
        "name" { 
            $allTags = $allTags | Sort-Object -Property Tag
            if ($Descending) { $allTags = $allTags | Sort-Object -Property Tag -Descending }
        }
        "size" { 
            $allTags = $allTags | Sort-Object -Property Size
            if ($Descending) { $allTags = $allTags | Sort-Object -Property Size -Descending }
        }
        "date" { 
            $allTags = $allTags | Sort-Object -Property LastUpdated
            if ($Descending) { $allTags = $allTags | Sort-Object -Property LastUpdated -Descending }
        }
    }
    
    # 确定显示的标签
    if ($All) {
        $displayTags = $allTags
    } else {
        $displayTags = $allTags | Select-Object -First $Count
    }
    
    Write-Host ""
    Write-Host "找到 $($allTags.Count) 个标签" -ForegroundColor Green
    if (-not $All) {
        Write-Host "显示前 $Count 个标签" -ForegroundColor Yellow
    }
    Write-Host ""
    
    # 显示结果
    $displayTags | Format-Table -AutoSize
    
    # 提示信息
    if (-not $All -and $allTags.Count -gt $Count) {
        Write-Host "`n提示: 使用 -All 参数查看所有 $($allTags.Count) 个标签" -ForegroundColor Yellow
    }
}

# 新增功能：提取指定镜像的 amd64 和 arm64 架构标签
function Get-DockerArchImages {
    param(
        [Parameter(Mandatory=$true)]
        [string]$ImageName,
        
        [string]$Username = "library",
        
        [int]$MaxPages = 5
    )
    
    Write-Host "Getting $Username/$ImageName image amd64 and arm64 architecture info..." -ForegroundColor Cyan
    
    $allTags = @()
    $page = 1
    $pageSize = 100
    
    # Get tag data from multiple pages
    for ($i = 1; $i -le $MaxPages; $i++) {
        try {
            $uri = "https://hub.docker.com/v2/repositories/$Username/$ImageName/tags/?page=$i" + "`&page_size=$pageSize"
            Write-Host "Getting page $i data..." -ForegroundColor Yellow
            
            $response = Invoke-WebRequest -Uri $uri -UseBasicParsing
            $data = $response.Content | ConvertFrom-Json
            
            if ($data.results.Count -eq 0) {
                break
            }
            
            foreach ($tag in $data.results) {
                # Extract architecture info
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
                
                # Check if contains amd64 or arm64 architecture
                $hasAmd64 = $archList -contains "amd64"
                $hasArm64 = ($archList -contains "arm64") -or ($archList -contains "arm64/v8")
                
                if ($hasAmd64 -or $hasArm64) {
                    $tagInfo = [PSCustomObject]@{
                        Tag = $tag.name
                        Status = $tag.tag_status
                        Size = [math]::Round($tag.full_size/1024/1024, 2)
                        Architecture = ($archList | Sort-Object -Unique) -join ", "
                        LastUpdated = $tag.last_updated
                        HasAmd64 = $hasAmd64
                        HasArm64 = $hasArm64
                    }
                    
                    $allTags += $tagInfo
                }
            }
            
            Start-Sleep -Milliseconds 500
        }
        catch {
            Write-Error "Failed to get data: $_"
            break
        }
    }
    
    # Sort by tag name
    $allTags = $allTags | Sort-Object -Property Tag
    
    Write-Host ""
    Write-Host "Found $($allTags.Count) tags with amd64 or arm64 architecture" -ForegroundColor Green
    Write-Host ""
    
    # Display table
    $allTags | Select-Object Tag, Status, Size, Architecture, LastUpdated | Format-Table -AutoSize
    
    # Generate Docker command list
    Write-Host "=== Docker Image Pull Commands ===" -ForegroundColor Yellow
    $dockerCommands = @()
    
    foreach ($tag in $allTags) {
        $tagName = $tag.Tag
        
        if ($tag.HasAmd64) {
            $dockerCommands += "$Username/$ImageName`:$tagName"
        }
        
        if ($tag.HasArm64) {
            $dockerCommands += "--platform linux/arm64 $Username/$ImageName`:$tagName"
        }
    }
    
    # Output all commands
    foreach ($cmd in $dockerCommands) {
        Write-Host $cmd -ForegroundColor White
    }
    
    Write-Host ""
    Write-Host "Total generated $($dockerCommands.Count) Docker commands" -ForegroundColor Green
}

# 测试调用
# 默认显示前10个
Get-DockerTags -ImageName "alpine" -Count 10
# 显示所有标签（这就是你要的！）
Get-DockerTags -ImageName "alpine" -All

# 搜索并显示所有结果
Get-DockerTags -ImageName "alpine" -Search "2.7" -All

# 按大小排序显示所有
Get-DockerTags -ImageName "alpine" -SortBy size -Descending -All



Write-Host ""
Write-Host "=== Architecture Image Extraction Demo ===" -ForegroundColor Magenta
Get-DockerArchImages -ImageName "alpine" -MaxPages 3