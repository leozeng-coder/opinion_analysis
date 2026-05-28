# n8n 工作流迁移计划

## 目标
将当前耦合的爬虫→AI打标→向量化→舆情分析流程拆分为独立的 n8n 工作流，提升可维护性和可观测性。

---

## 阶段 1：基础设施准备（1-2天）

### 1.1 部署 n8n
```yaml
# docker-compose.yml 新增服务
services:
  n8n:
    image: n8nio/n8n:latest
    ports:
      - "5678:5678"
    environment:
      - N8N_BASIC_AUTH_ACTIVE=true
      - N8N_BASIC_AUTH_USER=admin
      - N8N_BASIC_AUTH_PASSWORD=${N8N_PASSWORD}
      - WEBHOOK_URL=http://localhost:5678/
    volumes:
      - ./n8n_data:/home/node/.n8n
    depends_on:
      - mysql
      - redis
```

### 1.2 创建 HTTP API 适配层
在 Go 后端新增 `/api/internal/n8n/` 路由组，提供以下接口：

```go
// backend/src/api/handler/internal/n8n_adapter.go
type N8nAdapterHandler struct {
    articleRepo   *repository.ArticleRepository
    crawlerRepo   *repository.CrawlerRepository
    ragClient     *rag.Client
    taggerService *tagger.Service
}

// 接口列表
POST   /api/internal/n8n/crawler/stage1      # 触发 Stage1 热榜爬取
POST   /api/internal/n8n/crawler/stage2      # 触发 Stage2 深度爬取
POST   /api/internal/n8n/articles/batch      # 批量获取待处理文章
POST   /api/internal/n8n/articles/tag        # AI 打标单篇文章
POST   /api/internal/n8n/articles/vectorize  # 向量化单篇文章
POST   /api/internal/n8n/digest/generate     # 生成舆情摘要
GET    /api/internal/n8n/health              # 健康检查
```

**安全措施：**
- 使用独立的 API Key 认证（非 JWT）
- IP 白名单限制（仅允许 n8n 容器）
- 请求频率限制

---

## 阶段 2：工作流迁移（3-5天）

### Workflow 1: 爬虫数据采集流程

**触发方式：**
- 定时触发（Cron: 每天 8:00）
- Webhook 手动触发（前端按钮）

**节点设计：**
```
1. [Trigger] Schedule/Webhook
   ↓
2. [HTTP Request] POST /api/internal/n8n/crawler/stage1
   - 返回: { keywords: ["关键词1", "关键词2"], topic_id: 123 }
   ↓
3. [Code] 遍历关键词列表
   ↓
4. [HTTP Request] POST /api/internal/n8n/crawler/stage2
   - Body: { keyword: "{{$json.keyword}}", topic_id: 123 }
   - 批量模式：每次处理 5 个关键词
   ↓
5. [Wait] 等待 30 秒（避免平台限流）
   ↓
6. [HTTP Request] GET /api/internal/n8n/crawler/status/{{$json.run_id}}
   - 轮询检查爬取状态
   ↓
7. [IF] status == "success"
   ├─ Yes → 触发 Workflow 2（Webhook）
   └─ No  → [Send Email] 通知管理员失败
```

**错误处理：**
- Stage1 失败：重试 3 次，间隔 5 分钟
- Stage2 失败：跳过该关键词，继续下一个
- 全部失败：发送邮件 + 写入审计日志

---

### Workflow 2: AI 打标流程

**触发方式：**
- Webhook（由 Workflow 1 触发）
- 定时触发（每小时检查一次未打标文章）

**节点设计：**
```
1. [Trigger] Webhook/Schedule
   ↓
2. [HTTP Request] GET /api/internal/n8n/articles/batch?status=untagged&limit=50
   - 返回: [{ id: 1, title: "...", content: "..." }, ...]
   ↓
3. [Split In Batches] 每批 10 篇
   ↓
4. [HTTP Request] POST /api/internal/n8n/articles/tag
   - Body: { article_id: {{$json.id}}, title: "...", content: "..." }
   - 调用 DeepSeek API 生成标签
   - 返回: { tags: ["tag1", "tag2"], sentiment: "positive" }
   ↓
5. [MySQL] UPDATE articles SET ai_tags=?, sentiment=?, tagged_at=NOW()
   ↓
6. [Wait] 1 秒（避免 API 限流）
   ↓
7. [Loop] 返回步骤 3，直到所有批次完成
   ↓
8. [Webhook] 触发 Workflow 3（向量化）
```

**优化点：**
- 使用 n8n 的 `Split In Batches` 节点自动分批
- DeepSeek API 失败时，降级到 SnowNLP 本地情感分析
- 记录每篇文章的处理耗时到 `article_processing_logs` 表

---

### Workflow 3: 向量化流程

**触发方式：**
- Webhook（由 Workflow 2 触发）
- 定时触发（每 2 小时增量同步）

**节点设计：**
```
1. [Trigger] Webhook/Schedule
   ↓
2. [HTTP Request] GET /api/internal/n8n/articles/batch?vectorized=false&limit=100
   ↓
3. [Split In Batches] 每批 20 篇
   ↓
4. [HTTP Request] POST /api/internal/n8n/articles/vectorize
   - Body: { articles: [{id, title, content}, ...] }
   - 调用 RAG 服务批量向量化
   - 返回: { success_count: 18, failed_ids: [12, 34] }
   ↓
5. [MySQL] UPDATE articles SET vectorized=true WHERE id IN (...)
   ↓
6. [IF] failed_ids.length > 0
   ├─ Yes → [Wait] 10 秒后重试失败的文章
   └─ No  → 继续
   ↓
7. [Loop] 返回步骤 3
```

**性能优化：**
- 批量向量化（20 篇/批）减少 HTTP 请求
- 失败文章单独重试，不阻塞整体流程
- 向量化完成后触发 Milvus 索引刷新

---

### Workflow 4: 舆情分析流程

**触发方式：**
- 定时触发（每天 9:00 生成昨日分析）
- 手动触发（管理后台按钮）

**节点设计：**
```
1. [Trigger] Schedule/Webhook
   ↓
2. [MySQL] 查询近 7 天的文章统计
   SELECT 
     DATE(published_at) as date,
     platform,
     sentiment,
     COUNT(*) as count
   FROM articles
   WHERE published_at >= DATE_SUB(NOW(), INTERVAL 7 DAY)
   GROUP BY date, platform, sentiment
   ↓
3. [Code] 聚合数据为 JSON
   {
     "total_articles": 1234,
     "sentiment_distribution": {"positive": 60%, "negative": 20%, ...},
     "top_topics": ["话题1", "话题2"],
     "platform_breakdown": {...}
   }
   ↓
4. [HTTP Request] POST /api/internal/n8n/digest/generate
   - Body: { stats: {...}, date_range: "2026-05-19 to 2026-05-25" }
   - 调用 DeepSeek 生成分析报告
   - 返回: { summary: "本周舆情整体...", insights: [...] }
   ↓
5. [MySQL] INSERT INTO daily_digests (date, summary, stats, created_at)
   ↓
6. [Send Email] 发送报告给订阅用户（可选）
```

---

## 阶段 3：Go 后端改造（2-3天）

### 3.1 移除旧的后台任务

**删除/禁用：**
- `backend/src/service/tagger/` 的 goroutine 定时任务
- `rag/server.py` 中的 APScheduler 定时同步
- `crawler/scheduler.py` 的长期守护进程

**保留：**
- 爬虫的 `run_once.py` 脚本（改为被 n8n 调用）
- RAG 的 HTTP API（改为被 n8n 调用）

### 3.2 新增 n8n 适配层

```go
// backend/src/api/handler/internal/n8n_adapter.go
package internal

import (
    "github.com/gin-gonic/gin"
    "opinion_analysis/src/middleware"
)

func RegisterN8nRoutes(r *gin.RouterGroup) {
    n8n := r.Group("/n8n")
    n8n.Use(middleware.N8nAPIKeyAuth()) // 独立认证中间件
    
    handler := NewN8nAdapterHandler()
    
    // 爬虫相关
    n8n.POST("/crawler/stage1", handler.TriggerStage1)
    n8n.POST("/crawler/stage2", handler.TriggerStage2)
    n8n.GET("/crawler/status/:run_id", handler.GetCrawlerStatus)
    
    // 文章处理
    n8n.GET("/articles/batch", handler.GetArticlesBatch)
    n8n.POST("/articles/tag", handler.TagArticle)
    n8n.POST("/articles/vectorize", handler.VectorizeArticles)
    
    // 舆情分析
    n8n.POST("/digest/generate", handler.GenerateDigest)
}
```

### 3.3 配置文件更新

```yaml
# backend/config/config.yaml
n8n:
  enabled: true
  api_key: "n8n-secret-key-change-me"  # 从环境变量读取
  allowed_ips:
    - "127.0.0.1"
    - "172.18.0.0/16"  # Docker 网络
  webhook_base_url: "http://n8n:5678/webhook"
```

---

## 阶段 4：前端适配（1天）

### 4.1 爬虫触发页面

**改动：**
- 原来：调用 `POST /api/crawler/run-now`
- 现在：调用 `POST /api/crawler/trigger-workflow`（内部触发 n8n Webhook）

**新增功能：**
- 显示 n8n 工作流执行链接（跳转到 n8n UI）
- 实时显示各阶段状态（通过轮询 n8n API）

### 4.2 管理后台新增页面

**路径：** `/admin/workflows`

**功能：**
- 查看所有工作流的执行历史
- 手动触发任意工作流
- 查看失败节点的错误日志
- 重试失败的工作流

---

## 阶段 5：监控与告警（1-2天）

### 5.1 n8n 执行日志同步

**方案：**
- n8n 每个工作流结束时，调用 Go 后端 Webhook
- Go 后端将执行结果写入 `workflow_execution_logs` 表

```sql
CREATE TABLE workflow_execution_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  workflow_id VARCHAR(100) NOT NULL,
  workflow_name VARCHAR(200),
  execution_id VARCHAR(100) NOT NULL,
  status ENUM('success', 'error', 'waiting') NOT NULL,
  started_at DATETIME NOT NULL,
  finished_at DATETIME,
  error_message TEXT,
  execution_data JSON,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_workflow_id (workflow_id),
  INDEX idx_status (status),
  INDEX idx_started_at (started_at)
);
```

### 5.2 告警规则

**触发条件：**
- 工作流连续失败 3 次
- 单个工作流执行超过 30 分钟
- AI 打标成功率低于 80%
- 向量化失败率高于 10%

**通知方式：**
- 邮件（管理员）
- 企业微信/钉钉（可选）
- 管理后台红点提示

---

## 数据库变更

### 新增表

```sql
-- n8n 工作流执行日志
CREATE TABLE workflow_execution_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  workflow_id VARCHAR(100) NOT NULL,
  workflow_name VARCHAR(200),
  execution_id VARCHAR(100) NOT NULL,
  status ENUM('success', 'error', 'waiting') NOT NULL,
  started_at DATETIME NOT NULL,
  finished_at DATETIME,
  error_message TEXT,
  execution_data JSON,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 文章处理日志（用于性能分析）
CREATE TABLE article_processing_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  article_id BIGINT NOT NULL,
  stage ENUM('tagging', 'vectorization', 'analysis') NOT NULL,
  status ENUM('success', 'failed', 'skipped') NOT NULL,
  duration_ms INT,
  error_message TEXT,
  processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_article_id (article_id),
  INDEX idx_stage (stage)
);
```

### 修改表

```sql
-- articles 表新增字段
ALTER TABLE articles 
  ADD COLUMN vectorized BOOLEAN DEFAULT FALSE COMMENT '是否已向量化',
  ADD COLUMN tagged_at DATETIME COMMENT 'AI 打标时间',
  ADD COLUMN vectorized_at DATETIME COMMENT '向量化时间';

-- 为查询优化添加索引
CREATE INDEX idx_vectorized ON articles(vectorized);
CREATE INDEX idx_tagged_at ON articles(tagged_at);
```

---

## 优势对比

### 迁移前（当前架构）

| 问题 | 影响 |
|------|------|
| 流程不透明 | 无法知道文章卡在哪个环节 |
| 错误处理弱 | AI 打标失败后静默中断 |
| 重试困难 | 需要手动改数据库状态 |
| 扩展性差 | 新增环节需要改多处代码 |
| 监控缺失 | 不知道各环节的成功率和耗时 |

### 迁移后（n8n 架构）

| 优势 | 价值 |
|------|------|
| 可视化流程 | 一眼看出数据流转路径 |
| 自动重试 | n8n 内置重试机制 |
| 错误追踪 | 每个节点的输入输出都可查 |
| 灵活扩展 | 拖拽即可新增节点 |
| 性能监控 | n8n 自带执行时长统计 |
| 版本管理 | 工作流可导出为 JSON 版本控制 |

---

## 风险与应对

### 风险 1：n8n 单点故障
**应对：**
- n8n 使用 Docker 部署，配置自动重启
- 关键工作流设置失败告警
- 保留 Go 后端的手动触发接口作为降级方案

### 风险 2：迁移期间双轨运行
**应对：**
- 先部署 n8n，但不启用（`n8n.enabled=false`）
- 在测试环境验证所有工作流
- 生产环境灰度切换（先切 10% 流量）

### 风险 3：n8n 学习成本
**应对：**
- 提供工作流模板（JSON 导入）
- 编写操作文档（截图 + 视频）
- 关键节点添加注释说明

---

## 时间估算

| 阶段 | 工作量 | 依赖 |
|------|--------|------|
| 阶段 1：基础设施 | 1-2 天 | 无 |
| 阶段 2：工作流迁移 | 3-5 天 | 阶段 1 |
| 阶段 3：Go 后端改造 | 2-3 天 | 阶段 2 |
| 阶段 4：前端适配 | 1 天 | 阶段 3 |
| 阶段 5：监控告警 | 1-2 天 | 阶段 3 |
| **总计** | **8-13 天** | - |

---

## 下一步行动

1. **立即执行：** 在 `docker-compose.yml` 中添加 n8n 服务
2. **本周完成：** 创建第一个工作流（爬虫数据采集）
3. **下周完成：** 迁移 AI 打标和向量化流程
4. **两周后：** 全面切换到 n8n，移除旧的定时任务

---

## 参考资料

- [n8n 官方文档](https://docs.n8n.io/)
- [n8n Docker 部署指南](https://docs.n8n.io/hosting/installation/docker/)
- [n8n Webhook 触发器](https://docs.n8n.io/integrations/builtin/core-nodes/n8n-nodes-base.webhook/)
- [n8n MySQL 节点](https://docs.n8n.io/integrations/builtin/app-nodes/n8n-nodes-base.mysql/)
