# 舆情分析系统 — 环境准备与启动

本地开发默认：**Windows + 本机 MySQL + Go 后端 + Vite 前端**（与 `docker-compose` 二选一即可）。

---

## 1. 安装依赖

| 软件 | 版本建议 | 说明 |
|------|----------|------|
| Go | 1.22+（与 `backend/go.mod` 中 `go` 行一致） | 安装后新开终端，执行 `go version` |
| Node.js | LTS | 含 `npm`，执行 `node -v`、`npm -v` |
| MySQL | 8.x | 监听地址与端口需与下面配置文件一致（默认 `127.0.0.1:3306`） |
| Python | 3.11+（可选） | 跑 **`start.bat` 里的爬虫** 或 `scripts\run-crawler.cmd` 时需要 |

可选：本机安装 **Git**、**Make**（想用 `Makefile` 时）。

---

## 2. 配置数据库

1. 启动 MySQL 服务。  
2. 编辑 **`backend/config/config.yaml`**，修改 `database.dsn` 中的用户名、密码，使之与本机一致；库名默认 `opinion_analysis`。  
3. 首次在本机建库，在 **`backend`** 目录执行：

```bat
go mod tidy
go run ./cmd/createdb
```

`createdb` 会读取同一 `config.yaml`，自动 `CREATE DATABASE IF NOT EXISTS`（库名与 DSN 一致）。

---

## 3. 安装前端依赖（首次）

在 **`frontend`** 目录：

```bat
npm install
```

已有 `node_modules` 可跳过。

---

## 4. 启动项目

### 方式 A：一键脚本（推荐）

在仓库**根目录**双击 **`start.bat`**（或在该目录打开 CMD 后执行 `start.bat`）。

- 会新开窗口：**后端（8080）**、**前端（5173）**、**爬虫（定时任务）**。  
- 爬虫需要本机 **Python 3** 在 PATH 中；首次会在 `crawler\.venv` 创建虚拟环境并安装依赖。  
- 若修改了 `backend/config/config.yaml` 里的数据库密码，请同步修改 `scripts/run-crawler.cmd` 中的 `CRAWLER_DB_PASSWORD`，或在运行前 `set CRAWLER_DB_PASSWORD=...`。  
- 是否启动成功以**各子窗口**里的日志为准。

### 方式 B：命令行（两个终端）

终端 1 — 后端（须在 `backend` 目录，且已配置好 MySQL）：

```bat
cd backend
go run ./cmd/server
```

终端 2 — 前端：

```bat
cd frontend
npm run dev
```

### 方式 C：Makefile（需 Make）

在仓库根目录：

```bat
make install-frontend
make tidy
make dev
```

（`make dev` 会同时起前后端；Windows 下依赖本机 Make 实现。）

---

## 5. 访问地址

| 服务 | 地址 |
|------|------|
| 前端（开发） | http://localhost:5173 |
| 后端 API | http://localhost:8080 |

生产/联调时代理以 `frontend/vite.config.ts` 与 `frontend/nginx.conf` 为准。

---

## 6. Docker 全套（可选）

本机已安装 Docker 时，可在仓库根目录：

```bat
docker-compose up -d --build
```

具体服务与端口见根目录 **`docker-compose.yml`**。与「本机 Go + npm」不要重复占用同一 MySQL 端口。

---

## 7. 相关路径速查

| 路径 | 作用 |
|------|------|
| `start.bat` | Windows 一键启动入口 |
| `scripts/run-backend.cmd` | 后端子进程脚本 |
| `scripts/run-frontend.cmd` | 前端子进程脚本 |
| `scripts/run-crawler.cmd` | 爬虫：venv、`pip install`、`python scheduler.py` |
| `backend/config/config.yaml` | 端口、MySQL DSN、JWT 等 |
| `backend/cmd/createdb` | 按配置创建数据库 |

后端须从 **`backend`** 目录运行（或保证工作目录正确），以便加载 `config/config.yaml`。
