.PHONY: build run test clean wire docker-up docker-down migrate-up migrate-down lint

# 构建所有服务
build:
	go build -o bin/api-brand ./cmd/api-brand/
	go build -o bin/api-app ./cmd/api-app/
	go build -o bin/api-admin ./cmd/api-admin/

# 运行单个服务（本地开发）
run-brand:
	go run ./cmd/api-brand/

run-app:
	go run ./cmd/api-app/

# 	CONFIG_PATH=configs/config-admin.yaml go run ./cmd/api-admin/
# 	CONFIG_PATH=configs/config-brand.yaml go run ./cmd/api-brand/
run-admin:
	go run ./cmd/api-admin/

# 测试
test:
	go test -v -race -coverprofile=coverage.out ./...

# 清理
clean:
	rm -rf bin/ coverage.out

# 生成 Wire 依赖注入代码
wire:
	cd cmd/api-brand && wire
	cd cmd/api-app && wire
	cd cmd/api-admin && wire

# Docker 启动所有服务
docker-up:
	docker-compose up -d --build

# Docker 停止所有服务
docker-down:
	docker-compose down

# Batch 4.5：用 DATABASE_URL 兜底；本地开发未设时 fallback 到当前 OS user，
# 避免 Batch 4 那次"硬编码 postgres role 与开发机用户不一致 → make migrate-up 静默 no-op"的坑。
# 显式覆盖：DATABASE_URL=postgres://foo:bar@host/db make migrate-up
DATABASE_URL ?= postgres://$(or $(PG_USER),$(USER))@127.0.0.1:5432/mini_schedule?sslmode=disable

# 数据库迁移（向上）
migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

# 数据库迁移（向下）
migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down 1

# Lint
lint:
	golangci-lint run ./...
