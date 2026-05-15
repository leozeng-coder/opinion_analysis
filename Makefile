.PHONY: dev-backend dev-frontend dev install-frontend tidy docker-up docker-down

# 本地开发
dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

dev:
	@echo "启动前后端开发服务..."
	@$(MAKE) dev-backend & $(MAKE) dev-frontend

# 依赖管理
tidy:
	cd backend && go mod tidy

install-frontend:
	cd frontend && npm install

# Docker
docker-up:
	docker-compose up -d --build

docker-down:
	docker-compose down

# 构建
build-backend:
	cd backend && go build -o bin/server ./cmd/server

build-frontend:
	cd frontend && npm run build

# 数据库迁移（由 Go 启动时自动迁移）
migrate:
	cd backend && go run ./cmd/server

# 清理
clean:
	rm -rf backend/bin frontend/dist
