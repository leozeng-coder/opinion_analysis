# 舆情分析系统

基于 AI 的中文舆情监测与分析平台，集成多平台内容爬取、LLM 智能打标、RAG 向量检索与对话、实时数据可视化于一体。

---

## 目录

- [功能特性](#功能特性)
- [技术栈](#技术栈)
- [系统架构](#系统架构)
- [项目模块划分](#项目模块划分)
- [环境准备](#环境准备)
- [快速启动](#快速启动)
- [配置说明](#配置说明)
- [Docker 部署](#docker-部署)
- [访问地址](#访问地址)

---

## 功能特性

- **多平台内容采集**：自动拉取微博、知乎、B 站、今日头条等 12 个平台热榜关键词，驱动 Playwright 爬虫深度采集小红书、抖音、快手、微博、B 站、贴吧、知乎共 7 个平台的帖子与评论
- **AI 自动打标**：后台定时调用大模型（DeepSeek / OpenAI / Qwen 等）对文章批量生成 1~4 个话题标签
- **情感分析**：LLM 主力 + SnowNLP 回退，自动标记文章情感倾向
- **RAG 智能问答**：基于 Milvus Lite 向量库 + Sentence-Transformers / OpenAI Embedding，支持混合检索（语义 + 关键词）与 AI 多轮对话
- **预警规则引擎**：自定义关键词/情感/平台组合预警，实时触发预警记录
- **数据可视化**：ECharts 趋势图、词云、平台分布、话题热度看板
- **管理后台**：用户管理、系统配置、RAG 知识库管理、爬虫运维、任务管理、审计日志

---

## 技术栈

### 后端

| 技术 | 版本 | 用途 |
|------|------|------|
| Go | 1.22+ | 主服务语言 |
| Gin | v1.10 | HTTP 框架 |
| GORM | v1.25 | ORM（MySQL） |
| JWT (golang-jwt) | v5 | 身份认证 |
| Viper | v1.19 | 配置管理 |
| Zap | v1.27 | 结构化日志 |
| MySQL | 8.x | 主数据库 |
| Redis | 7.x | 缓存（可选） |

### 爬虫与 RAG 服务（Python）

| 技术 | 版本 | 用途 |
|------|------|------|
| Python | 3.11+ | 爬虫与 RAG 服务语言 |
| FastAPI + Uvicorn | 0.115 / 0.29 | RAG HTTP 服务 |
| APScheduler | 3.10+ | RAG 定时同步任务 |
| Playwright | 1.49+ | 浏览器自动化爬虫 |
| Milvus Lite | 2.4+ | 本地向量数据库 |
| PyMilvus | 2.4+ | Milvus 客户端 |
| Sentence-Transformers | 3.0+ | 本地向量化模型 |
| PyTorch | 2.1+ | 模型推理后端 |
| OpenAI SDK | 1.40+ | LLM / Embedding API 调用 |
| SnowNLP / jieba | latest | 中文情感分析、分词 |
| Pydantic | 2.x | 数据验证 |

### 前端

| 技术 | 版本 | 用途 |
|------|------|------|
| React | 18 | UI 框架 |
| TypeScript | 5.6 | 类型系统 |
| Vite | 5 | 构建工具 |
| Ant Design | 5 | UI 组件库 |
| ECharts | 5 | 数据可视化图表 |
| echarts-wordcloud | 2 | 词云图 |
| React Router | 6 | 路由 |
| Zustand | 5 | 全局状态管理 |
| Axios | 1.7 | HTTP 请求 |
| dayjs | 1.11 | 日期处理 |

---

## 系统架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                           用户 / 管理员                              │
└───────────┬────────────────────────────────────┬────────────────────┘
            │                                    │
            ▼                                    ▼
┌───────────────────┐                ┌───────────────────────┐
│  用户端前端        │                │     管理后台前端        │
│  React + ECharts  │                │  React + Ant Design   │
│  :5173 (dev)      │                │  :5174 (dev) /admin   │
└────────┬──────────┘                └──────────┬────────────┘
         │                                      │
         └───────────────┬──────────────────────┘
                         │  REST API
                         ▼
              ┌─────────────────────┐
              │    Go 后端 API      │
              │  Gin + GORM  :8080  │
              │                     │
              │  ┌───────────────┐  │
              │  │ AI 打标服务   │  │   ←  大模型 API
              │  │ (goroutine)   │  │      DeepSeek / OpenAI
              │  └───────────────┘  │
              │  ┌───────────────┐  │
              │  │ RAG 进程管理  │  │
              │  │ (subprocess)  │  │
              │  └───────┬───────┘  │
              └──────────┼──────────┘
                         │
              ┌──────────┼──────────────────────────────┐
              │          │                              │
              ▼          ▼                              ▼
         ┌────────┐ ┌──────────────────────┐  ┌───────────────┐
         │ MySQL  │ │  RAG 向量服务         │  │    Redis      │
         │  :3306 │ │  FastAPI  :5055       │  │    :6379      │
         │        │ │  Milvus Lite (本地)   │  │  （可选缓存）  │
         └────────┘ │  APScheduler 定时同步 │  └───────────────┘
              ▲     │  混合检索（语义+关键词）│
              │     └──────────────────────┘
              │
    ┌─────────────────────────┐
    │    Python 爬虫调度器     │
    │  scheduler.py           │
    │                         │
    │  ┌─────────────────┐    │
    │  │ Stage 1         │    │
    │  │ 宽泛话题提取     │    │   ←  NewsNow API（12 平台热榜）
    │  │ DeepSeek 关键词  │    │       + DeepSeek LLM 提取关键词
    │  └────────┬────────┘    │
    │           │             │
    │  ┌────────▼────────┐    │
    │  │ Stage 2         │    │
    │  │ 深度情感爬取     │    │   ←  Playwright（xhs/dy/ks/bili/
    │  │ MediaCrawler    │    │       wb/tieba/zhihu）
    │  └────────┬────────┘    │
    │           │ article_sync│
    └───────────┼─────────────┘
                │  写入 articles 表
                ▼
             MySQL
```

**数据流向：**
1. 爬虫调度器（Python）每日定时运行两阶段管道，将采集内容同步至 MySQL `articles` 表
2. Go 后端的 AI 打标服务（goroutine）定时扫描未打标文章，批量调用 LLM 生成 `ai_tags`
3. RAG 服务（Python FastAPI）定时增量同步 MySQL 文章到 Milvus 向量库
4. 用户在前端发起 AI 对话时，Go 后端通过 RAG 服务检索相关文章片段，构建增强 Prompt 后调用 LLM

---

## 项目模块划分

```
opinion_analysis/
├── backend/                    # Go 后端服务
│   ├── cmd/
│   │   ├── server/main.go      # 服务入口：加载配置、DB 迁移、启动后台服务
│   │   └── createdb/main.go    # 独立建库工具
│   ├── config/
│   │   ├── config.go           # 配置结构体定义
│   │   └── config.yaml.example # 配置模板
│   └── src/
│       ├── api/
│       │   ├── router.go       # 路由注册（公开 / 用户态 / 管理员）
│       │   └── handler/        # HTTP Handler
│       │       ├── user/       # 文章、话题、预警、爬虫、AI 对话
│       │       └── admin/      # 用户管理、系统配置、RAG、审计日志
│       ├── middleware/         # JWT 鉴权、角色校验、请求日志
│       ├── model/              # 17 个 GORM 实体（用户、文章、话题、预警等）
│       ├── repository/         # 数据访问层（CRUD + 复杂查询）
│       ├── service/
│       │   ├── tagger/         # AI 打标后台任务（goroutine + 热更新）
│       │   ├── ragprocess/     # RAG 子进程生命周期管理
│       │   └── rag/            # RAG HTTP 客户端
│       └── pkg/
│           ├── response/       # 统一 JSON 响应
│           └── utils/          # JWT 工具、敏感字段脱敏
│
├── frontend/                   # 用户端前端（React + Vite）
│   └── src/pages/
│       ├── dashboard/          # 数据看板（统计卡片）
│       ├── opinion/            # 舆情文章列表与详情
│       ├── topics/             # 话题列表
│       ├── alerts/             # 预警规则与记录
│       ├── stats/              # 统计图表（ECharts 趋势/词云/分布）
│       ├── crawler/            # 爬虫手动触发
│       └── assistant/          # AI 智能助手（RAG 多轮对话）
│
├── frontend-admin/             # 管理后台前端（React + Vite）
│   └── src/pages/
│       ├── system/             # 系统状态监控（DB / LLM / RAG 健康）
│       ├── config/             # 系统配置（Embedding、大模型、系统设置）
│       ├── tasks/              # 任务管理（AI 打标触发 / RAG 向量同步）
│       ├── users/              # 用户管理
│       ├── crawler/            # 爬虫运维（蜘蛛配置 / 运行记录）
│       ├── rag/                # RAG 知识库文章管理
│       ├── datasource/         # 数据源 CRUD
│       └── audit/              # 审计日志
│
├── crawler/                    # Python 爬虫 + RAG 服务
│   ├── scheduler.py            # 长期运行调度守护进程（从 DB 读配置定时触发）
│   ├── run_once.py             # 单次运行入口（Go 后端子进程调用）
│   ├── BroadTopicExtraction/   # Stage 1：从 NewsNow 拉热榜 + LLM 提取关键词
│   ├── DeepSentimentCrawling/  # Stage 2：Playwright 多平台爬虫（MediaCrawler）
│   ├── bridge/                 # 数据桥接：爬虫原始数据 → articles 表 + 情感分析
│   └── rag_service/            # RAG 向量检索服务
│       ├── server.py           # FastAPI 主服务（向量同步 / 检索 / 配置热更新）
│       └── embedder.py         # Embedder 抽象（本地模型 / OpenAI 兼容 API）
│
├── scripts/                    # Windows 启动脚本
│   ├── run-backend.cmd         # 建库 + 启动 Go 服务
│   ├── run-frontend.cmd        # 启动用户端前端（:5173）
│   ├── run-admin.cmd           # 启动管理后台前端（:5174）
│   ├── run-crawler.cmd         # 创建 venv + 安装依赖 + 启动爬虫调度
│   └── run-rag-service.cmd     # 安装 RAG 依赖 + 启动 RAG 服务（:5055）
│
├── docker-compose.yml          # Docker 全套编排
├── Makefile                    # 开发/构建快捷命令
└── start.bat                   # Windows 一键启动（5 个独立窗口）
```

### 用户角色

| 角色 | 权限 |
|------|------|
| `admin` | 全部功能 + 管理后台 |
| `analyst` | 查看文章/话题/统计，创建预警，手动触发爬虫 |
| `viewer` | 只读查看 |

---

## 环境准备

### 必须

| 软件 | 版本 | 说明 |
|------|------|------|
| Go | 1.22+ | 后端服务 |
| Node.js | LTS (20+) | 前端构建 |
| MySQL | 8.x | 主数据库，监听 `127.0.0.1:3306` |

### 爬虫与 RAG（可选，需要采集或向量检索功能时安装）

| 软件 | 版本 | 说明 |
|------|------|------|
| Python | 3.11+ | 爬虫调度 + RAG 服务 |
| Redis | 7.x | 爬虫内容去重缓存（可用内存缓存代替） |

> **PyTorch 安装说明**：RAG 服务的本地 Sentence-Transformers 依赖 PyTorch。
> 若仅使用 CPU，`pip install torch --index-url https://download.pytorch.org/whl/cpu` 可显著缩小安装包体积。
> 若使用 OpenAI 兼容 Embedding API 则无需本地 PyTorch。

---

## 快速启动

### 方式 A：一键脚本（Windows 推荐）

**第一步：复制并编辑配置文件**

```bat
cd backend
copy config\config.yaml.example config\config.yaml
```

编辑 `backend/config/config.yaml`，至少修改：
- `database.dsn`：填入正确的 MySQL 用户名和密码
- `jwt.secret`：修改为随机字符串

**第二步：双击 `start.bat`**（或在根目录 CMD 执行）

```bat
start.bat
```

首次运行会自动：
1. 创建数据库（`go run ./cmd/createdb`）
2. 安装前端依赖（`npm install`）
3. 创建 Python 虚拟环境并安装依赖（`crawler\.venv`）
4. 分别在独立窗口中启动后端、RAG 服务、用户前端、管理后台、爬虫调度

> 跳过 RAG 服务：执行 `set SKIP_RAG=1 && start.bat`
> 跳过爬虫：关闭对应的 CMD 窗口即可

---

### 方式 B：手动逐步启动

**1. 创建数据库**

```bat
cd backend
go mod tidy
go run ./cmd/createdb
```

**2. 启动后端**（终端 1，在 `backend/` 目录）

```bat
go run ./cmd/server
```

**3. 启动用户端前端**（终端 2）

```bat
cd frontend
npm install
npm run dev
```

**4. 启动管理后台前端**（终端 3）

```bat
cd frontend-admin
npm install
npm run dev
```

**5. 启动 RAG 服务**（终端 4，可选）

```bat
cd crawler
python -m venv .venv
.venv\Scripts\activate
pip install -r rag_service/requirements-rag.txt
python rag_service/server.py
```

> 本地 Sentence-Transformers 模型首次启动会自动下载（~400MB），请确保网络畅通或提前手动下载至 HuggingFace 缓存目录。

**6. 启动爬虫调度**（终端 5，可选）

```bat
cd crawler
.venv\Scripts\activate
pip install -r requirements.txt
playwright install chromium
python scheduler.py
```

---

### 方式 C：Makefile

```bash
make install-frontend   # npm install（两个前端）
make tidy               # go mod tidy
make dev                # 同时启动后端 + 两个前端
```

---

## 配置说明

### 后端配置（`backend/config/config.yaml`）

```yaml
server:
  port: "8080"
  mode: "debug"           # debug | release

database:
  dsn: "root:PASSWORD@tcp(127.0.0.1:3306)/opinion_analysis?charset=utf8mb4&parseTime=True&loc=Local"
  maxOpenConn: 100
  maxIdleConn: 10

jwt:
  secret: "change-me-in-production"
  expireHour: 24

crawler:
  enabled: true
  root: "../crawler"
  python: ""              # 留空自动检测 crawler/.venv/Scripts/python.exe

rag:
  enabled: false          # 是否启用 RAG 检索增强
  embedding_service_url: "http://127.0.0.1:5055"
  managed: false          # true = Go 自动拉起/重启本机 RAG 子进程
  auto_start: true
  root: "../crawler"
  python: ""
  server_script: "rag_service/server.py"
```

> **LLM 配置**（API Key、Base URL、Model）通过管理后台 → **系统配置 → 大模型配置** 维护，持久化在数据库 `system_settings` 表，无需写入 YAML。紧急情况可用环境变量 `DEEPSEEK_API_KEY` 覆盖。

### 爬虫配置（`crawler/config.py`）

```bash
cp crawler/config.py.example crawler/config.py
```

编辑 `crawler/config.py`，填入：
- `DB_HOST / DB_USER / DB_PASSWORD / DB_NAME`
- `DEEPSEEK_API_KEY`（用于关键词提取）

---

## Docker 部署

确保本机已安装 Docker 与 Docker Compose。

```bash
# 复制配置（首次）
cp backend/config/config.yaml.example backend/config/config.yaml
# 编辑 config.yaml，database.dsn 中 host 改为 mysql（Docker 服务名）

# 启动全部服务
docker-compose up -d --build

# 查看日志
docker-compose logs -f backend

# 停止
docker-compose down
```

Docker Compose 服务说明：

| 服务 | 说明 |
|------|------|
| `mysql` | MySQL 8.0，含 healthcheck |
| `redis` | Redis 7 |
| `backend` | Go API 服务，依赖 mysql + redis 就绪 |
| `frontend` | 用户端 nginx，反代 `/api/` → backend，`/admin/` → admin-frontend |
| `admin-frontend` | 管理后台 nginx，提供 `/admin/` 路径的 SPA |

> 初始管理员账户：`admin` / `admin`（可通过 `ADMIN_INIT_PASSWORD` 环境变量修改）

---

## 访问地址

### 本地开发

| 服务 | 地址 |
|------|------|
| 用户端前端 | http://localhost:5173 |
| 管理后台 | http://localhost:5174 |
| 后端 API | http://localhost:8080 |
| RAG 服务 | http://localhost:5055 |

### Docker 生产

| 服务 | 地址 |
|------|------|
| 用户端 | http://localhost |
| 管理后台 | http://localhost/admin |
| 后端 API | http://localhost/api（经 nginx 反代） |

---

## 路径速查

| 路径 | 说明 |
|------|------|
| `start.bat` | Windows 一键启动入口 |
| `backend/config/config.yaml` | 后端核心配置（端口、DB、JWT） |
| `crawler/config.py` | 爬虫数据库与 API Key 配置 |
| `crawler/rag_service/server.py` | RAG FastAPI 服务主文件 |
| `backend/src/service/tagger/` | AI 打标后台任务实现 |
| `backend/src/service/ragprocess/` | RAG 子进程管理 |
| `docker-compose.yml` | Docker 全套编排配置 |
| `Makefile` | 开发/构建快捷命令 |
