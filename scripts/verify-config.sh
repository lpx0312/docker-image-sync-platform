#!/bin/bash

# ============================================================================
# Docker镜像同步平台 - 配置验证脚本
# ============================================================================
# 
# 本脚本用于验证环境变量配置是否正确设置
# 
# 使用方法：
# 1. 确保已创建 .env 文件
# 2. 运行: bash scripts/verify-config.sh
# 
# ============================================================================

echo "🔍 Docker镜像同步平台 - 配置验证"
echo "=================================="

# 检查 .env 文件是否存在
if [ ! -f ".env" ]; then
    echo "❌ 错误: .env 文件不存在"
    echo "💡 请复制 .env.production 为 .env 并配置相应的值"
    echo "   cp .env.production .env"
    exit 1
fi

echo "✅ .env 文件存在"

# 加载环境变量
source .env

# 验证必需的配置项
echo ""
echo "🔧 验证必需配置项..."

# 数据库配置验证
echo ""
echo "📊 数据库配置:"
if [ -z "$MYSQL_ROOT_PASSWORD" ]; then
    echo "❌ MYSQL_ROOT_PASSWORD 未设置"
    ERRORS=1
else
    echo "✅ MYSQL_ROOT_PASSWORD 已设置"
fi

if [ -z "$MYSQL_PASSWORD" ]; then
    echo "❌ MYSQL_PASSWORD 未设置"
    ERRORS=1
else
    echo "✅ MYSQL_PASSWORD 已设置"
fi

# Git配置验证
echo ""
echo "🔗 Git仓库配置:"
GITEE_CONFIGURED=0
GITHUB_CONFIGURED=0

if [ -n "$GITEE_REPO_URL" ] && [ -n "$GITEE_USERNAME" ] && [ -n "$GITEE_PASSWORD" ]; then
    echo "✅ Gitee 配置完整"
    GITEE_CONFIGURED=1
else
    echo "⚠️  Gitee 配置不完整"
fi

if [ -n "$GITHUB_REPO_URL" ] && [ -n "$GITHUB_USERNAME" ] && [ -n "$GITHUB_TOKEN" ]; then
    echo "✅ GitHub 配置完整"
    GITHUB_CONFIGURED=1
else
    echo "⚠️  GitHub 配置不完整"
fi

if [ $GITEE_CONFIGURED -eq 0 ] && [ $GITHUB_CONFIGURED -eq 0 ]; then
    echo "❌ 错误: 至少需要配置一个Git仓库 (Gitee 或 GitHub)"
    ERRORS=1
fi

# 阿里云配置验证
echo ""
echo "☁️  阿里云配置:"
if [ -n "$ALIYUN_REGISTRY" ] && [ -n "$ALIYUN_NAMESPACE" ] && [ -n "$ALIYUN_USERNAME" ] && [ -n "$ALIYUN_PASSWORD" ]; then
    echo "✅ 阿里云配置完整"
else
    echo "⚠️  阿里云配置不完整 (如果不使用阿里云可忽略)"
fi

# 基础配置验证
echo ""
echo "⚙️  基础配置:"
echo "   GIN_MODE: ${GIN_MODE:-debug}"
echo "   LOG_LEVEL: ${LOG_LEVEL:-info}"
echo "   APP_ENV: ${APP_ENV:-development}"

# 总结
echo ""
echo "=================================="
if [ -n "$ERRORS" ]; then
    echo "❌ 配置验证失败，请检查上述错误项"
    exit 1
else
    echo "✅ 配置验证通过！"
    echo ""
    echo "🚀 现在可以启动服务:"
    echo "   docker-compose -f docker-compose-all.yml up -d"
    echo ""
    echo "📝 查看服务状态:"
    echo "   docker-compose -f docker-compose-all.yml ps"
    echo ""
    echo "📋 查看服务日志:"
    echo "   docker-compose -f docker-compose-all.yml logs -f"
fi