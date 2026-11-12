# cronApi 常用命令
APP      := cronApi
BIN      := bin/$(APP)
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
TEST_LDFLAGS ?=

.PHONY: help deps check run build test test-race lint tidy fmt db-init db-schema redis-start db-down psql redis-cli

help: ## 显示所有命令
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

deps: ## 检查本机依赖（Go/MySQL/Redis）是否就绪
	@printf "Go     : %s\n" "$$(go version 2>/dev/null || echo '未安装')"
	@printf "MySQL  : %s\n" "$$(nc -z 127.0.0.1 3306 2>/dev/null && echo '3306 已监听 OK' || echo '3306 未启动，请先启动 MySQL')"
	@printf "Redis  : %s\n" "$$(nc -z 127.0.0.1 6379 2>/dev/null && echo '6379 已监听 OK' || echo '6379 未启动，可设为 redis.enabled=false 或执行 make redis-start')"

check: deps ## 检查依赖并做编译检查
	@go build ./... && echo "编译检查通过"

db-init: ## 创建 cron_api 库和 3 张表（读取 deploy/mysql/schema.sql，root 密码用 -p 输入）
	mysql -h127.0.0.1 -P3306 -uroot -p < deploy/mysql/schema.sql

db-schema: ## 只打印建表语句，不执行
	@cat deploy/mysql/schema.sql

redis-start: ## 后台启动本机 Redis（无持久化，仅开发用）
	@redis-server --port 6379 --daemonize yes --save '' --appendonly no && echo "Redis 已启动 (6379)"

redis-stop: ## 停止本机 Redis
	@redis-cli -p 6379 shutdown nosave 2>/dev/null; echo "Redis 已停止"

db-down: redis-stop ## 停止本机 Redis

run: ## 本地运行（使用 configs/config.local.yaml）
	go run ./cmd/server -c configs/config.local.yaml

build: ## 编译到 bin/
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server

test: ## 单元测试
	go test -count=1 $(TEST_LDFLAGS) ./...

test-race: ## 竞态检测
	go test -count=1 -race $(TEST_LDFLAGS) ./...

fmt: ## 格式化
	goimports -w -local cronApi cmd internal pkg

tidy: ## 整理依赖
	go mod tidy

lint: ## vet + gofmt 检查
	go vet ./...
	@out=$$(gofmt -l cmd internal pkg); if [ -n "$$out" ]; then echo "未格式化: $$out"; exit 1; fi

psql: ## 进入本机 MySQL 的 cron_api 库
	mysql -h127.0.0.1 -P3306 -uroot -p cron_api

redis-cli: ## 进入本机 Redis 命令行
	redis-cli -p 6379
