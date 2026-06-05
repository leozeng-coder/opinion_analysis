# 工业级爬虫设计方案 v2

> 目标：每平台每天 200+ 篇不重复文章，合规、安全、可持续运行  
> 更新日期：2026-06-05  
> 范围：`MediaCrawler/` Python 爬虫服务 + Go 调度层 + 后台管理动态配置

---

## 一、现状产能评估

### 每次爬取实际产出（当前 CRAWLER_MAX_NOTES_COUNT=5）

| 平台 | 每页条数 | 当前实际产出/次 | 达到 200 篇需要的页数 | 预估单次耗时（优化后）|
|------|---------|--------------|---------------------|----------------|
| XHS  | 20      | ~20 篇       | 10 页               | ~8 分钟        |
| 抖音  | 10      | ~10 篇       | 20 页               | ~12 分钟       |
| 快手  | 20      | ~20 篇       | 10 页               | ~8 分钟        |
| B 站  | 20      | ~20 篇       | 10 页               | ~8 分钟        |
| 微博  | 10      | ~10 篇       | 20 页               | ~12 分钟       |
| 贴吧  | 30      | ~30 篇       | 7 页                | ~6 分钟        |
| 知乎  | 20      | ~20 篇       | 10 页               | ~8 分钟        |

> 耗时按 MAX_CONCURRENCY_NUM=3、CRAWLER_MAX_SLEEP_SEC=(1,3) 估算。

### 当前核心瓶颈

| 瓶颈 | 当前值 | 影响 |
|------|-------|------|
| CRAWLER_MAX_NOTES_COUNT=5 | 每次只爬 5 篇 | ×10 差距 |
| MAX_CONCURRENCY_NUM=1 | 详情请求串行 | ×3 差距 |
| CRAWLER_MAX_SLEEP_SEC=(2,5) | 平均 3.5s/篇 | ×2 差距 |
| 排序方式=热度降序 | 4 次爬取返回同样数据 | 有效产出×1 |
| CrawlerManager 单进程 | 7 平台必须排队 | ×7 差距 |
| 无每日多次调度 | 每天只触发 1 次 | ×4 差距 |
| 所有配置写死在文件 | 需改代码才能调整 | 运维负担极高 |

---

## 二、设计目标

- **产量**：每平台 200+ 篇/天（7 平台合计 1400+ 篇/天）
- **不重复**：使用时间排序确保每次爬取的是新发布内容
- **动态配置**：爬取参数、代理配置全部从后台管理界面配置，不改代码
- **安全**：代理密钥通过进程环境变量注入（不出现在命令行参数中）
- **Cookie 管理**：手动扫码登录后在后台粘贴 Cookie，支持 7 个平台分别管理
- **合规**：时间排序 + 请求间隔 + IP 轮换三重保障

---

## 三、排序策略：确保每次爬取不重复

### 根本原因

当前 XHS 使用 `popularity_descending`（热度降序），热门帖子排名短期不变，导致 4 次/天爬取结果几乎完全相同。

### 各平台排序方案

不同平台的排序 API 参数不同，需按平台分别配置：

| 平台 | 推荐排序值 | 参数名 | 效果 |
|------|-----------|-------|------|
| XHS  | `time_descending` | `SORT_TYPE` | 最新发布，每次返回新内容 |
| 抖音  | `2`（LATEST）+ `1`（最近一天） | `DY_SORT_TYPE` + `DY_PUBLISH_TIME_TYPE` | 最新发布的当天视频 |
| 快手  | 无专用排序参数 | — | 默认综合，结合分散调度自然去重 |
| B 站  | 无专用 sort 配置（硬编码 DEFAULT） | — | 同上 |
| 微博  | `real_time`（值`"61"`）| `WEIBO_SEARCH_TYPE` | 实时搜索，返回最新微博 |
| 贴吧  | 硬编码 `TIME_DESC` | — | 已是时间降序，无需改动 |
| 知乎  | `created_time` + `a_day` | `ZHIHU_SORT` + `ZHIHU_SEARCH_TIME` | 最新回答，限最近一天 |

> 知乎 `get_note_by_keyword()` 接收 `sort` 和 `search_time` 参数，但当前 `core.py` 调用时未传（用默认值）。需改 `zhihu/core.py` 传入配置值。
> 快手/B 站的排序写死在 `core.py` 中无配置化路径，通过分散调度 + 去重机制保证有效性。

### 需要新增到各平台 config 文件的配置键

```python
# config/xhs_config.py
SORT_TYPE = "time_descending"           # 改：popularity_descending → time_descending

# config/dy_config.py
DY_SORT_TYPE = 2                        # 新增：0=综合 1=最多赞 2=最新
PUBLISH_TIME_TYPE = 1                   # 改：0=不限 → 1=最近一天

# config/weibo_config.py
WEIBO_SEARCH_TYPE = "real_time"         # 改：default → real_time

# config/zhihu_config.py（新增）
ZHIHU_SORT = "created_time"             # 新增："" = 默认  "created_time" = 最新
ZHIHU_SEARCH_TIME = "a_day"            # 新增："" = 不限  "a_day" = 最近一天
```

---

## 四、动态配置架构

### 4.1 配置分类

爬虫配置分三大类，全部存入 `system_settings` 表（已有基础设施）：

**A. 全局爬取行为**（对所有平台生效）

| system_settings key | 默认值 | 说明 |
|---------------------|-------|------|
| `crawler.max_notes_count` | `50` | 每次最多爬取篇数 |
| `crawler.max_concurrency_num` | `3` | 并发详情抓取数 |
| `crawler.sleep_sec_min` | `1` | 请求间隔最小值（秒）|
| `crawler.sleep_sec_max` | `3` | 请求间隔最大值（秒）|
| `crawler.enable_comments` | `false` | 是否爬取评论（关闭以提速）|
| `crawler.enable_sub_comments` | `false` | 是否爬取二级评论 |

**B. IP 代理池**

| system_settings key | 默认值 | 说明 |
|---------------------|-------|------|
| `crawler.enable_ip_proxy` | `false` | 是否启用代理 |
| `crawler.ip_proxy_pool_count` | `10` | 代理池大小 |
| `crawler.ip_proxy_provider` | `kuaidaili` | 服务商：`kuaidaili` / `wandouhttp` |
| `crawler.proxy_kdl_secret_id` | — | 快代理 SecretID（脱敏展示）|
| `crawler.proxy_kdl_signature` | — | 快代理 Signature（脱敏展示）|
| `crawler.proxy_kdl_username` | — | 快代理用户名 |
| `crawler.proxy_kdl_password` | — | 快代理密码（脱敏展示）|
| `crawler.proxy_wandou_app_key` | — | 万象 AppKey（脱敏展示）|

**C. 平台排序策略**（按平台分别配置）

| system_settings key | 可选值 | 说明 |
|---------------------|-------|------|
| `crawler.xhs.sort_type` | `time_descending` / `popularity_descending` / `general` | XHS 排序 |
| `crawler.dy.sort_type` | `0`/`1`/`2`（综合/最多赞/最新）| 抖音排序 |
| `crawler.dy.publish_time` | `0`/`1`/`7`/`180`（不限/一天/一周/半年）| 抖音发布时间 |
| `crawler.wb.search_type` | `default` / `real_time` / `popular` / `video` | 微博搜索类型 |
| `crawler.zhihu.sort` | ` `（默认）/ `created_time` / `upvoted_count` | 知乎排序 |
| `crawler.zhihu.search_time` | ` `（不限）/ `a_day` / `a_week` / `a_month` | 知乎时间范围 |

**D. 平台 Cookie**（按平台独立管理）

| system_settings key | 说明 |
|---------------------|------|
| `crawler.cookie.xhs` | 小红书 Cookie（脱敏展示）|
| `crawler.cookie.dy` | 抖音 Cookie（脱敏展示）|
| `crawler.cookie.ks` | 快手 Cookie（脱敏展示）|
| `crawler.cookie.bili` | B 站 Cookie（脱敏展示）|
| `crawler.cookie.wb` | 微博 Cookie（脱敏展示）|
| `crawler.cookie.tieba` | 贴吧 Cookie（脱敏展示）|
| `crawler.cookie.zhihu` | 知乎 Cookie（脱敏展示）|

### 4.2 配置流转路径

```
后台管理界面（前端）
  └─► PUT /api/admin/system/crawler（Go handler）
        └─► system_settings 表（MySQL 持久化）
              ↓
        爬虫触发时（Go CrawlerService.callMediaCrawlerAPI）
              └─► CrawlerConfigLoader.Load()：读取所有 crawler.* 键
                    └─► MediaCrawlerStartRequest（新增 30+ 字段）
                          └─► POST /api/crawler/start（Python FastAPI）
                                └─► CrawlerManager._build_command()
                                      ├─► CLI 参数：--max_notes_count, --sort_type...
                                      └─► env 变量：KDL_SECERT_ID, COOKIES_XHS...
                                            └─► subprocess.Popen(env={...})
                                                  └─► main.py 读取 config 和 os.environ
```

### 4.3 代理密钥安全方案

代理密钥绝不出现在命令行参数（防止 `ps aux` 泄露）。通过 `subprocess.Popen(env=...)` 注入环境变量：

```python
# crawler_manager.py - _build_env() 新方法
def _build_env(self, config: CrawlerStartRequest) -> dict:
    env = {**os.environ}                     # 继承当前环境
    if config.proxy_kdl_secret_id:
        env["KDL_SECERT_ID"]   = config.proxy_kdl_secret_id
        env["KDL_SIGNATURE"]   = config.proxy_kdl_signature
        env["KDL_USER_NAME"]   = config.proxy_kdl_username
        env["KDL_USER_PWD"]    = config.proxy_kdl_password
    if config.proxy_wandou_app_key:
        env["WANDOU_APP_KEY"]  = config.proxy_wandou_app_key
    # Cookie 同样走 env（覆盖 config.COOKIES，不出现在 ps 中）
    if config.cookies:
        env["CRAWLER_COOKIES"] = config.cookies
    return env

# Popen 调用时：
self.process = subprocess.Popen(
    cmd,
    env=self._build_env(config),    # ← 密钥走 env，不走 cmd
    ...
)
```

Python 代理库（`kuaidl_proxy.py`）本身已从 `os.environ` 读取 `KDL_SECERT_ID` 等变量，**无需改代理库代码**。

### 4.4 Cookie 管理方案

Cookie 通过扫码登录后手动复制粘贴到后台，存入 `system_settings`：

- 展示时脱敏（只显示前 20 个字符 + `***`）
- 写入时通过 `env["CRAWLER_COOKIES"]` 注入，`main.py` 启动时 `config.COOKIES = os.environ.get("CRAWLER_COOKIES", config.COOKIES)`
- Cookie 有有效期（通常 30-90 天），后台展示距离上次更新的天数，超过 60 天标红提示更新

---

## 五、改动文件清单

### Phase 1：配置动态化 + 排序修复（最高优先级）

| 文件 | 改动内容 |
|------|---------|
| `backend/src/api/handler/admin/system.go` | 新增 `GetCrawlerConfig` / `UpdateCrawlerConfig` handler（参考 `UpdateTagger` 模式）|
| `backend/src/service/crawler/config_loader.go` | 新文件：从 `system_settings` 读取所有 `crawler.*` 键，组装 `CrawlerDynamicConfig` 结构体 |
| `backend/src/service/crawler/service.go` | `callMediaCrawlerAPI()` 调用 `config_loader` 读取动态配置并注入请求 |
| `backend/src/api/router.go` | 注册 `GET/PUT /api/admin/system/crawler` 路由 |
| `MediaCrawler/api/schemas/crawler.py` | `CrawlerStartRequest` 新增：排序参数、代理开关、池大小、密钥字段（全部可选）|
| `MediaCrawler/api/services/crawler_manager.py` | 新增 `_build_env()` 方法；`Popen` 改为传 `env=self._build_env(config)` |
| `MediaCrawler/cmd_arg/arg.py` | 新增 `--max_notes_count`、`--sort_type`、`--publish_time_type`、`--weibo_search_type`、`--zhihu_sort`、`--zhihu_search_time` 参数，覆盖对应 config 值 |
| `MediaCrawler/config/xhs_config.py` | `SORT_TYPE = "time_descending"` |
| `MediaCrawler/config/dy_config.py` | 新增 `DY_SORT_TYPE = 2`；`PUBLISH_TIME_TYPE = 1` |
| `MediaCrawler/config/weibo_config.py` | `WEIBO_SEARCH_TYPE = "real_time"` |
| `MediaCrawler/config/zhihu_config.py` | 新增 `ZHIHU_SORT = "created_time"`；`ZHIHU_SEARCH_TIME = "a_day"` |
| `MediaCrawler/media_platform/zhihu/core.py` | `get_note_by_keyword()` 调用时传入 `sort=SearchSort(config.ZHIHU_SORT)` 和 `search_time=SearchTime(config.ZHIHU_SEARCH_TIME)` |
| `MediaCrawler/media_platform/douyin/core.py` | 传入 `sort_type=SearchSortType(config.DY_SORT_TYPE)` |
| `frontend-admin/src/pages/config/CrawlerConfigPage.tsx` | 新增"爬虫参数配置"Card（A+B+C 三组表单）和"平台 Cookie 管理"Card（D 组）|

### Phase 2：多平台并发（解决 CrawlerManager 单进程限制）

| 文件 | 改动内容 |
|------|---------|
| `MediaCrawler/api/schemas/crawler.py` | `CrawlerStartRequest` 已有字段；`CrawlerStatusResponse` 扩展为多平台状态 |
| `MediaCrawler/api/services/crawler_manager.py` | `process` → `processes: dict[str, Popen]`；`start()` 仅检查当前平台；`stop()` 支持指定平台；`get_status()` 聚合 |
| `MediaCrawler/api/routers/crawler.py` | `stop` 支持 `?platform=xhs` 参数 |
| `backend/src/service/crawler/service.go` | 多平台并发触发（goroutine per platform + WaitGroup）|

### Phase 3：每日多次调度

| 操作 | 说明 |
|------|------|
| 在工作流编辑器中创建 "全平台日常爬取" 工作流 | Cron 触发，平台列表含 7 个平台，启用并发模式 |
| 添加 4 个 Cron 触发：08:00 / 12:00 / 18:00 / 22:00 | 每次 50 篇 × 4 次 = 200 篇/天 |

---

## 六、后台管理界面设计

在现有 `CrawlerConfigPage.tsx` 新增两个 Card：

### Card 1：爬虫参数配置

```
┌────────────────────────────────────────────────────────┐
│ 爬取行为                                          [保存] │
│ 每次最多爬取数  [50]  并发数 [3]  间隔(秒) [1] - [3]    │
│ 爬取评论 [OFF]  爬取二级评论 [OFF]                       │
├────────────────────────────────────────────────────────┤
│ IP 代理池                                               │
│ 启用代理 [ON/OFF]  服务商 [快代理 ▼]  池大小 [10]        │
│ SecretID [****]  Signature [****]                       │
│ 用户名 [__]  密码 [****]                                │
├────────────────────────────────────────────────────────┤
│ 平台排序策略                                             │
│ XHS     [最新发布 ▼]                                    │
│ 抖音    排序 [最新 ▼]  时间 [最近一天 ▼]                 │
│ 微博    [实时搜索 ▼]                                    │
│ 知乎    排序 [最新发布 ▼]  时间范围 [最近一天 ▼]          │
│ 快手/B站/贴吧  （固定排序，不可配置）                     │
└────────────────────────────────────────────────────────┘
```

### Card 2：平台 Cookie 管理

```
┌────────────────────────────────────────────────────────┐
│ 平台 Cookie 管理                                         │
│ 扫码登录后，将浏览器 Cookie 粘贴到对应平台输入框          │
│                                                         │
│ 小红书  [abcde...***]  更新于 3 天前          [更新]      │
│ 抖音    [未配置]                              [配置]      │
│ 快手    [fffff...***]  更新于 45 天前 ⚠️过期  [更新]      │
│ B站     [xxxxx...***]  更新于 10 天前         [更新]      │
│ 微博    [未配置]                              [配置]      │
│ 贴吧    [bbbbb...***]  更新于 7 天前          [更新]      │
│ 知乎    [zzzzz...***]  更新于 2 天前          [更新]      │
└────────────────────────────────────────────────────────┘
```

---

## 七、合规与风控

### 排序方式与风控关系

时间排序每次返回的是**平台上真实最新发布的内容**，请求行为与正常用户浏览"最新"页面完全一致，风控风险比热度排序更低。

### 风控阈值参考（每平台单日请求上限）

| 平台 | 建议单日搜索次数 | 建议间隔 | 代理建议 |
|------|--------------|---------|---------|
| XHS  | < 500 次     | ≥ 1s    | 建议     |
| 抖音  | < 300 次     | ≥ 1.5s  | 强烈建议  |
| 知乎  | < 400 次     | ≥ 1s    | 可选     |
| B 站  | < 600 次     | ≥ 0.8s  | 可选     |
| 微博  | < 400 次     | ≥ 1s    | 建议     |
| 贴吧  | < 500 次     | ≥ 0.5s  | 可选     |
| 快手  | < 300 次     | ≥ 1.5s  | 建议     |

50 篇/次 × 4 次/天 × 平均 2 请求/篇 ≈ 400 次/天，在安全阈值内。

### 账号安全

- CDP 模式 + stealth.min.js，使用真实 Chrome 环境，反检测效果最好
- 各平台独立 `user_data_dir`，互不干扰
- Cookie 统一在后台管理，到期提示更新（超过 60 天标红）

---

## 八、资源消耗预估

| 维度 | 当前 | 优化后 |
|------|------|-------|
| 每日产出 | ~70 篇（7 平台合计） | ~1400 篇（7 平台合计）|
| 单次爬取耗时 | ~3-5 分钟（5 篇）| ~8-12 分钟（50 篇）|
| 7 平台总耗时/次 | ~35 分钟（串行）| ~12 分钟（并发）|
| 每日爬取总耗时 | ~35 分钟（1 次）| ~48 分钟（4 次×12 分钟）|
| 服务器内存需求 | ~300MB（1 Chrome）| ~2-3.5GB（7 Chrome 并发）|
| 配置变更方式 | 改代码 + 重启 | 后台填表单，秒生效 |
