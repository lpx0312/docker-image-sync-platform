#!/bin/bash

# ============================================================================
# 配置迁移脚本
# ============================================================================
# 
# 此脚本用于将config.yaml中的配置迁移到数据库中
# 
# 使用方法:
#   chmod +x scripts/run_migration.sh
#   ./scripts/run_migration.sh
# 
# ============================================================================

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

log_info "项目根目录: $PROJECT_ROOT"

# 检查必要文件
CONFIG_FILE="$PROJECT_ROOT/config.yaml"
MIGRATION_SCRIPT="$SCRIPT_DIR/migrate_config.go"

if [ ! -f "$CONFIG_FILE" ]; then
    log_error "配置文件不存在: $CONFIG_FILE"
    exit 1
fi

if [ ! -f "$MIGRATION_SCRIPT" ]; then
    log_error "迁移脚本不存在: $MIGRATION_SCRIPT"
    exit 1
fi

log_info "检查文件完成"

# 进入项目根目录
cd "$PROJECT_ROOT"

# 检查Go环境
if ! command -v go &> /dev/null; then
    log_error "Go环境未安装或未配置到PATH"
    exit 1
fi

log_info "Go版本: $(go version)"

# 检查go.mod文件
if [ ! -f "go.mod" ]; then
    log_error "go.mod文件不存在，请确保在Go项目根目录执行"
    exit 1
fi

# 下载依赖
log_info "下载Go模块依赖..."
if ! go mod download; then
    log_error "下载依赖失败"
    exit 1
fi

log_success "依赖下载完成"

# 编译迁移脚本
log_info "编译迁移脚本..."
MIGRATION_BINARY="$PROJECT_ROOT/tmp/migrate_config"
mkdir -p "$(dirname "$MIGRATION_BINARY")"

if ! go build -o "$MIGRATION_BINARY" "$MIGRATION_SCRIPT"; then
    log_error "编译迁移脚本失败"
    exit 1
fi

log_success "迁移脚本编译完成"

# 执行迁移
log_info "开始执行配置迁移..."
log_warning "注意: 此操作将会修改数据库中的配置数据"

# 询问用户确认
read -p "是否继续执行迁移? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "用户取消操作"
    exit 0
fi

# 执行迁移
if "$MIGRATION_BINARY"; then
    log_success "配置迁移完成！"
    
    # 清理临时文件
    rm -f "$MIGRATION_BINARY"
    log_info "清理临时文件完成"
    
    echo
    log_success "所有操作完成！配置已成功从config.yaml迁移到数据库"
    log_info "您现在可以通过Web界面管理这些配置"
    
else
    log_error "配置迁移失败"
    exit 1
fi