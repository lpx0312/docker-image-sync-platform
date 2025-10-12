# 测试脚本功能
$env:HTTP_PROXY = "http://127.0.0.1:7897"
$env:HTTPS_PROXY = "http://127.0.0.1:7897"

# 导入主脚本的函数
. .\docker_hub_search.ps1


Write-Host "=== 测试默认时间倒序排序 ===" -ForegroundColor Cyan
Get-DockerTags -ImageName "alpine" -Count 3

Write-Host "`n=== 测试按名称排序 ===" -ForegroundColor Cyan  
Get-DockerTags -ImageName "alpine" -Count 3 -SortBy "name"

Write-Host "`n=== 测试按大小排序 ===" -ForegroundColor Cyan
Get-DockerTags -ImageName "alpine" -Count 3 -SortBy "size" -Descending

Write-Host "`n=== 测试按查看所有Tag ===" -ForegroundColor Cyan
Get-DockerTags -ImageName "alpine" -All -OutputFile "alpine_img.txt"


# 默认模式：只显示 Linux 架构
Get-DockerTags -ImageName "alpine" -Count 3

# 显示 OS 模式：显示所有架构并添加 OS 前缀
Get-DockerTags -ImageName "alpine" -Count 3 -ShowOS


Write-Host "`n=== 测试架构镜像提取功能 ===" -ForegroundColor Cyan
Get-DockerArchImages -ImageName "alpine" -Count 3 -OutputFile "alpine_img.txt"

Write-Host "`n=== 测试架构镜像提取显示所有 ===" -ForegroundColor Cyan
Get-DockerArchImages -ImageName "mongo" -All -OutputFile "mongo_img.txt"



# Library 镜像（会省略 library/ 前缀）
Get-DockerArchImages -ImageName "mongo" -Count 3

# 用户镜像（保持完整格式）
Get-DockerArchImages -ImageName "nginx/nginx-ingress" -Count 3

# 标签查询也支持用户镜像
Get-DockerTags -ImageName "nginx/nginx-ingress" -Count 5 -SortBy "name"