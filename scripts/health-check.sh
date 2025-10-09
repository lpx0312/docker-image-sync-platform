#!/bin/bash

# 健康检查脚本

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