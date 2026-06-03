# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

AI-powered Chinese public opinion monitoring platform. The system crawls content from 7 social platforms, runs LLM-based tagging and sentiment analysis, stores vectors in Milvus for RAG Q&A, and displays analytics via two React frontends.

## Architecture

Five independently running services:

| Service | Language | Port | Directory |
|---------|----------|------|-----------|
| Go API backend | Go 1.24, Gin + GORM | 8080 | `backend/` |
| User frontend | React 18 + Vite + Ant Design | 5173 | `frontend/` |
| Admin frontend | React 18 + Vite + Ant Design | 5174 | `frontend-admin/` |
| RAG vector service | Python, FastAPI + Milvus Lite | 5055 | `rag/` |
| Crawler scheduler | Python, Playwright + APScheduler | — | `crawler/` |

The Go backend manages RAG service as an optional subprocess (`backend/src/service/ragprocess/`). LLM config (API keys, model names, base URLs) is stored in the `system_settings` DB table and managed via the admin UI — not in YAML.

**Data flow:** Crawler → MySQL `articles` → Go tagger goroutine adds `ai_tags` → RAG service syncs to Milvus → frontend AI chat queries RAG → LLM generates response.

## Commands

### Backend (run from `backend/`)
```bat
go run ./cmd/createdb          # create database (first time only)
go run ./cmd/server            # start API server
go build -o bin/server ./cmd/server
go mod tidy
go test ./...                  # run all tests
go test ./src/api/handler/user/... -run TestAlertHandler  # single test
```

### Frontend (run from `frontend/` or `frontend-admin/`)
```bat
npm install
npm run dev
npm run build
```

### Python services
```bat
# RAG service
cd rag && .venv\Scripts\activate && python server.py

# Crawler (one-shot)
cd crawler && .venv\Scripts\activate && python run_once.py

# Crawler (long-running scheduler)
cd crawler && .venv\Scripts\activate && python scheduler.py
```

### Makefile shortcuts
```bash
make tidy                 # go mod tidy
make install              # npm install both frontends
make dev-backend / dev-frontend / dev-admin
make build                # build all three
make docker-up / docker-down
```

### One-click start (Windows)
```bat
start.bat                 # opens 5 CMD windows
set SKIP_RAG=1 && start.bat   # skip RAG service
```

## Backend Structure (`backend/src/`)

- `api/router.go` — route registration; three groups: public, user (JWT), admin (JWT + role)
- `api/handler/user/` — article, topic, alert, dashboard, crawler trigger, AI chat, platform data
- `api/handler/admin/` — user management, system settings, RAG management, audit log
- `model/` — 17 GORM entities; `migrate.go` runs AutoMigrate on startup
- `repository/` — data access layer; `store.go` wires all repos together
- `service/tagger/` — background goroutine that polls for untagged articles and calls LLM
- `service/ragprocess/` — manages the Python RAG subprocess lifecycle
- `service/rag/` — HTTP client for the RAG FastAPI service
- `service/alertengine/` — rule engine evaluating keyword/sentiment/platform alert conditions
- `service/workflow/` — workflow engine with typed nodes (crawler, processor, control, action)
- `service/sentiment/` — LLM-primary + SnowNLP fallback sentiment scoring
- `middleware/` — JWT auth (`auth.go`), admin role check (`admin.go`), request logger

## Configuration

**Backend** (`backend/config/config.yaml`):
- `database.dsn` — MySQL connection string (required)
- `jwt.secret` — must be changed from default
- `rag.enabled` / `rag.managed` — toggle RAG; set `managed: true` for Go to auto-start RAG subprocess
- `crawler.root` — path to crawler directory

**LLM settings** (API keys, model, base URL) live in the DB, not YAML. Edit via admin UI → System Config → LLM Config. Environment variable `DEEPSEEK_API_KEY` can override.

**Crawler** (`crawler/config.py`) and **RAG** (`rag/config.py`) each need DB credentials. Copy from `.example` files.

## Key Patterns

- All Go HTTP responses use `pkg/response/` helpers (`response.Success`, `response.Error`)
- GORM AutoMigrate runs at startup — add new fields to model structs, not raw SQL
- Workflow nodes implement the `Node` interface in `service/workflow/nodes/node.go`
- The alert engine uses `github.com/Knetic/govaluate` to evaluate rule expressions
- Frontend API calls go through `src/api/` Axios wrappers in each frontend; base URL is proxied via Vite to `:8080`

## Docker

In Docker, set `database.dsn` host to `mysql` (service name). Initial admin: `admin` / `admin` (override with `ADMIN_INIT_PASSWORD` env var).
