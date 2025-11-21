#!/bin/bash

# ============================================================================
# Docker镜像同步平台 - 数据备份脚本
# ============================================================================
# 
# 功能描述:
#   自动化备份Docker镜像同步平台的关键数据和配置文件
#   支持数据库备份、配置文件备份、日志备份等多种备份类型
# 
# 主要功能:
#   1. MySQL数据库完整备份 (包含表结构、数据、存储过程、触发器)
#   2. 配置文件备份 (config.yaml, .env等)
#   3. 日志文件备份 (应用日志、监控日志)
#   4. Git仓库数据备份 (本地克隆的仓库)
#   5. 自动清理过期备份文件
#   6. 备份完整性验证
# 
# 备份策略:
#   - 数据库: 使用mysqldump进行逻辑备份
#   - 文件: 使用tar压缩打包
#   - 命名: 时间戳格式 (YYYYMMDD_HHMMSS)
#   - 保留: 默认保留7天的备份文件
#   - 存储: 本地backups目录
# 
# 使用方法:
#   ./backup.sh              # 完整备份
#   ./backup.sh -d           # 仅备份数据库
#   ./backup.sh -c           # 仅备份配置文件
#   ./backup.sh -l           # 仅备份日志文件
#   ./backup.sh --help       # 显示帮助信息
# 
# 依赖要求:
#   - Docker (用于访问MySQL容器)
#   - mysqldump (数据库备份工具)
#   - tar (文件打包工具)
#   - 足够的磁盘空间
# 
# 注意事项:
#   - 备份过程中会短暂锁定数据库表
#   - 确保有足够的磁盘空间存储备份文件
#   - 生产环境建议在低峰期执行备份
#   - 定期验证备份文件的完整性
# 
# 作者: Docker镜像同步平台开发团队
# 版本: v1.0.0
# 更新: 2024-12-19
# ============================================================================

set -e

# ============================================================================
# 备份配置参数
# ============================================================================

# 备份存储目录 (相对于项目根目录)
BACKUP_DIR="backups"

# 生成带时间戳的备份名称 (格式: YYYYMMDD_HHMMSS)
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_NAME="docker_sync_backup_$DATE"

# MySQL数据库连接配置
MYSQL_CONTAINER="docker-sync-mysql-dev"    # MySQL容器名称
MYSQL_USER="root"                           # 数据库用户名
MYSQL_PASSWORD="123456"                      # 数据库密码 (与config.yaml保持一致)
MYSQL_DATABASE="docker_sync"                 # 数据库名称

# 备份保留策略 (保留最近N天的备份文件)
RETENTION_DAYS=7

# ============================================================================
# 初始化和工具函数
# ============================================================================

# 创建备份目录 (如果不存在)
mkdir -p "$BACKUP_DIR"

# 日志记录函数 - 统一的日志输出格式
# 参数: $1 - 日志消息内容
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# ============================================================================
# 数据库备份功能
# ============================================================================

# 备份MySQL数据库
# 功能: 使用mysqldump工具备份完整的数据库结构和数据
# 输出: ${BACKUP_NAME}_database.sql
backup_database() {
    log "开始备份数据库..."
    
    # 检查MySQL容器是否正在运行
    if docker ps | grep -q "$MYSQL_CONTAINER"; then
        # 使用Docker容器内的mysqldump工具进行备份
        # --single-transaction: 确保备份的一致性 (InnoDB表)
        # --routines: 备份存储过程和函数
        # --triggers: 备份触发器
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

# ============================================================================
# 配置文件备份功能
# ============================================================================

# 备份系统配置文件
# 功能: 打包所有重要的配置文件，确保系统可以完整恢复
# 输出: ${BACKUP_NAME}_configs.tar.gz
backup_configs() {
    log "开始备份配置文件..."

    # 构建tar命令的文件列表
    local files_to_backup=""
    local config_files=("config.yaml" ".env" "docker-compose.yml" "nginx.conf")
    local existing_files=()

    # 检查哪些配置文件存在
    for file in "${config_files[@]}"; do
        if [ -f "$file" ]; then
            existing_files+=("$file")
        fi
    done

    # 如果没有找到任何配置文件，记录并返回
    if [ ${#existing_files[@]} -eq 0 ]; then
        log "没有找到配置文件需要备份"
        return 0
    fi

    # 使用tar命令打包压缩存在的配置文件
    tar -czf "$BACKUP_DIR/${BACKUP_NAME}_configs.tar.gz" "${existing_files[@]}"

    log "配置文件备份完成: ${BACKUP_NAME}_configs.tar.gz (包含 ${#existing_files[@]} 个文件)"
}

# ============================================================================
# 日志文件备份功能
# ============================================================================

# 备份应用日志文件
# 功能: 备份logs目录下的所有日志文件，用于问题排查和审计
# 输出: ${BACKUP_NAME}_logs.tar.gz
backup_logs() {
    log "开始备份日志文件..."
    
    # 检查logs目录是否存在且不为空
    if [ -d "logs" ] && [ "$(ls -A logs)" ]; then
        # 打包压缩整个logs目录
        tar -czf "$BACKUP_DIR/${BACKUP_NAME}_logs.tar.gz" logs/
        log "日志文件备份完成: ${BACKUP_NAME}_logs.tar.gz"
    else
        log "没有日志文件需要备份"
    fi
}

# ============================================================================
# Git仓库备份功能
# ============================================================================

# 备份Git临时数据
# 功能: 备份temp目录，包含Git操作临时数据
# 输出: ${BACKUP_NAME}_temp.tar.gz
backup_temp_data() {
    log "开始备份临时数据..."

    # 检查temp目录是否存在且不为空
    if [ -d "temp" ] && [ "$(ls -A temp 2>/dev/null)" ]; then
        # 打包压缩整个temp目录
        tar -czf "$BACKUP_DIR/${BACKUP_NAME}_temp.tar.gz" temp/
        log "临时数据备份完成: ${BACKUP_NAME}_temp.tar.gz"
    else
        log "没有临时数据需要备份"
    fi
}

# ============================================================================
# 完整备份包创建功能
# ============================================================================

# 创建统一的完整备份包
# 功能: 将所有单独的备份文件合并为一个完整的备份包，便于管理和传输
# 输出: ${BACKUP_NAME}_full.tar.gz
create_full_backup() {
    log "创建完整备份包..."
    
    # 进入备份目录进行操作
    cd "$BACKUP_DIR"
    
    # 将所有相关的备份文件打包为一个完整备份
    # 包括: 数据库备份(.sql)和各种配置/数据备份(.tar.gz)
    tar -czf "${BACKUP_NAME}_full.tar.gz" ${BACKUP_NAME}_*.sql ${BACKUP_NAME}_*.tar.gz 2>/dev/null || true
    
    # 清理单独的备份文件，只保留完整备份包
    # 这样可以节省存储空间，避免文件冗余
    rm -f ${BACKUP_NAME}_*.sql ${BACKUP_NAME}_configs.tar.gz ${BACKUP_NAME}_logs.tar.gz ${BACKUP_NAME}_git_repo.tar.gz 2>/dev/null || true
    
    # 返回原目录
    cd ..
    log "完整备份包创建完成: ${BACKUP_NAME}_full.tar.gz"
}

# ============================================================================
# 备份清理功能
# ============================================================================

# 清理过期的备份文件
# 功能: 根据保留策略删除超过指定天数的旧备份，节省存储空间
# 参数: 使用全局变量 RETENTION_DAYS (默认7天)
cleanup_old_backups() {
    log "清理 $RETENTION_DAYS 天前的备份..."
    
    # 使用find命令查找并删除过期的备份文件
    # -name: 匹配文件名模式
    # -type f: 仅匹配文件 (不包括目录)
    # -mtime +N: 修改时间超过N天的文件
    # -delete: 删除匹配的文件
    find "$BACKUP_DIR" -name "docker_sync_backup_*" -type f -mtime +$RETENTION_DAYS -delete
    
    log "旧备份清理完成"
}

# ============================================================================
# 备份验证功能
# ============================================================================

# 验证备份文件的完整性
# 功能: 检查备份文件是否成功创建，并显示文件大小信息
# 返回: 0=验证成功, 1=验证失败
verify_backup() {
    local backup_file="$BACKUP_DIR/${BACKUP_NAME}_full.tar.gz"
    
    # 检查备份文件是否存在
    if [ -f "$backup_file" ]; then
        # 获取文件大小 (人类可读格式)
        local size=$(du -h "$backup_file" | cut -f1)
        log "备份验证成功: $backup_file (大小: $size)"
        return 0
    else
        log "错误: 备份文件不存在"
        return 1
    fi
}

# ============================================================================
# 主备份流程
# ============================================================================

# 执行完整的备份流程
# 功能: 按顺序执行所有备份步骤，并进行验证和清理
# 流程: 数据库备份 -> 配置备份 -> 日志备份 -> Git仓库备份 -> 
#       创建完整包 -> 验证备份 -> 清理旧备份
main() {
    log "开始备份流程..."
    
    # 执行各项备份任务
    backup_database      # 备份MySQL数据库
    backup_configs       # 备份配置文件
    backup_logs         # 备份日志文件
    backup_temp_data    # 备份临时数据
    create_full_backup  # 创建完整备份包
    
    # 验证备份是否成功
    if verify_backup; then
        cleanup_old_backups  # 清理旧备份文件
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
    echo "  -g, --git      仅备份临时数据"
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
        backup_temp_data
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