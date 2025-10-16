我这里使用本地开发模式：
本地开发安装依赖和启动前后端：
make deps         - 安装所有依赖包
make frontend     - 仅启动前端开发服务器 (http://localhost:3000)"
make backend      - 仅启动后端开发服务器 (http://localhost:8080)"
本地开发连接mysql:
mysql -h 192.168.0.180 -P 3307 -u root -pLipanxiang@1102

要求: 
1. 确保前端和后端端口不变
2. 每次启动和重启前端必须使用: 
    lsof -i:3000 | awk 'NR>1{print $2}' | xargs kill -9 
    make frontend
3. 每次启动和重启后端必须使用: 
    lsof -i:8080 | awk 'NR>1{print $2}' | xargs kill -9
    make backend


测试初始化配置保存到数据库system_configs表,可以
1. 清空system_configs表
    mysql -h 192.168.0.180 -P 3307 -u root -pLipanxiang@1102 -e "USE docker_sync; DELETE FROM system_configs;"
2. 重启后端,会初始化默认配置到数据库
3. 检查system_configs表,是否有默认配置
    mysql -h 192.168.0.180 -P 3307 -u root -pLipanxiang@1102 -e "USE docker_sync; SELECT config_key, config_value FROM system_configs ORDER BY config_key;"