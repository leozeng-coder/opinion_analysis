# 爬虫模块效率分析

> 分析日期：2026-06-05  
> 分析范围：`MediaCrawler/` 爬虫服务 + `backend/src/service/crawler/` Go 调度层

---

## 一、架构概览

```
工作流触发
  └─► Go RunNode (workflow/nodes/crawler/run.go)
        └─► Go CrawlerService (service/crawler/service.go)
              └─► HTTP POST /api/crawler/start
                    └─► Python CrawlerManager (api/services/crawler_manager.py)
                          └─► subprocess: python main.py --platform xxx ...
```

整个爬取分两个阶段：
- **阶段一（慢）**：MediaCrawler Python 进程爬取平台数据，写入 MySQL 原始表（xhs_note / douyin_aweme / ...）
- **阶段二**：Go 平台同步层把原始表数据写入 articles 表

本文聚焦阶段一的三个核心问题。

---

## 二、问题一：每次爬取文章数量极少

### 根因

`MediaCrawler/config/base_config.py` 第 98 行：

```python
CRAWLER_MAX_NOTES_COUNT = 5
```

默认每次只爬 **5 篇**文章。

### 传参链路断裂

`cmd_arg/arg.py` 的 `parse_cmd()` **没有** `--max_notes_count` 参数，
导致 `CrawlerManager._build_command()` 无法把请求中的数量传给 main.py。

无论工作流或前端传入多大的数字，实际执行时始终用配置文件里的 5。

### 影响

一次完整爬取最多产出 5 篇文章，流程走完却数据量极小。

---

## 三、问题二：单次爬取速度慢

### 根因 1：高并发数固定为 1

`base_config.py` 第 102 行：

```python
MAX_CONCURRENCY_NUM = 1
```

平台内部同时只发 1 个请求，虽然该参数已通过 `--max_concurrency_num` 暴露，
但默认值没有调整，且工作流节点也没有传这个参数。

### 根因 2：请求间随机 sleep 2-5 秒

`base_config.py` 第 134 行：

```python
CRAWLER_MAX_SLEEP_SEC = (2, 5)
```

每爬一个内容随机等待 2-5 秒。5 篇文章 × 平均 3.5 秒 = 约 17.5 秒仅用于等待。
若提升到 50 篇，等待时间将达 175 秒（近 3 分钟）。

### 综合效果

| 参数 | 当前值 | 影响 |
|------|--------|------|
| CRAWLER_MAX_NOTES_COUNT | 5 | 产出量极低 |
| MAX_CONCURRENCY_NUM | 1 | 请求串行化 |
| CRAWLER_MAX_SLEEP_SEC | (2, 5) 秒 | 每篇额外等待 |

---

## 四、问题三：无法多平台并发爬取

### 根因：CrawlerManager 单进程设计

`api/services/crawler_manager.py`（第 35 行）：

```python
class CrawlerManager:
    def __init__(self):
        self.process: Optional[subprocess.Popen] = None  # 只有一个进程槽
```

`start()` 方法（第 96 行）：

```python
async def start(self, config: CrawlerStartRequest) -> bool:
    async with self._lock:
        if self.process and self.process.poll() is None:
            return False  # 任何平台在跑，新请求直接拒绝
```

`CrawlerStartRequest` schema 也只接受单个平台：

```python
class CrawlerStartRequest(BaseModel):
    platform: PlatformEnum  # 单值枚举，不支持列表
```

### 传导链路

```
Go RunNode.Execute()
  └─► crawlerSvc.Trigger()          # 每次只传一个 platform
        └─► callMediaCrawlerAPI()   # POST /start 单平台
              └─► CrawlerManager.start()  # 如果上一个平台未结束 → 返回 false
                    └─► Go 记录 status=failed
```

所以如果工作流想连续爬知乎再爬小红书，必须等第一个完全结束才能启动第二个，**完全串行**。

### 影响

7 个平台顺序爬取，总耗时 = 各平台耗时之和。若每个平台爬 50 篇耗时 10 分钟，7 个平台需要 **70 分钟**。并发情况下只需 10 分钟。

---

## 五、改造方案

### 方案概览

| 编号 | 改动文件 | 改动内容 | 解决问题 |
|------|---------|---------|---------|
| 1 | `config/base_config.py` | NOTES 5→50，并发 1→3，sleep (2,5)→(1,3) | 问题一、二 |
| 2 | `cmd_arg/arg.py` | 新增 `--max_notes_count` CLI 参数 | 问题一 |
| 3 | `api/schemas/crawler.py` | `CrawlerStartRequest` 增加 `max_notes_count` 字段 | 问题一 |
| 4 | `api/services/crawler_manager.py` | 重构为 `Dict[platform, process]` 多进程管理 | 问题三 |
| 5 | `api/routers/crawler.py` | stop 支持指定平台，status 聚合多平台 | 问题三 |
| 6 | `backend/src/service/crawler/service.go` | `MediaCrawlerStartRequest` 增加字段；多平台并发调用 | 问题三 |

### 方案一（配置层，5 分钟）

直接修改 `base_config.py`：

```python
CRAWLER_MAX_NOTES_COUNT = 50      # 5 → 50
MAX_CONCURRENCY_NUM = 3            # 1 → 3
CRAWLER_MAX_SLEEP_SEC = (1, 3)    # (2,5) → (1,3)
```

立即生效，无需改代码，重启 MediaCrawler 服务即可。

### 方案二（参数传透，中等）

让工作流节点能动态控制爬取数量：

1. `cmd_arg/arg.py` 新增 `--max_notes_count`
2. `CrawlerStartRequest` 新增 `max_notes_count: int = 50`
3. `CrawlerManager._build_command()` 追加 `--max_notes_count` 参数
4. Go `MediaCrawlerStartRequest` 新增 `MaxNotesCount int` 字段并传入命令行

### 方案三（多平台并发，较大）

核心改造：把 `CrawlerManager.process` 改为 `processes: Dict[str, Popen]`。

**Python 侧逻辑变化：**
- `start(config)` → 仅检查当前 platform 是否在跑，不影响其他平台
- `stop(platform=None)` → 停指定平台或全部
- `get_status()` → 聚合：任一平台 running 则返回 running，全部 idle 则返回 idle

**Go 侧逻辑变化：**
- 当 `platforms` 列表有多个值时，并发启动多个 goroutine 各调用 `/start`
- `waitMediaCrawlerIdle` 轮询直到 status 为 idle（此时所有平台均完成）

---

## 六、风险说明

| 风险 | 说明 |
|------|------|
| 平台风控 | 并发数和 sleep 调低后被封号/限流的概率上升，建议按平台测试后再上线 |
| 内存占用 | 多平台并发 = 多个 Chrome 进程同时运行，内存需求线性增加（每个约 300-500MB） |
| Cookie 冲突 | 各平台用独立的 `user_data_dir`，已由配置 `USER_DATA_DIR = "%s_user_data_dir"` 隔离，无冲突 |
| DB 写入竞争 | 多平台并发写入同一 MySQL，需确认连接池上限（当前 `maxOpenConn: 100`，足够） |
