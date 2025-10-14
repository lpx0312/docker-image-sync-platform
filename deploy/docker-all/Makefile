# ============================================================================
# Docker镜像同步平台 - 完整版部署 Makefile
# ============================================================================
# 
# 使用说明：
# 1. 确保已安装 Docker 和 Docker Compose
# 2. 根据需要配置 .env 文件 (可选，会自动读取项目根目录的 .env)
# 3. 运行相应的 make 命令
# 
# 主要命令：
#   make build          - 构建完整版镜像
#   make up             - 启动服务 (不包含MySQL)
#   make up-mysql       - 启动服务 (包含MySQL)
#   make down           - 停止服务
#   make restart        - 重启服务
#   make logs           - 查看日志
#   make clean          - 清理资源
# 
# ============================================================================

# 默认目标
.DEFAULT_GOAL := help

# 项目配置
PROJECT_NAME := docker-image-sync-platform
IMAGE_NAME := $(PROJECT_NAME):all
CONTAINER_NAME := docker-sync-all
MYSQL_CONTAINER_NAME := docker-sync-mysql

# Docker Compose 文件
COMPOSE_FILE := docker-compose-all.yml
COMPOSE_FILE_MYSQL := docker-compose-all-mysql.yml

# 颜色定义
RED := \033[31m
GREEN := \033[32m
YELLOW := \033[33m
BLUE := \033[34m
MAGENTA := \033[35m
CYAN := \033[36m
WHITE := \033[37m
RESET := \033[0m

# ============================================================================
# 帮助信息
# ============================================================================

.PHONY: help
help: ## 显示帮助信息
	@echo "$(CYAN)============================================================================$(RESET)"
	@echo "$(CYAN)Docker镜像同步平台 - 完整版部署 Makefile$(RESET)"
	@echo "$(CYAN)============================================================================$(RESET)"
	@echo ""
	@echo "$(YELLOW)构建相关:$(RESET)"
	@echo "  $(GREEN)build$(RESET)           构建完整版Docker镜像"
	@echo "  $(GREEN)rebuild$(RESET)         强制重新构建镜像 (不使用缓存)"
	@echo ""
	@echo "$(YELLOW)服务管理:$(RESET)"
	@echo "  $(GREEN)up$(RESET)              启动服务 (不包含MySQL，使用外部数据库)"
	@echo "  $(GREEN)up-mysql$(RESET)        启动服务 (包含MySQL，本地数据库)"
	@echo "  $(GREEN)down$(RESET)            停止所有服务"
	@echo "  $(GREEN)restart$(RESET)         重启服务 (不包含MySQL)"
	@echo "  $(GREEN)restart-mysql$(RESET)   重启服务 (包含MySQL)"
	@echo ""
	@echo "$(YELLOW)监控和调试:$(RESET)"
	@echo "  $(GREEN)status$(RESET)          查看服务状态"
	@echo "  $(GREEN)logs$(RESET)            查看应用日志"
	@echo "  $(GREEN)logs-mysql$(RESET)      查看MySQL日志"
	@echo "  $(GREEN)logs-all$(RESET)        查看所有服务日志"
	@echo "  $(GREEN)shell$(RESET)           进入应用容器Shell"
	@echo "  $(GREEN)mysql-shell$(RESET)     进入MySQL容器Shell"
	@echo ""
	@echo "$(YELLOW)健康检查:$(RESET)"
	@echo "  $(GREEN)health$(RESET)          检查服务健康状态"
	@echo "  $(GREEN)test$(RESET)            运行基本功能测试"
	@echo ""
	@echo "$(YELLOW)清理和维护:$(RESET)"
	@echo "  $(GREEN)clean$(RESET)           清理停止的容器和未使用的镜像"
	@echo "  $(GREEN)clean-all$(RESET)       清理所有相关资源 (危险操作)"
	@echo "  $(GREEN)prune$(RESET)           清理Docker系统缓存"
	@echo ""
	@echo "$(YELLOW)环境配置:$(RESET)"
	@echo "  $(GREEN)env-init$(RESET)        初始化环境配置文件"
	@echo "  $(GREEN)env-check$(RESET)       检查环境配置"
	@echo ""
	@echo "$(CYAN)============================================================================$(RESET)"

# ============================================================================
# 构建相关
# ============================================================================

## @chmod +x build-all.sh
## @./build-all.sh
.PHONY: build
build: ## 构建完整版Docker镜像
	@echo "$(BLUE)🐳 构建完整版Docker镜像...$(RESET)"
	@docker build --no-cache -f Dockerfile-all -t $(IMAGE_NAME) ../../
	@echo "$(GREEN)✅ 镜像构建完成！$(RESET)"

.PHONY: rebuild
rebuild: ## 强制重新构建镜像 (不使用缓存)
	@echo "$(BLUE)🐳 强制重新构建Docker镜像...$(RESET)"
	@docker build --no-cache -f Dockerfile-all -t $(IMAGE_NAME) ../../
	@echo "$(GREEN)✅ 镜像重新构建完成！$(RESET)"

# ============================================================================
# 服务管理
# ============================================================================

.PHONY: up
up: ## 启动服务 (不包含MySQL)
	@echo "$(BLUE)🚀 启动完整版服务 (外部MySQL)...$(RESET)"
	@docker-compose -f $(COMPOSE_FILE) up -d
	@echo "$(GREEN)✅ 服务启动完成！$(RESET)"
	@$(MAKE) _show_urls

.PHONY: up-mysql
up-mysql: ## 启动服务 (包含MySQL)
	@echo "$(BLUE)🚀 启动完整版服务 (包含MySQL)...$(RESET)"
	@docker-compose -f $(COMPOSE_FILE_MYSQL) up -d
	@echo "$(GREEN)✅ 服务启动完成！$(RESET)"
	@$(MAKE) _show_urls_mysql

.PHONY: down
down: ## 停止所有服务
	@echo "$(BLUE)🛑 停止完整版服务...$(RESET)"
	@docker-compose -f $(COMPOSE_FILE) down 2>/dev/null || true
	@docker-compose -f $(COMPOSE_FILE_MYSQL) down 2>/dev/null || true
	@echo "$(GREEN)✅ 服务已停止！$(RESET)"

.PHONY: restart
restart: down up ## 重启服务 (不包含MySQL)

.PHONY: restart-mysql
restart-mysql: down up-mysql ## 重启服务 (包含MySQL)

# ============================================================================
# 监控和调试
# ============================================================================

.PHONY: status
status: ## 查看服务状态
	@echo "$(BLUE)📊 服务状态:$(RESET)"
	@docker ps --filter "name=$(CONTAINER_NAME)" --filter "name=$(MYSQL_CONTAINER_NAME)" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

.PHONY: logs
logs: ## 查看应用日志
	@echo "$(BLUE)📋 应用日志:$(RESET)"
	@docker-compose -f $(COMPOSE_FILE) logs -f app-all 2>/dev/null || \
	 docker-compose -f $(COMPOSE_FILE_MYSQL) logs -f app-all 2>/dev/null || \
	 echo "$(RED)❌ 服务未运行$(RESET)"

.PHONY: logs-mysql
logs-mysql: ## 查看MySQL日志
	@echo "$(BLUE)📋 MySQL日志:$(RESET)"
	@docker-compose -f $(COMPOSE_FILE_MYSQL) logs -f mysql 2>/dev/null || \
	 echo "$(RED)❌ MySQL服务未运行$(RESET)"

.PHONY: logs-all
logs-all: ## 查看所有服务日志
	@echo "$(BLUE)📋 所有服务日志:$(RESET)"
	@docker-compose -f $(COMPOSE_FILE) logs -f 2>/dev/null || \
	 docker-compose -f $(COMPOSE_FILE_MYSQL) logs -f 2>/dev/null || \
	 echo "$(RED)❌ 服务未运行$(RESET)"

.PHONY: shell
shell: ## 进入应用容器Shell
	@echo "$(BLUE)🐚 进入应用容器...$(RESET)"
	@docker exec -it $(CONTAINER_NAME) /bin/sh || \
	 echo "$(RED)❌ 容器未运行，请先启动服务$(RESET)"

.PHONY: mysql-shell
mysql-shell: ## 进入MySQL容器Shell
	@echo "$(BLUE)🐚 进入MySQL容器...$(RESET)"
	@docker exec -it $(MYSQL_CONTAINER_NAME) mysql -uroot -p || \
	 echo "$(RED)❌ MySQL容器未运行，请先启动MySQL服务$(RESET)"

# ============================================================================
# 健康检查
# ============================================================================

.PHONY: health
health: ## 检查服务健康状态
	@echo "$(BLUE)🏥 检查服务健康状态...$(RESET)"
	@echo "$(YELLOW)应用健康检查:$(RESET)"
	@curl -s http://localhost/health | jq . 2>/dev/null || \
	 curl -s http://localhost/health || \
	 echo "$(RED)❌ 健康检查失败$(RESET)"
	@echo ""
	@echo "$(YELLOW)容器状态:$(RESET)"
	@docker ps --filter "name=$(CONTAINER_NAME)" --filter "name=$(MYSQL_CONTAINER_NAME)" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

.PHONY: test
test: ## 运行基本功能测试
	@echo "$(BLUE)🧪 运行基本功能测试...$(RESET)"
	@echo "$(YELLOW)测试前端访问:$(RESET)"
	@curl -s -o /dev/null -w "前端页面: %{http_code}\n" http://localhost/ || echo "$(RED)❌ 前端访问失败$(RESET)"
	@echo "$(YELLOW)测试API健康检查:$(RESET)"
	@curl -s -o /dev/null -w "健康检查: %{http_code}\n" http://localhost/health || echo "$(RED)❌ API访问失败$(RESET)"
	@echo "$(YELLOW)测试API接口:$(RESET)"
	@curl -s -o /dev/null -w "API接口: %{http_code}\n" http://localhost/api/v1/health || echo "$(RED)❌ API接口访问失败$(RESET)"

# ============================================================================
# 清理和维护
# ============================================================================

.PHONY: clean
clean: ## 清理停止的容器和未使用的镜像
	@echo "$(BLUE)🧹 清理资源...$(RESET)"
	@docker container prune -f
	@docker image prune -f
	@echo "$(GREEN)✅ 清理完成！$(RESET)"

.PHONY: clean-all
clean-all: ## 清理所有相关资源 (危险操作)
	@echo "$(RED)⚠️  警告: 这将删除所有相关的容器、镜像和数据卷！$(RESET)"
	@read -p "确定要继续吗? (y/N): " confirm && [ "$$confirm" = "y" ] || exit 1
	@echo "$(BLUE)🧹 清理所有资源...$(RESET)"
	@$(MAKE) down
	@docker rm -f $(CONTAINER_NAME) $(MYSQL_CONTAINER_NAME) 2>/dev/null || true
	@docker rmi $(IMAGE_NAME) 2>/dev/null || true
	@docker volume rm docker-all_mysql_data 2>/dev/null || true
	@docker network rm docker-all_app-network 2>/dev/null || true
	@echo "$(GREEN)✅ 所有资源已清理！$(RESET)"

.PHONY: prune
prune: ## 清理Docker系统缓存
	@echo "$(BLUE)🧹 清理Docker系统缓存...$(RESET)"
	@docker system prune -f
	@docker builder prune -f
	@echo "$(GREEN)✅ 系统缓存清理完成！$(RESET)"

# ============================================================================
# 环境配置
# ============================================================================

.PHONY: env-init
env-init: ## 初始化环境配置文件
	@echo "$(BLUE)⚙️  初始化环境配置...$(RESET)"
	@if [ ! -f .env ]; then \
		cp env-example .env; \
		echo "$(GREEN)✅ 已创建 .env 文件，请根据需要修改配置$(RESET)"; \
	else \
		echo "$(YELLOW)⚠️  .env 文件已存在$(RESET)"; \
	fi

.PHONY: env-check
env-check: ## 检查环境配置
	@echo "$(BLUE)🔍 检查环境配置...$(RESET)"
	@echo "$(YELLOW)Docker版本:$(RESET)"
	@docker --version
	@echo "$(YELLOW)Docker Compose版本:$(RESET)"
	@docker-compose --version
	@echo "$(YELLOW)环境变量文件:$(RESET)"
	@if [ -f .env ]; then \
		echo "$(GREEN)✅ .env 文件存在$(RESET)"; \
	else \
		echo "$(YELLOW)⚠️  .env 文件不存在，将使用默认配置$(RESET)"; \
	fi
	@if [ -f ../../.env ]; then \
		echo "$(GREEN)✅ 项目根目录 .env 文件存在$(RESET)"; \
	else \
		echo "$(YELLOW)⚠️  项目根目录 .env 文件不存在$(RESET)"; \
	fi

# ============================================================================
# 内部辅助函数
# ============================================================================

.PHONY: _show_urls
_show_urls:
	@echo ""
	@echo "$(CYAN)🌐 服务访问地址:$(RESET)"
	@echo "  $(GREEN)前端界面:$(RESET) http://localhost"
	@echo "  $(GREEN)后端API:$(RESET)  http://localhost:8080/api"
	@echo "  $(GREEN)健康检查:$(RESET) http://localhost/health"
	@echo ""

.PHONY: _show_urls_mysql
_show_urls_mysql:
	@$(MAKE) _show_urls
	@echo "  $(GREEN)MySQL:$(RESET)     localhost:3306"
	@echo ""

# ============================================================================
# 快捷命令别名
# ============================================================================

.PHONY: start
start: up ## 启动服务的别名

.PHONY: start-mysql
start-mysql: up-mysql ## 启动MySQL服务的别名

.PHONY: stop
stop: down ## 停止服务的别名

.PHONY: log
log: logs ## 查看日志的别名

.PHONY: ps
ps: status ## 查看状态的别名