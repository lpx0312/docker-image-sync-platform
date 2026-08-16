# ============================================================================
# Docker镜像同步平台 - Makefile
# ============================================================================
#
# 本Makefile提供了完整的开发、构建、测试和部署工具链
# 
# 快速开始：
#   make init    - 初始化项目环境
#   make deps    - 安装所有依赖
#   make dev     - 启动开发环境
#   make build   - 构建生产版本
#   make docker-run - 使用Docker运行
#
# 环境要求：
#   - Go 1.21+
#   - Node.js 18+
#   - Docker & Docker Compose
#   - golangci-lint (可选，用于代码检查)
#
# ============================================================================


# 项目配置
PROJECT_NAME := docker-image-sync-platform
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO_VERSION := $(shell go version | awk '{print $$3}')

# 构建标志
LDFLAGS := -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GoVersion=$(GO_VERSION)
BUILD_FLAGS := -ldflags "$(LDFLAGS)" -trimpath

# 目录定义
BIN_DIR := bin
WEB_DIR := web
LOGS_DIR := logs
TEMP_DIR := temp

# 声明所有伪目标
.PHONY: help init deps build run dev test clean docker-build docker-run docker-stop \
        frontend backend build-frontend build-backend test-backend fmt lint \
        docker-logs docker-clean docker-rebuild health-check cli cli-install

# ============================================================================
# 帮助信息
# ============================================================================

# 默认目标：显示帮助信息
help:
	@echo "============================================================================"
	@echo "Docker镜像同步平台 - 构建工具"
	@echo "============================================================================"
	@echo ""
	@echo "🚀 快速开始:"
	@echo "  init         - 初始化项目环境（首次运行必须）"
	@echo "  deps         - 安装所有依赖包"
	@echo "  dev          - 启动完整开发环境（前端+后端）"
	@echo ""
	@echo "🔧 开发相关:"
	@echo "  frontend     - 仅启动前端开发服务器 (http://localhost:3000)"
	@echo "  backend      - 仅启动后端开发服务器 (http://localhost:8080)"
	@echo "  health-check - 检查开发环境健康状态"
	@echo ""
	@echo "🏗️  构建相关:"
	@echo "  build        - 构建完整应用（前端+后端）"
	@echo "  build-frontend - 仅构建前端静态文件"
	@echo "  build-backend  - 仅构建后端可执行文件"
	@echo "  cli          - 构建 dsync 命令行客户端（bin/dsync）"
	@echo "  cli-install  - 安装 dsync 到 /usr/local/bin（需要 sudo）"
	@echo "  run          - 运行构建后的应用"
	@echo ""
	@echo "🧪 测试相关:"
	@echo "  test         - 运行所有测试"
	@echo "  test-backend - 运行后端单元测试"
	@echo ""
	@echo "🔍 代码质量:"
	@echo "  fmt          - 格式化所有代码"
	@echo "  lint         - 运行代码检查"
	@echo ""
	@echo "🧹 清理相关:"
	@echo "  clean        - 清理所有构建文件和缓存"
	@echo ""
	@echo "📊 项目信息:"
	@echo "  版本: $(VERSION)"
	@echo "  Go版本: $(GO_VERSION)"
	@echo "  构建时间: $(BUILD_TIME)"
	@echo ""
	@echo "============================================================================"

# ============================================================================
# 项目初始化
# ============================================================================

# 初始化项目环境
# 创建必要的目录结构，复制配置文件模板
init:
	@echo "🚀 初始化项目环境..."
	@echo "创建目录结构..."
	@mkdir -p $(BIN_DIR) $(LOGS_DIR) $(TEMP_DIR)
# @echo "复制配置文件模板..."
# @if [ ! -f .env ]; then \
# 	if [ -f .env.example ]; then \
# 		cp .env.example .env; \
# 		echo "✅ 已创建 .env 文件，请根据需要修改配置"; \
# 	else \
# 		echo "⚠️  未找到 .env.example 文件"; \
# 	fi \
# else \
# 	echo "✅ .env 文件已存在"; \
# fi
# @echo "✅ 项目初始化完成！"
# @echo ""
# @echo "下一步："
	@echo "1. 编辑 .env 文件配置环境变量"
	@echo "2. 运行 'make deps' 安装依赖"
	@echo "3. 运行 'make dev' 启动开发环境"

# ============================================================================
# 依赖管理
# ============================================================================

# 安装所有依赖包
# 包括Go模块依赖和前端npm包
# 	# @go env -w GOPROXY=https://goproxy.cn,https://goproxy.io,https://mirrors.aliyun.com/goproxy/,direct
deps:
	@echo "📦 安装项目依赖..."
	@echo "安装Go模块依赖..."
	@echo "使用国内Go模块代理..."
	@go env -w GOPROXY=https://goproxy.cn,https://goproxy.io,https://mirrors.aliyun.com/goproxy/,direct
	@go env -w GOSUMDB=sum.golang.google.cn
	@go env -w GO111MODULE=on
	@go mod tidy
	@go mod download
	@echo "安装前端依赖..."
	@if [ -d "$(WEB_DIR)" ]; then \
		npm config set registry https://registry.npmmirror.com/  && \
		cd $(WEB_DIR) && npm install; \
	else \
		echo "⚠️  前端目录不存在，跳过前端依赖安装"; \
	fi
	@echo "✅ 依赖安装完成！"

# ============================================================================
# 开发环境
# ============================================================================

# 启动完整开发环境
# 同时启动前端和后端开发服务器
dev:
	@echo "🔧 启动开发环境..."
	@echo "启动后端开发服务器..."; 
	@go run main.go &
	@echo "启动前端开发服务器..."; 
	@cd $(WEB_DIR) && npm run dev -- --host 0.0.0.0 --port 3000; 

# 仅启动前端开发服务器
# 默认运行在 http://localhost:3000
frontend:
	@echo "🎨 启动前端开发服务器..."
	@if [ -d "$(WEB_DIR)" ]; then \
		cd $(WEB_DIR) && npm run dev; \
	else \
		echo "❌ 前端目录不存在！"; \
		exit 1; \
	fi

# 仅启动后端开发服务器
# 默认运行在 http://localhost:8080
backend:
	@echo "⚙️  启动后端开发服务器..."
	@go run main.go

# 检查开发环境健康状态
health-check:
	@echo "🏥 检查开发环境健康状态..."
	@echo "检查后端服务..."
	@curl -f http://localhost:8080/api/v1/health >/dev/null 2>&1 && \
		echo "✅ 后端服务正常" || echo "❌ 后端服务异常"
	@echo "检查前端服务..."
	@curl -f http://localhost:3000 >/dev/null 2>&1 && \
		echo "✅ 前端服务正常" || echo "❌ 前端服务异常"

# ============================================================================
# 构建相关
# ============================================================================

# 构建完整应用
# 先构建前端，再构建后端
build: build-frontend build-backend
	@echo "✅ 应用构建完成！"
	@echo "可执行文件: $(BIN_DIR)/$(PROJECT_NAME)"
	@echo "前端文件: $(WEB_DIR)/dist/"

# 构建前端静态文件
# 生成生产环境优化的静态资源
build-frontend:
	@echo "🎨 构建前端..."
	@if [ -d "$(WEB_DIR)" ]; then \
		cd $(WEB_DIR) && npm run build; \
		echo "✅ 前端构建完成: $(WEB_DIR)/dist/"; \
	else \
		echo "⚠️  前端目录不存在，跳过前端构建"; \
	fi

# 构建后端可执行文件
# 包含版本信息和构建时间
build-backend:
	@echo "⚙️  构建后端..."
	@mkdir -p $(BIN_DIR)
	@go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(PROJECT_NAME) main.go
	@echo "✅ 后端构建完成: $(BIN_DIR)/$(PROJECT_NAME)"
	@echo "版本信息: $(VERSION)"

# 构建 dsync 命令行客户端
# 版本信息注入 cmd/dsync 包的 main.version/main.buildTime
cli:
	@echo "🛠️  构建 dsync CLI..."
	@mkdir -p $(BIN_DIR)
	@go build -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)" -trimpath -o $(BIN_DIR)/dsync ./cmd/dsync
	@echo "✅ CLI 构建完成: $(BIN_DIR)/dsync"
	@echo "使用帮助: $(BIN_DIR)/dsync --help（详见 docs/dsync-cli.md）"

# 安装 dsync 到 /usr/local/bin（需要 sudo 执行 make cli-install）
cli-install: cli
	@install -m 0755 $(BIN_DIR)/dsync /usr/local/bin/dsync
	@echo "✅ 已安装: /usr/local/bin/dsync"

# 运行构建后的应用
# 需要先执行 make build
run: 
	@echo "🚀 启动应用..."
	@if [ -f "$(BIN_DIR)/$(PROJECT_NAME)" ]; then \
		./$(BIN_DIR)/$(PROJECT_NAME); \
	else \
		echo "❌ 可执行文件不存在，请先运行 'make build'"; \
		exit 1; \
	fi


# ============================================================================
# 测试相关
# ============================================================================

# 运行所有测试
test: test-backend
	@echo "✅ 所有测试完成！"

# 运行后端单元测试
# 包含覆盖率报告
test-backend:
	@echo "🧪 运行后端测试..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✅ 后端测试完成！"
	@echo "覆盖率报告: coverage.html"

# ============================================================================
# 代码质量
# ============================================================================

# 格式化所有代码
# 包括Go代码和前端代码
fmt:
	@echo "🔍 格式化代码..."
	@echo "格式化Go代码..."
	@go fmt ./...
	@goimports -w . 2>/dev/null || true
	@echo "格式化前端代码..."
	@if [ -d "$(WEB_DIR)" ]; then \
		cd $(WEB_DIR) && npm run lint:fix 2>/dev/null || true; \
	fi
	@echo "✅ 代码格式化完成！"

# 运行代码检查
# 使用golangci-lint检查Go代码，ESLint检查前端代码
lint:
	@echo "🔍 运行代码检查..."
	@echo "检查Go代码..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "⚠️  golangci-lint未安装，跳过Go代码检查"; \
		echo "安装方法: https://golangci-lint.run/usage/install/"; \
	fi
	@echo "检查前端代码..."
	@if [ -d "$(WEB_DIR)" ]; then \
		cd $(WEB_DIR) && npm run lint 2>/dev/null || true; \
	fi
	@echo "✅ 代码检查完成！"

# ============================================================================
# 清理相关
# ============================================================================

# 清理所有构建文件和缓存
# 包括二进制文件、前端构建产物、日志等
clean:
	@echo "🧹 清理构建文件..."
	@echo "清理二进制文件..."
	@rm -rf $(BIN_DIR)/
	@echo "清理前端构建产物..."
	@rm -rf $(WEB_DIR)/dist/
	@echo "清理依赖缓存..."
	@rm -rf $(WEB_DIR)/node_modules/
	@echo "清理日志文件..."
	@rm -rf $(LOGS_DIR)/*
	@echo "清理临时文件..."
	@rm -rf $(TEMP_DIR)/*
	@rm -f coverage.out coverage.html
	@echo "清理Docker资源..."
	@docker-compose down --volumes --remove-orphans 2>/dev/null || true
	@echo "✅ 清理完成！"