#!/bin/bash

# 数据备份脚本

set -e

# 配置
BACKUP_DIR="backups"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_NAME="docker_sync_backup_$DATE"
MYSQL_CONTAINER="docker-sync-mysql"
MYSQL_USER="root"
MYSQL_PASSWORD="root123456"
MYSQL_DATABASE="docker_sync"
RETENTION_DAYS=7

# 创建备份目录
mkdir -p "$BACKUP_DIR"

# 日志函数
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# 备份数据库
backup_database() {
    log "开始备份数据库..."
    
    if docker ps | grep -q "$MYSQL_CONTAINER"; then
        # 使用Docker容器备份
        docker exec "$MYSQL_CONTAINER" mysqldump \
            -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" \
            --single-transaction \
            --routines \
            --triggers \
            "$MYSQL_DATABASE" > "$BACKUP_DIR/${BACKUP_NAME}_database.sql"
        
        log "数据库备份完成: ${BACKUP_NAME}_database.sql"
    else
        log "警告: MySQL容器未运行，跳过数据库备份"
    fi
}

# 备份配置文件
backup_configs() {
    log "开始备份配置文件..."
    
    tar -czf "$BACKUP_DIR/${BACKUP_NAME}_configs.tar.gz" \
        config.yaml \
        .env 2>/dev/null || true \
        docker-compose.yml \
        nginx.conf
    
    log "配置文件备份完成: ${BACKUP_NAME}_configs.tar.gz"
}

# 备份日志文件
backup_logs() {
    log "开始备份日志文件..."
    
    if [ -d "logs" ] && [ "$(ls -A logs)" ]; then
        tar -czf "$BACKUP_DIR/${BACKUP_NAME}_logs.tar.gz" logs/
        log "日志文件备份完成: ${BACKUP_NAME}_logs.tar.gz"
    else
        log "没有日志文件需要备份"
    fi
}

# 备份Git仓库
backup_git_repo() {
    log "开始备份Git仓库..."
    
    if [ -d "git_repo" ] && [ "$(ls -A git_repo)" ]; then
        tar -czf "$BACKUP_DIR/${BACKUP_NAME}_git_repo.tar.gz" git_repo/
        log "Git仓库备份完成: ${BACKUP_NAME}_git_repo.tar.gz"
    else
        log "没有Git仓库需要备份"
    fi
}

# 创建完整备份包
create_full_backup() {
    log "创建完整备份包..."
    
    cd "$BACKUP_DIR"
    tar -czf "${BACKUP_NAME}_full.tar.gz" ${BACKUP_NAME}_*.sql ${BACKUP_NAME}_*.tar.gz 2>/dev/null || true
    
    # 删除单独的备份文件
    rm -f ${BACKUP_NAME}_*.sql ${BACKUP_NAME}_configs.tar.gz ${BACKUP_NAME}_logs.tar.gz ${BACKUP_NAME}_git_repo.tar.gz 2>/dev/null || true
    
    cd ..
    log "完整备份包创建完成: ${BACKUP_NAME}_full.tar.gz"
}

# 清理旧备份
cleanup_old_backups() {
    log "清理 $RETENTION_DAYS 天前的备份..."
    
    find "$BACKUP_DIR" -name "docker_sync_backup_*" -type f -mtime +$RETENTION_DAYS -delete
    
    log "旧备份清理完成"
}

# 验证备份
verify_backup() {
    local backup_file="$BACKUP_DIR/${BACKUP_NAME}_full.tar.gz"
    
    if [ -f "$backup_file" ]; then
        local size=$(du -h "$backup_file" | cut -f1)
        log "备份验证成功: $backup_file (大小: $size)"
        return 0
    else
        log "错误: 备份文件不存在"
        return 1
    fi
}

# 主备份流程
main() {
    log "开始备份流程..."
    
    backup_database
    backup_configs
    backup_logs
    backup_git_repo
    create_full_backup
    
    if verify_backup; then
        cleanup_old_backups
        log "备份流程完成"
    else
        log "备份流程失败"
        exit 1
    fi
}

# 显示帮助
show_help() {
    echo "数据备份脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help     显示帮助信息"
    echo "  -d, --database 仅备份数据库"
    echo "  -c, --config   仅备份配置文件"
    echo "  -l, --logs     仅备份日志文件"
    echo "  -g, --git      仅备份Git仓库"
    echo ""
    echo "示例:"
    echo "  $0              # 完整备份"
    echo "  $0 -d           # 仅备份数据库"
    echo "  $0 -c -l        # 备份配置和日志"
}

# 参数处理
case "${1:-}" in
    -h|--help)
        show_help
        exit 0
        ;;
    -d|--database)
        backup_database
        ;;
    -c|--config)
        backup_configs
        ;;
    -l|--logs)
        backup_logs
        ;;
    -g|--git)
        backup_git_repo
        ;;
    "")
        main
        ;;
    *)
        echo "未知选项: $1"
        show_help
        exit 1
        ;;
esac