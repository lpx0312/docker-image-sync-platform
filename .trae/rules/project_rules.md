本地调试就用：
deps         - 安装所有依赖包
dev          - 启动完整开发环境（前端+后端）
本地前端测试：http://localhost:3000
本地后端测试：http://localhost:8080
上面前后端端口不要变更，如果需要重新启动，请删除所有占用端口进程
后端接口占用可以用： lsof -i:8080 | awk 'NR>1{print $2}' | xargs kill -9 删除进程