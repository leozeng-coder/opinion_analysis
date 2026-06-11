# 工作流并发爬虫节点可行性评估报告

**日期**：2026-06-11  
**范围**：工作流引擎（Go）+ MediaCrawler API（Python）并发爬取支持  
**结论**：技术上可行，但 MediaCrawler 侧存在硬阻塞，需要改造后才能并发

---

## 一、需求描述

在工作流中为每个平台（xhs / dy / zhihu / wb / tieba / ks / bili）分别创建一个 `crawler_run` 节点，并让这些节点并发执行，以缩短多平台爬取的总耗时。

---

## 二、MediaCrawler 并发能力评估

### 2.1 核心结论：当前不支持并发

MediaCrawler API 服务（`api/services/crawler_manager.py`）采用**全局单例 `CrawlerManager`**，内部只保存一个 `subprocess.Popen` 进程引用：

```python
# api/services/crawler_manager.py:327
crawler_manager = CrawlerManager()   # 模块级单例，FastAPI 全局共用

class CrawlerManager:
    def __init__(self):
        self.process: Optional[subprocess.Popen] = None  # 仅一个进程槽
```

`POST /api/crawler/start` 路由在收到并发请求时**主动拒绝**：

```python
# api/routers/crawler.py:33
if crawler_manager.process and crawler_manager.process.poll() is None:
    raise HTTPException(status_code=400, detail="Crawler is already running")
```

这是一个有意为之的设计决策，不是缺陷。

### 2.2 如果强行绕过（假设并发）—— 其他约束分析

| 维度 | 状态 | 说明 |
|------|------|------|
| **MySQL 写入** | ✅ 无冲突 | 各平台写独立源表（`xhs_notes`、`dy_notes` 等），行级锁足够 |
| **Browser 数据目录** | ✅ 无冲突 | `USER_DATA_DIR = "%s_user_data_dir"`，各平台路径独立 |
| **config.PLATFORM** | ✅ 进程隔离 | 各子进程独立 Python 运行时，`config.PLATFORM` 互不干扰 |
| **asyncio ContextVar** | ✅ 进程隔离 | `var.py` 中的 `ContextVar` 作用域在进程内 |
| **CDP 调试端口** | ⚠️ 冲突风险 | `CDP_DEBUG_PORT = 9222`（`config/base_config.py:58`）所有实例默认同一端口；多实例同时启动会端口抢占 |
| **Log 流** | ⚠️ 混淆 | `CrawlerManager._logs` 单一列表，多进程日志交叉写入，可读性差 |
| **进程管理** | ❌ 阻塞 | `CrawlerManager.process` 单槽，Stop 信号只能发给一个进程 |

### 2.3 改造方案

要支持并发，需要将 MediaCrawler API 改造为 **多实例模型**：

**方案 A（推荐）：按平台拆分 CrawlerManager**

将全局单例改为 `Dict[str, CrawlerManager]`，每个平台一个独立的 manager：

```python
# 改造后
crawler_managers: Dict[str, CrawlerManager] = {}

def get_manager(platform: str) -> CrawlerManager:
    if platform not in crawler_managers:
        crawler_managers[platform] = CrawlerManager(platform)
    return crawler_managers[platform]
```

同时需要让每个实例分配独立的 CDP 端口（如 9222、9223、9224...），通过启动参数传入：

```python
def _build_command(self, config: CrawlerStartRequest) -> list:
    cmd = [...]
    cmd.extend(["--cdp_port", str(self.cdp_port)])  # 需要 main.py 支持此参数
```

**方案 B：多进程直接调用**

不修改 `CrawlerManager`，改为在 Go 侧直接 spawn `main.py` 子进程（绕过 FastAPI API 层），每次调用独立启动一个 Python 子进程并捕获 PID。实现最简单但失去 API 层的状态追踪。

**方案 C：运行多个 MediaCrawler 实例（重量级）**

为每个平台启动一个独立的 FastAPI 服务，监听不同端口（8085、8086...）。Go 侧根据平台路由到对应端口。隔离最彻底，运维成本最高。

### 2.4 MediaCrawler 改造工作量估算

| 改造项 | 方案 A | 方案 B | 方案 C |
|--------|--------|--------|--------|
| 修改量 | 中（~100 行） | 小（~50 行，主要在 Go 侧） | 大（部署配置） |
| 风险 | 低 | 中（进程泄露风险） | 低 |
| 推荐 | ✅ | 备选 | 不推荐 |

---

## 三、Go 工作流引擎并发评估

### 3.1 核心结论：架构上支持，有 3 处需要修改

#### 问题 1：纯串行执行循环

`engine.go` 的 `Execute` 方法对拓扑排序后的节点做线性迭代，即使多个节点没有依赖关系也串行执行：

```go
// engine.go:239 — 当前串行
for _, node := range sortedNodes {
    output, err := e.executeNode(execCtx, execution.ID, node, input)
    ...
}
```

**改法**：Wave-based 执行。在 Kahn 算法处理过程中，每一批"同时入度归零"的节点即为同一 wave，用 `sync.WaitGroup` 并发执行同一 wave 内的节点。

#### 问题 2：activeNodes 只存一个节点

```go
// engine.go:51
activeNodes  map[int64]NodeExecutor  // executionID → 单个节点
```

并发时第二个节点会覆盖第一个，导致 `CancelExecution` 只能停最后注册的节点。

**改法**：改为 `map[int64][]NodeExecutor`，取消时遍历列表逐个调用 `OnCancel`。

#### 问题 3：RunNode 的 activeRunID 是单值

```go
// nodes/crawler/run.go:26
activeRunID atomic.Uint64  // 单个 runID
```

`RunNode` 是全局注册的单例。两个 crawler_run 节点并发时，第二个节点执行 `n.activeRunID.Store(...)` 会覆盖第一个，`OnCancel` 只停后者。

**改法**：改为 `sync.Map` 存储所有活跃 runID（以 runID 为 key），`OnCancel` 遍历全部停掉：

```go
activeRunIDs sync.Map  // runID(uint64) → struct{}

// Execute 内：
n.activeRunIDs.Store(uint64(result.RunID), struct{}{})
defer n.activeRunIDs.Delete(uint64(result.RunID))

// OnCancel 内：
n.activeRunIDs.Range(func(k, _ any) bool {
    go n.crawlerSvc.StopCrawler(uint(k.(uint64)))
    return true
})
```

#### 问题 4：nodeOutputs 并发写安全

同一 wave 内多节点并发写 `nodeOutputs` map 需要加锁。改用 `sync.Map` 或 `map` + `sync.Mutex`。

#### 问题 5：output 字段命名冲突

并发爬取节点各自输出相同 key（`platform`、`crawlerRunId`、`syncPlatformCodes` 等），直接合并会互相覆盖。

**改法**：并发节点输出时以节点 ID 为前缀 namespace，或将 `syncPlatformCodes` 改为数组 append 合并语义。下游 `platform_sync` 节点也需相应支持多组 baseline。

---

## 四、端到端影响面汇总

```
工作流引擎（Go）           MediaCrawler API（Python）      MediaCrawler main.py
─────────────────────      ─────────────────────────────   ──────────────────────
engine.go                  api/services/crawler_manager.py  config/base_config.py
  Wave-based 执行           CrawlerManager 改多实例          CDP_DEBUG_PORT 改动态
  activeNodes → 列表        GET/POST 路由按 platform 路由
nodes/crawler/run.go
  activeRunID → sync.Map
  output namespace 化
```

---

## 五、改造工作量估算

| 模块 | 文件 | 改动量 | 难度 |
|------|------|--------|------|
| Go 引擎主循环 | `engine.go` | ~60 行 | 中 |
| Go RunNode | `nodes/crawler/run.go` | ~20 行 | 低 |
| Go platform_sync 节点 | `nodes/processor/platform_sync.go` | ~15 行 | 低 |
| Python CrawlerManager | `api/services/crawler_manager.py` | ~80 行 | 中 |
| Python 路由 | `api/routers/crawler.py` | ~20 行 | 低 |
| main.py CDP 端口参数 | `main.py` / `cmd_arg` | ~10 行 | 低 |
| **合计** | 6 个文件 | **~205 行** | **中** |

---

## 六、风险与限制

| 风险 | 等级 | 说明 |
|------|------|------|
| 平台反爬检测 | 高 | 多平台同时高频请求会消耗更多网络出口，部分平台对 IP 并发行为敏感 |
| 资源占用翻倍 | 中 | 7 平台并发 = 7 个 Chromium/CDP 浏览器进程，内存 ~2-4 GB |
| 调试复杂度上升 | 中 | 并发执行时节点日志交叉，排查问题更困难；需完善 progress 追踪 |
| Cookie 有效性 | 低 | 各平台 cookie 独立，并发不增加单平台 cookie 失效风险 |
| MySQL 写入竞争 | 低 | 各平台写独立源表，row-level lock 足够，无冲突风险 |

---

## 七、结论与建议

**MediaCrawler 现在不支持并发，是硬阻塞。** 需要先改造 `CrawlerManager` 为按平台的多实例模型，再推进 Go 引擎的 wave-based 并发。

**建议分两步走：**

1. **第一步**：改造 MediaCrawler API（方案 A），验证多平台并发进程稳定性。核心是把 `crawler_manager` 单例改为 `Dict[platform, CrawlerManager]`，处理 CDP 端口分配。
2. **第二步**：改造 Go 引擎，实现 wave-based 并发执行、`activeNodes` 多节点追踪、`RunNode.activeRunIDs` 改 `sync.Map`，以及下游节点对多 baseline 的支持。

如果仅想快速验证并发效果，可临时采用**方案 B**（Go 侧直接 spawn 子进程），跳过 MediaCrawler API 层的改造，代价是失去统一的状态追踪和 WebSocket 日志推送。
