.PHONY: dev-backend dev-frontend dev-admin dev install-frontend install-admin install tidy docker-up docker-down

# 本地开发
dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

dev-admin:
	cd frontend-admin && npm run dev

dev:
	@echo "Start backend, frontend, and admin in separate terminals:"
	@echo "  make dev-backend"
	@echo "  make dev-frontend"
	@echo "  make dev-admin"

# 依赖管理
tidy:
	cd backend && go mod tidy

install-frontend:
	cd frontend && npm install

install-admin:
	cd frontend-admin && npm install

install: install-frontend install-admin

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

build-admin:
	cd frontend-admin && npm run build

build: build-backend build-frontend build-admin

# 数据库迁移（由 Go 启动时自动迁移）
migrate:
	cd backend && go run ./cmd/server

# 清理
clean:
	rm -rf backend/bin frontend/dist frontend-admin/dist
