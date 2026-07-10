# ============================================================================
# LingFlow - 项目构建与管理 Makefile
# ============================================================================
# 模块名称: ling_flow
# 语言: Go 1.26.1 + Vue 3 (Bun + Vite)
# 部署目标: AWS Lambda
# ============================================================================

# ----------------------------------------------------------------------------
# 加载环境变量（如果 .env 文件存在）
# ----------------------------------------------------------------------------
-include .env
export

# ----------------------------------------------------------------------------
# 项目变量
# ----------------------------------------------------------------------------
APP_NAME    = LingFlow
GO          = go
BUN         = bun
GOFMT       = gofmt
BINARY      = LingFlow
DEMO_DIR    = demo
BUILD_DIR   = build
LAMBDA_ZIP  = $(BUILD_DIR)/lambda.zip
GOFLAGS     = -v
LDFLAGS     = -s -w

# ----------------------------------------------------------------------------
# ANSI 颜色代码
# ----------------------------------------------------------------------------
GREEN   = \033[0;32m
RED     = \033[0;31m
YELLOW  = \033[0;33m
BLUE    = \033[0;34m
NC      = \033[0m

# ============================================================================
# 默认目标：显示帮助信息
# ============================================================================
.DEFAULT_GOAL := help

help: ## 显示所有可用目标及说明
	@printf "\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  $(APP_NAME) - 项目构建与管理系统$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(YELLOW)可用目标:$(NC)\n"
	@printf "\n"
	@printf "  $(GREEN)运行相关:$(NC)\n"
	@printf "    make %-20s %s\n" "run"           "同时启动后端与前端开发服务器"
	@printf "    make %-20s %s\n" "run-backend"   "仅启动 Go 后端服务"
	@printf "    make %-20s %s\n" "run-frontend"  "仅启动前端开发服务器"
	@printf "\n"
	@printf "  $(GREEN)构建相关:$(NC)\n"
	@printf "    make %-20s %s\n" "build"          "完整构建（后端 + 前端）"
	@printf "    make %-20s %s\n" "build-backend"  "仅构建 Go 二进制文件"
	@printf "    make %-20s %s\n" "build-frontend" "仅构建前端静态资源"
	@printf "    make %-20s %s\n" "build-lambda"   "构建 AWS Lambda 部署包"
	@printf "\n"
	@printf "  $(GREEN)代码质量:$(NC)\n"
	@printf "    make %-20s %s\n" "format"         "格式化所有代码（Go + 前端）"
	@printf "    make %-20s %s\n" "lint"           "运行 Go 静态分析检查"
	@printf "    make %-20s %s\n" "test"           "运行所有测试"
	@printf "    make %-20s %s\n" "test-coverage"  "运行测试并生成覆盖率报告"
	@printf "\n"
	@printf "  $(GREEN)依赖管理:$(NC)\n"
	@printf "    make %-20s %s\n" "deps"           "安装所有依赖"
	@printf "    make %-20s %s\n" "deps-update"    "更新所有依赖到最新版本"
	@printf "\n"
	@printf "  $(GREEN)部署相关:$(NC)\n"
	@printf "    make %-20s %s\n" "deploy"         "部署到 AWS Lambda"
	@printf "    make %-20s %s\n" "deploy-check"   "检查 AWS 部署前置条件"
	@printf "    make %-20s %s\n" "env-check"      "验证 .env 环境变量配置"
	@printf "\n"
	@printf "  $(GREEN)其他:$(NC)\n"
	@printf "    make %-20s %s\n" "clean"          "清理所有构建产物与缓存"
	@printf "    make %-20s %s\n" "docker-build"   "构建 Docker 镜像"
	@printf "    make %-20s %s\n" "help"           "显示此帮助信息"
	@printf "\n"

# ============================================================================
# 运行目标
# ============================================================================

run: ## 同时启动后端与前端开发服务器
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  启动 $(APP_NAME) 全栈开发环境$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)▶ 正在启动 Go 后端服务...$(NC)\n"
	@printf "$(GREEN)▶ 正在启动 Vue 前端开发服务器...$(NC)\n"
	@printf "$(YELLOW)提示: 按 Ctrl+C 停止所有服务$(NC)\n"
	@printf "\n"
	@trap 'printf "\n$(YELLOW)正在停止所有服务...$(NC)\n"; kill 0; exit 0' INT TERM; \
	$(GO) run . & \
	(cd $(DEMO_DIR) && $(BUN) dev) & \
	wait

run-backend: ## 仅启动 Go 后端服务
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  启动 $(APP_NAME) 后端服务$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)▶ 正在启动 Go 后端...$(NC)\n"
	$(GO) run .

run-frontend: ## 仅启动前端开发服务器
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  启动 $(APP_NAME) 前端开发服务器$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)▶ 正在安装前端依赖...$(NC)\n"
	cd $(DEMO_DIR) && $(BUN) install
	@printf "$(GREEN)▶ 正在启动 Vite 开发服务器...$(NC)\n"
	cd $(DEMO_DIR) && $(BUN) dev

# ============================================================================
# 构建目标
# ============================================================================

build: build-backend build-frontend ## 完整构建（后端 + 前端）
	@printf "\n"
	@printf "$(GREEN)============================================================================$(NC)\n"
	@printf "$(GREEN)  构建完成！$(NC)\n"
	@printf "$(GREEN)============================================================================$(NC)\n"
	@printf "\n"
	@printf "  $(BLUE)二进制文件:$(NC) $(BUILD_DIR)/$(BINARY)\n"
	@if [ -f "$(BUILD_DIR)/$(BINARY)" ]; then \
		SIZE=$$(du -h "$(BUILD_DIR)/$(BINARY)" | cut -f1); \
		printf "  $(BLUE)文件大小:$(NC)   $$SIZE\n"; \
	fi
	@printf "  $(BLUE)前端产物:$(NC)   $(DEMO_DIR)/dist/\n"
	@printf "\n"

build-backend: ## 仅构建 Go 二进制文件
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  构建 Go 后端二进制文件$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@mkdir -p $(BUILD_DIR)
	@printf "$(GREEN)▶ 正在编译 Go 项目...$(NC)\n"
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) .
	@printf "\n$(GREEN)✓ 后端构建成功: $(BUILD_DIR)/$(BINARY)$(NC)\n"

build-frontend: ## 仅构建前端静态资源
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  构建前端静态资源$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)▶ 正在安装前端依赖...$(NC)\n"
	cd $(DEMO_DIR) && $(BUN) install
	@printf "$(GREEN)▶ 正在构建前端项目...$(NC)\n"
	cd $(DEMO_DIR) && $(BUN) run build
	@printf "\n$(GREEN)✓ 前端构建成功: $(DEMO_DIR)/dist/$(NC)\n"

build-lambda: ## 构建 AWS Lambda 部署包
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  构建 AWS Lambda 部署包$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@mkdir -p $(BUILD_DIR)
	@printf "$(GREEN)▶ 正在为 Linux/amd64 交叉编译...$(NC)\n"
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/bootstrap .
	@printf "\n$(GREEN)✓ Lambda 二进制文件构建成功: $(BUILD_DIR)/bootstrap$(NC)\n"

# ============================================================================
# 清理目标
# ============================================================================

clean: ## 清理所有构建产物与缓存
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  清理项目构建产物与缓存$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(YELLOW)▶ 清理 Go 构建缓存...$(NC)\n"
	-@$(GO) clean -cache 2>/dev/null
	@printf "$(YELLOW)▶ 清理 Go 测试缓存...$(NC)\n"
	-@$(GO) clean -testcache 2>/dev/null
	@printf "$(YELLOW)▶ 删除构建目录 $(BUILD_DIR)/...$(NC)\n"
	-@rm -rf $(BUILD_DIR)
	@printf "$(YELLOW)▶ 删除二进制文件 $(BINARY)...$(NC)\n"
	-@rm -f $(BINARY)
	@printf "$(YELLOW)▶ 删除前端依赖 $(DEMO_DIR)/node_modules/...$(NC)\n"
	-@rm -rf $(DEMO_DIR)/node_modules
	@printf "$(YELLOW)▶ 删除前端构建产物 $(DEMO_DIR)/dist/...$(NC)\n"
	-@rm -rf $(DEMO_DIR)/dist
	@printf "$(YELLOW)▶ 删除 Nuxt 缓存 $(DEMO_DIR)/.nuxt/...$(NC)\n"
	-@rm -rf $(DEMO_DIR)/.nuxt
	@printf "$(YELLOW)▶ 删除 Nuxt 输出 $(DEMO_DIR)/.output/...$(NC)\n"
	-@rm -rf $(DEMO_DIR)/.output
	@printf "$(YELLOW)▶ 删除调试二进制文件 __debug_bin*...$(NC)\n"
	-@rm -f __debug_bin*
	@printf "$(YELLOW)▶ 删除日志文件 *.log...$(NC)\n"
	-@rm -f *.log
	@printf "\n$(GREEN)✓ 清理完成！所有构建产物与缓存已移除。$(NC)\n\n"

# ============================================================================
# 代码质量目标
# ============================================================================

format: ## 格式化所有代码（Go + 前端）
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  格式化项目代码$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)▶ 格式化 Go 代码...$(NC)\n"
	$(GOFMT) -w .
	@printf "$(GREEN)✓ Go 代码格式化完成$(NC)\n"
	@if command -v prettier >/dev/null 2>&1; then \
		printf "$(GREEN)▶ 格式化前端代码（Prettier）...$(NC)\n"; \
		prettier --write "$(DEMO_DIR)/src/**/*.{js,ts,vue}" 2>/dev/null || true; \
		printf "$(GREEN)✓ 前端代码格式化完成$(NC)\n"; \
	else \
		printf "$(YELLOW)⚠ 未检测到 Prettier，跳过前端代码格式化$(NC)\n"; \
		printf "$(YELLOW)  安装方式: bun add -g prettier$(NC)\n"; \
	fi
	@printf "\n$(GREEN)✓ 代码格式化全部完成$(NC)\n\n"

lint: ## 运行 Go 静态分析检查
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  运行 Go 静态分析检查$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)▶ 执行 go vet...$(NC)\n"
	$(GO) vet ./...
	@printf "\n$(GREEN)✓ 静态分析检查通过$(NC)\n\n"

test: ## 运行所有测试
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  运行项目测试$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)▶ 执行 Go 测试...$(NC)\n"
	$(GO) test ./... -v
	@printf "\n$(GREEN)✓ 所有测试执行完毕$(NC)\n\n"

test-coverage: ## 运行测试并生成覆盖率报告
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  运行测试并生成覆盖率报告$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@mkdir -p $(BUILD_DIR)
	@printf "$(GREEN)▶ 执行测试并收集覆盖率数据...$(NC)\n"
	$(GO) test ./... -v -coverprofile=$(BUILD_DIR)/coverage.out
	@printf "\n$(GREEN)▶ 生成覆盖率报告...$(NC)\n"
	$(GO) tool cover -func=$(BUILD_DIR)/coverage.out
	@printf "\n$(GREEN)✓ 覆盖率报告已生成: $(BUILD_DIR)/coverage.out$(NC)\n"
	@printf "$(BLUE)  提示: 运行 'go tool cover -html=$(BUILD_DIR)/coverage.out' 可在浏览器中查看详细报告$(NC)\n\n"

# ============================================================================
# 依赖管理目标
# ============================================================================

deps: ## 安装所有依赖
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  安装项目依赖$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)▶ 下载 Go 模块依赖...$(NC)\n"
	$(GO) mod download
	@printf "$(GREEN)▶ 整理 Go 模块依赖...$(NC)\n"
	$(GO) mod tidy
	@printf "$(GREEN)✓ Go 依赖安装完成$(NC)\n\n"
	@printf "$(GREEN)▶ 安装前端依赖...$(NC)\n"
	cd $(DEMO_DIR) && $(BUN) install
	@printf "\n$(GREEN)✓ 所有依赖安装完成$(NC)\n\n"

deps-update: ## 更新所有依赖到最新版本
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  更新项目依赖$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@printf "$(GREEN)▶ 更新 Go 模块依赖...$(NC)\n"
	$(GO) get -u ./...
	$(GO) mod tidy
	@printf "$(GREEN)✓ Go 依赖更新完成$(NC)\n\n"
	@printf "$(GREEN)▶ 更新前端依赖...$(NC)\n"
	cd $(DEMO_DIR) && $(BUN) update
	@printf "\n$(GREEN)✓ 所有依赖更新完成$(NC)\n\n"

# ============================================================================
# 部署目标
# ============================================================================

deploy: build-lambda ## 部署到 AWS Lambda
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  部署 $(APP_NAME) 到 AWS Lambda$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@# 检查 AWS CLI 是否已安装
	@which aws >/dev/null 2>&1 || \
		(printf "$(RED)✗ 错误: 未检测到 AWS CLI，请先安装$(NC)\n" && \
		 printf "$(YELLOW)  安装指南: https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html$(NC)\n" && \
		 exit 1)
	@# 检查 AWS 凭证是否已配置
	@aws sts get-caller-identity >/dev/null 2>&1 || \
		(printf "$(RED)✗ 错误: AWS 凭证未配置或已过期$(NC)\n" && \
		 printf "$(YELLOW)  请检查 .env 文件中的 AWS_ACCESS_KEY_ID 和 AWS_SECRET_ACCESS_KEY$(NC)\n" && \
		 exit 1)
	@printf "$(GREEN)▶ 正在打包 Lambda 部署文件...$(NC)\n"
	@mkdir -p $(BUILD_DIR)
	cd $(BUILD_DIR) && zip -j lambda.zip bootstrap
	@printf "$(GREEN)▶ 正在上传并更新 Lambda 函数代码...$(NC)\n"
	aws lambda update-function-code \
		--function-name $(APP_NAME) \
		--zip-file fileb://$(LAMBDA_ZIP) \
		--region $(AWS_REGION)
	@printf "\n"
	@printf "$(GREEN)============================================================================$(NC)\n"
	@printf "$(GREEN)  ✓ 部署成功！$(NC)\n"
	@printf "$(GREEN)============================================================================$(NC)\n"
	@printf "\n"
	@printf "  $(BLUE)函数名称:$(NC) $(APP_NAME)\n"
	@printf "  $(BLUE)部署区域:$(NC) $(AWS_REGION)\n"
	@printf "  $(BLUE)部署文件:$(NC) $(LAMBDA_ZIP)\n"
	@printf "\n"

deploy-check: ## 检查 AWS 部署前置条件
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  检查 AWS 部署前置条件$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@# 检查 AWS CLI 版本
	@printf "$(BLUE)▶ AWS CLI 版本:$(NC) "
	@if command -v aws >/dev/null 2>&1; then \
		aws --version 2>&1; \
		printf "  $(GREEN)✓ AWS CLI 已安装$(NC)\n"; \
	else \
		printf "\n  $(RED)✗ AWS CLI 未安装$(NC)\n"; \
	fi
	@printf "\n"
	@# 检查 AWS 身份认证
	@printf "$(BLUE)▶ AWS 身份认证:$(NC)\n"
	@if aws sts get-caller-identity >/dev/null 2>&1; then \
		aws sts get-caller-identity 2>&1; \
		printf "  $(GREEN)✓ AWS 凭证有效$(NC)\n"; \
	else \
		printf "  $(RED)✗ AWS 凭证无效或未配置$(NC)\n"; \
	fi
	@printf "\n"
	@# 检查 S3 存储桶访问
	@printf "$(BLUE)▶ S3 存储桶访问:$(NC)\n"
	@if [ -n "$(AWS_SKILLS_S3_BUCKET)" ]; then \
		if aws s3 ls --region $(AWS_REGION) >/dev/null 2>&1; then \
			printf "  $(GREEN)✓ S3 存储桶可访问$(NC)\n"; \
		else \
			printf "  $(RED)✗ S3 存储桶访问失败$(NC)\n"; \
		fi; \
	else \
		printf "  $(YELLOW)⚠ 未配置 AWS_SKILLS_S3_BUCKET$(NC)\n"; \
	fi
	@printf "\n"
	@# 检查 Bedrock 模型访问
	@printf "$(BLUE)▶ Bedrock 模型访问:$(NC)\n"
	@if [ -n "$(AWS_BEDROCK_MODEL_ID)" ]; then \
		printf "  模型 ID: $(AWS_BEDROCK_MODEL_ID)\n"; \
		printf "  区域:    $(AWS_BEDROCK_REGION)\n"; \
		if aws bedrock list-foundation-models --region $(AWS_BEDROCK_REGION) --max-results 1 >/dev/null 2>&1; then \
			printf "  $(GREEN)✓ Bedrock 服务可访问$(NC)\n"; \
		else \
			printf "  $(YELLOW)⚠ Bedrock 服务访问失败（可能需要额外权限）$(NC)\n"; \
		fi; \
	else \
		printf "  $(YELLOW)⚠ 未配置 AWS_BEDROCK_MODEL_ID$(NC)\n"; \
	fi
	@printf "\n"

env-check: ## 验证 .env 环境变量配置
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  验证 .env 环境变量配置$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@PASS=0; FAIL=0; \
	if [ -n "$(AWS_ACCESS_KEY_ID)" ]; then \
		printf "  $(GREEN)✓$(NC) AWS_ACCESS_KEY_ID        已配置\n"; \
		PASS=$$((PASS + 1)); \
	else \
		printf "  $(RED)✗$(NC) AWS_ACCESS_KEY_ID        $(RED)未配置$(NC)\n"; \
		FAIL=$$((FAIL + 1)); \
	fi; \
	if [ -n "$(AWS_SECRET_ACCESS_KEY)" ]; then \
		printf "  $(GREEN)✓$(NC) AWS_SECRET_ACCESS_KEY    已配置\n"; \
		PASS=$$((PASS + 1)); \
	else \
		printf "  $(RED)✗$(NC) AWS_SECRET_ACCESS_KEY    $(RED)未配置$(NC)\n"; \
		FAIL=$$((FAIL + 1)); \
	fi; \
	if [ -n "$(AWS_REGION)" ]; then \
		printf "  $(GREEN)✓$(NC) AWS_REGION               已配置 ($(AWS_REGION))\n"; \
		PASS=$$((PASS + 1)); \
	else \
		printf "  $(RED)✗$(NC) AWS_REGION               $(RED)未配置$(NC)\n"; \
		FAIL=$$((FAIL + 1)); \
	fi; \
	if [ -n "$(AWS_BEDROCK_REGION)" ]; then \
		printf "  $(GREEN)✓$(NC) AWS_BEDROCK_REGION       已配置 ($(AWS_BEDROCK_REGION))\n"; \
		PASS=$$((PASS + 1)); \
	else \
		printf "  $(RED)✗$(NC) AWS_BEDROCK_REGION       $(RED)未配置$(NC)\n"; \
		FAIL=$$((FAIL + 1)); \
	fi; \
	if [ -n "$(AWS_BEDROCK_MODEL_ID)" ]; then \
		printf "  $(GREEN)✓$(NC) AWS_BEDROCK_MODEL_ID    已配置 ($(AWS_BEDROCK_MODEL_ID))\n"; \
		PASS=$$((PASS + 1)); \
	else \
		printf "  $(RED)✗$(NC) AWS_BEDROCK_MODEL_ID    $(RED)未配置$(NC)\n"; \
		FAIL=$$((FAIL + 1)); \
	fi; \
	printf "\n"; \
	printf "  $(BLUE)检查结果:$(NC) $$PASS 项通过"; \
	if [ $$FAIL -gt 0 ]; then \
		printf ", $(RED)$$FAIL 项缺失$(NC)"; \
	fi; \
	printf "\n\n"; \
	if [ $$FAIL -gt 0 ]; then \
		printf "  $(YELLOW)请检查项目根目录下的 .env 文件，补充缺失的环境变量。$(NC)\n\n"; \
		exit 1; \
	fi

# ============================================================================
# Docker 目标
# ============================================================================

docker-build: ## 构建 Docker 镜像
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "$(BLUE)  构建 Docker 镜像$(NC)\n"
	@printf "$(BLUE)============================================================================$(NC)\n"
	@printf "\n"
	@if [ -f "Dockerfile" ]; then \
		printf "$(GREEN)▶ 正在构建 Docker 镜像 $(APP_NAME)...$(NC)\n"; \
		docker build -t $(APP_NAME):latest .; \
		printf "\n$(GREEN)✓ Docker 镜像构建成功: $(APP_NAME):latest$(NC)\n\n"; \
	else \
		printf "$(YELLOW)⚠ 未找到 Dockerfile$(NC)\n"; \
		printf "$(YELLOW)  请在项目根目录创建 Dockerfile 后再执行此命令。$(NC)\n\n"; \
	fi

# ============================================================================
# 伪目标声明
# ============================================================================
.PHONY: help \
        run run-backend run-frontend \
        build build-backend build-frontend build-lambda \
        clean \
        format lint test test-coverage \
        deps deps-update \
        deploy deploy-check env-check \
        docker-build
