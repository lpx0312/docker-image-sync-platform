#!/bin/bash

# 监控脚本

set -e

# 配置
API_URL="http://localhost:8080/api/v1"
LOG_FILE="logs/monitor.log"
CHECK_INTERVAL=60  # 检查间隔（秒）
ALERT_THRESHOLD=5  # 连续失败次数阈值

# 创建日志目录
mkdir -p logs

# 计数器
failure_count=0

# 日志函数
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

# 发送告警（可以扩展为邮件、钉钉等）
send_alert() {
    local message=$1
    log "ALERT: $message"
    
    # 这里可以添加邮件、钉钉、Slack等告警通知
    # 例如：
    # curl -X POST "https://oapi.dingtalk.com/robot/send?access_token=YOUR_TOKEN" \
    #      -H "Content-Type: application/json" \
    #      -d "{\"msgtype\":\"text\",\"text\":{\"content\":\"$message\"}}"
}

# 检查服务健康状态
check_health() {
    if response=$(curl -s --connect-timeout 10 "$API_URL/health" 2>/dev/null); then
        if echo "$response" | jq -e '.status' | grep -q "ok"; then
            return 0
        fi
    fi
    return 1
}

# 获取系统状态
get_system_stats() {
    local stats=""
    
    # 获取镜像统计
    if response=$(curl -s --connect-timeout 10 "$API_URL/images/stats" 2>/dev/null); then
        total=$(echo "$response" | jq -r '.total // 0')
        success=$(echo "$response" | jq -r '.success // 0')
        failed=$(echo "$response" | jq -r '.failed // 0')
        syncing=$(echo "$response" | jq -r '.syncing // 0')
        
        stats="镜像统计: 总计=$total, 成功=$success, 失败=$failed, 同步中=$syncing"
    fi
    
    # 获取Docker容器状态
    if command -v docker >/dev/null 2>&1; then
        if docker-compose ps 2>/dev/null | grep -q "Up"; then
            stats="$stats | Docker: 运行中"
        else
            stats="$stats | Docker: 停止"
        fi
    fi
    
    echo "$stats"
}

# 主监控循环
monitor() {
    log "开始监控服务..."
    
    while true; do
        if check_health; then
            if [ $failure_count -gt 0 ]; then
                log "服务恢复正常"
                send_alert "Docker镜像同步平台服务已恢复正常"
                failure_count=0
            fi
            
            # 记录系统状态
            stats=$(get_system_stats)
            log "服务正常 | $stats"
            
        else
            failure_count=$((failure_count + 1))
            log "服务检查失败 (第 $failure_count 次)"
            
            if [ $failure_count -eq $ALERT_THRESHOLD ]; then
                send_alert "Docker镜像同步平台服务连续 $failure_count 次检查失败，请立即检查！"
            elif [ $failure_count -gt $ALERT_THRESHOLD ]; then
                # 每10次失败发送一次告警
                if [ $((failure_count % 10)) -eq 0 ]; then
                    send_alert "Docker镜像同步平台服务仍然异常，已失败 $failure_count 次"
                fi
            fi
        fi
        
        sleep $CHECK_INTERVAL
    done
}

# 信号处理
trap 'log "监控服务停止"; exit 0' INT TERM

# 启动监控
log "启动监控服务，检查间隔: ${CHECK_INTERVAL}秒"
monitor