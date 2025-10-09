# Docker镜像同步平台 Makefile

.PHONY: help build run dev test clean docker-build docker-run docker-stop frontend backend deps

# 默认目标
help:
	@echo "Docker镜像同步平台 - 可用命令:"
	@echo ""
	@echo "开发相关:"
	@echo "  dev          - 启动开发环境"
	@echo "  frontend     - 启动前端开发服务器"
	@echo "  backend      - 启动后端开发服务器"
	@echo "  deps         - 安装所有依赖"
	@echo ""
	@echo "构建相关:"
	@echo "  build        - 构建应用"
	@echo "  build-frontend - 构建前端"
	@echo "  build-backend  - 构建后端"
	@echo ""
	@echo "Docker相关:"
	@echo "  docker-build - 构建Docker镜像"
	@echo "  docker-run   - 运行Docker容器"
	@echo "  docker-stop  - 停止Docker容器"
	@echo "  docker-logs  - 查看Docker日志"
	@echo ""
	@echo "测试相关:"
	@echo "  test         - 运行测试"
	@echo "  test-backend - 运行后端测试"
	@echo ""
	@echo "其他:"
	@echo "  clean        - 清理构建文件"
	@echo "  fmt          - 格式化代码"
	@echo "  lint         - 代码检查"

# 开发环境
dev:
	@echo "启动开发环境..."
	@./dev.sh

# 安装依赖
deps:
	@echo "安装Go依赖..."
	@go mod tidy
	@echo "安装前端依赖..."
	@cd web && npm install

# 构建
build: build-frontend build-backend

build-frontend:
	@echo "构建前端..."
	@cd web && npm run build

build-backend:
	@echo "构建后端..."
	@go build -o bin/docker-sync-platform main.go

# 运行
run: build
	@echo "启动应用..."
	@./bin/docker-sync-platform

# 前端开发服务器
frontend:
	@echo "启动前端开发服务器..."
	@cd web && npm run dev

# 后端开发服务器
backend:
	@echo "启动后端开发服务器..."
	@go run main.go

# Docker相关
docker-build:
	@echo "构建Docker镜像..."
	@docker build -t docker-sync-platform .

docker-run:
	@echo "启动Docker容器..."
	@docker-compose up -d

docker-stop:
	@echo "停止Docker容器..."
	@docker-compose down

docker-logs:
	@echo "查看Docker日志..."
	@docker-compose logs -f

# 测试
test: test-backend

test-backend:
	@echo "运行后端测试..."
	@go test ./...

# 代码格式化
fmt:
	@echo "格式化Go代码..."
	@go fmt ./...
	@echo "格式化前端代码..."
	@cd web && npm run lint:fix

# 代码检查
lint:
	@echo "检查Go代码..."
	@golangci-lint run
	@echo "检查前端代码..."
	@cd web && npm run lint

# 清理
clean:
	@echo "清理构建文件..."
	@rm -rf bin/
	@rm -rf web/dist/
	@rm -rf web/node_modules/
	@rm -rf logs/*
	@docker-compose down --volumes --remove-orphans 2>/dev/null || true

# 创建必要的目录
init:
	@echo "初始化项目目录..."
	@mkdir -p bin logs git_repo
	@cp .env.example .env 2>/dev/null || true
	@echo "请编辑 .env 文件配置您的环境变量"