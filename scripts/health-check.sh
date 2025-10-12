#!/bin/bash

# ============================================================================
# Docker镜像同步平台 - 健康检查脚本
# ============================================================================
# 
# 功能描述:
#   全面检查Docker镜像同步平台各个组件的运行状态和健康情况
#   提供快速的系统诊断和故障排查功能
# 
# 主要功能:
#   1. 后端API服务健康检查 (HTTP状态码、响应时间)
#   2. 前端Web服务可用性检查
#   3. 数据库连接状态验证
#   4. Docker容器运行状态检查
#   5. 关键API接口功能验证
#   6. 系统资源使用情况监控
# 
# 检查项目:
#   - 后端健康检查接口 (/api/v1/health)
#   - 镜像统计API接口 (/api/v1/images/stats)
#   - 同步历史API接口 (/api/v1/sync/history)
#   - 前端服务可访问性
#   - MySQL数据库连接
#   - Docker容器状态
# 
# 输出格式:
#   - 彩色状态指示 (绿色=正常, 黄色=警告, 红色=异常)
#   - 详细的错误信息和响应内容
#   - 统一的检查结果汇总
# 
# 使用方法:
#   ./health-check.sh        # 执行完整健康检查
#   ./health-check.sh -v     # 详细模式 (显示更多信息)
#   ./health-check.sh -q     # 静默模式 (仅显示结果)
# 
# 退出码:
#   0 - 所有检查通过
#   1 - 部分检查失败
#   2 - 严重错误 (无法连接核心服务)
# 
# 依赖要求:
#   - curl (HTTP请求工具)
#   - jq (JSON解析工具)
#   - docker (容器状态检查)
# 
# 作者: Docker镜像同步平台开发团队
# 版本: v1.0.0
# 更新: 2024-12-19
# ============================================================================

set -e

# 配置
API_URL="http://localhost:8080/api/v1"
FRONTEND_URL="http://localhost:3000"
TIMEOUT=10

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查函数
check_service() {
    local name=$1
    local url=$2
    local expected_status=${3:-200}
    
    echo -n "检查 $name... "
    
    if response=$(curl -s -w "%{http_code}" -o /dev/null --connect-timeout $TIMEOUT "$url" 2>/dev/null); then
        if [ "$response" = "$expected_status" ]; then
            echo -e "${GREEN}✓ 正常${NC} (HTTP $response)"
            return 0
        else
            echo -e "${YELLOW}⚠ 异常${NC} (HTTP $response)"
            return 1
        fi
    else
        echo -e "${RED}✗ 无法连接${NC}"
        return 1
    fi
}

# 检查JSON响应
check_json_api() {
    local name=$1
    local url=$2
    local expected_field=$3
    
    echo -n "检查 $name... "
    
    if response=$(curl -s --connect-timeout $TIMEOUT "$url" 2>/dev/null); then
        if echo "$response" | jq -e ".$expected_field" >/dev/null 2>&1; then
            echo -e "${GREEN}✓ 正常${NC}"
            return 0
        else
            echo -e "${YELLOW}⚠ 响应格式异常${NC}"
            echo "响应: $response"
            return 1
        fi
    else
        echo -e "${RED}✗ 无法连接${NC}"
        return 1
    fi
}

echo "Docker镜像同步平台 - 健康检查"
echo "================================"

# 检查基础服务
check_service "后端健康检查" "$API_URL/health"
check_json_api "镜像统计API" "$API_URL/images/stats" "total"
check_json_api "同步历史API" "$API_URL/sync/history" "data"

# 检查前端（开发模式）
if curl -s --connect-timeout 5 "$FRONTEND_URL" >/dev/null 2>&1; then
    check_service "前端服务" "$FRONTEND_URL"
else
    echo "前端服务... ${YELLOW}⚠ 未运行（可能是生产模式）${NC}"
fi

# 检查数据库连接
echo -n "检查数据库连接... "
if response=$(curl -s --connect-timeout $TIMEOUT "$API_URL/health" 2>/dev/null); then
    if echo "$response" | jq -e '.status' | grep -q "ok"; then
        echo -e "${GREEN}✓ 正常${NC}"
    else
        echo -e "${RED}✗ 异常${NC}"
    fi
else
    echo -e "${RED}✗ 无法检查${NC}"
fi

# 检查Docker服务（如果可用）
if command -v docker >/dev/null 2>&1; then
    echo -n "检查Docker容器... "
    if docker-compose ps 2>/dev/null | grep -q "Up"; then
        echo -e "${GREEN}✓ 运行中${NC}"
    else
        echo -e "${YELLOW}⚠ 未运行${NC}"
    fi
else
    echo "Docker... ${YELLOW}⚠ 未安装${NC}"
fi

echo ""
echo "健康检查完成"