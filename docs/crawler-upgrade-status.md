# 爬虫模块升级状态

> 更新日期：2026-06-07  
> 范围：`MediaCrawler/` Python 爬虫服务 + `backend/src/service/crawler/` Go 调度层 + `frontend-admin/` 动态配置前端

---

## 一、模块现状

### 架构

```
工作流触发 / 手动触发
  └─► Go RunNode  (service/workflow/nodes/crawler/run.go)
        └─► Go CrawlerService  (service/crawler/service.go)
              │  从 system_settings 读取动态配置（DB）
              └─► HTTP POST /api/crawler/start
                    └─► Python CrawlerManager  (MediaCrawler/api/services/crawler_manager.py)
                          │  注入代理凭据环境变量
                          └─► subprocess: python main.py --platform xxx [参数...]
                                └─► 写入平台原始表 (xhs_note / douyin_aweme / ...)
                                      └─► Go 平台同步节点 → articles 表
```

### 支持平台

| 平台 | 代码 | Cookie 管理 | 登录方式 |
|------|------|------------|---------|
| 小红书 | xhs | ✅ | cookie |
| 抖音 | dy | ✅ | cookie |
| 快手 | ks | ✅ | cookie |
| B 站 | bili | ✅ | cookie |
| 微博 | wb | ✅ | cookie |
| 贴吧 | tieba | ✅ | cookie |
| 知乎 | zhihu | ✅ | cookie |

### 动态配置项（存 system_settings 表）

| 分类 | 配置项 | 说明 |
|------|-------|------|
| 爬取行为 | max_notes_count | 单次最大爬取数量 |
| 爬取行为 | max_concurrency_num | 并发详情请求数 |
| 爬取行为 | sleep_sec_min / max | 请求间隔随机范围（秒） |
| 爬取行为 | enable_comments / enable_sub_comments | 是否抓取评论 |
| IP 代理 | enable_ip_proxy | 总开关 |
| IP 代理 | ip_proxy_pool_count | 代理池大小 |
| IP 代理 | ip_proxy_provider | kuaidaili / wandouhttp |
| IP 代理 | proxy_kdl_* (4 个字段) | 快代理凭据 |
| IP 代理 | proxy_wandou_app_key | 豌豆 HTTP 凭据 |
| 平台排序 | xhs_sort_type | general / time_descending / popularity_descending |
| 平台排序 | weibo_search_type | default / real_time / popular / video |
| 平台排序 | dy_sort_type | 0 综合 / 1 最多点赞 / 2 最新 |
| 平台排序 | zhihu_sort / zhihu_search_time | 知乎排序 + 时间范围 |
| Cookie | cookie.{platform} × 7 | 各平台登录 Cookie（脱敏展示） |

---

## 二、开发进度

### 已完成

| 阶段 | 内容 | 关键文件 |
|------|------|---------|
| **Stage 1** | 代码梳理，确认瓶颈（硬编码、串行、单次 5 篇） | `docs/crawler-analysis.md` |
| **Stage 2** | 设计工业级升级方案 | `docs/crawler-industrial-design.md` |
| **Stage 3** | Python 侧 CLI 参数扩展（性能 + 平台排序 + IP 代理开关） | `MediaCrawler/cmd_arg/arg.py` |
| **Stage 4** | Go 后端动态配置：`system_settings` 读写、Cookie 脱敏、`GET/PUT /api/admin/system/crawler` 接口 | `backend/src/repository/crawler_config.go`<br>`backend/src/api/handler/admin/system.go` |
| **Stage 5** | 代理凭据经环境变量注入子进程：`_build_env()`；Go 侧将 DB 代理配置透传给 Python API | `MediaCrawler/api/services/crawler_manager.py`<br>`MediaCrawler/api/schemas/crawler.py`<br>`backend/src/service/crawler/service.go` |
| **Stage 6** | 管理前端爬虫配置页：爬取参数 + IP 代理 + 平台排序 + Cookie 管理四卡，路由 `/config/crawler` 已注册 | `frontend-admin/src/pages/config/CrawlerConfigPage.tsx`<br>`frontend-admin/src/api/admin-crawler.ts` |

### 待完成

| 阶段 | 内容 | 预估工作量 |
|------|------|---------|
| **Stage 7** | 多平台并发爬取：Python 侧 `CrawlerManager` 改为 `processes: dict[str, Popen]`，Go 侧 goroutine per platform，状态聚合 | 大 |
| **Stage 8** | Cron 定时调度：工作流编辑器支持 Cron Trigger 节点，自动触发多平台爬取 | 中 |

---

## 三、关键设计决策

**动态配置而非硬编码**  
所有运行参数存 `system_settings` 表，key 前缀 `crawler.*`。Go 服务启动爬虫时从 DB 读取，无需重启服务生效。

**代理凭据传递方式**  
选择环境变量而非 CLI 参数，原因：MediaCrawler 的代理 provider（kuaidl_proxy.py / wandou_http_proxy.py）已内置 `os.getenv` 读取，环境变量方式零侵入，且不会把凭据暴露在进程列表的命令行参数里。

**Cookie 脱敏**  
前端 GET 接口返回 `{ set: bool, masked: string }`，明文只在 PUT 写入时通过 HTTPS 传输，不回显。Go 侧 `BuildCrawlerConfigResponse` 用 `utils.MaskString` 处理。

**IP 代理凭据不允许单次 Trigger 覆盖**  
代理凭据敏感且与账号绑定，只从 DB 读取，工作流节点的 `TriggerParams` 不暴露这些字段，避免误操作。
