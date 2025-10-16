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

测试最终生效的配置是config.yaml 还是环境变量的配置存入到数据库system_configs表，可以
1. 不设置环境变量,只设置config.yaml
2. 设置环境变量,不设置config.yaml
3. 同时设置环境变量和config.yaml
分上面几种方式测试，存入所有数据库的配置，都要之上上面的测试，aliyun_registry, aliyun_namespace, aliyun_username, aliyun_password, gitee_repo_url, gitee_username, gitee_password 
例如: git.gitee.token(config.yaml中存在) GIT_GITEE_TOKEN(环境变量中存在) gitee_token(数据库中字段)  value(数据库中的值)
 1. 不设置环境变量,只设置config.yaml, 数据库中gitee_token字段值为config.yaml中的git.gitee.token值
 2. 设置环境变量,不设置config.yaml, 数据库中gitee_token字段值为环境变量中的GIT_GITEE_TOKEN值
 3. 同时设置环境变量和config.yaml, 数据库中gitee_token字段值为环境变量中的GIT_GITEE_TOKEN值, 忽略config.yaml中的git.gitee.token值
 注: 当前环境为ubuntu 22.04 ，环境变量可以通过: 设置 export GIT_GITEE_TOKEN="gitee-token-value"  取消 unset GIT_GITEE_TOKEN 
