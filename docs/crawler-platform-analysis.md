# 爬虫平台架构分析文档

## 框架总览

本项目基于 [MediaCrawler](https://github.com/NanmiCoder/MediaCrawler) 框架，使用 **Playwright + httpx** 双引擎架构爬取 7 个社交平台的内容和评论。

### 统一架构模式

所有平台共享相同的代码结构和生命周期：

```
启动浏览器（Playwright/CDP）
  → 检测登录状态
  → 未登录则执行登录（QR码/Cookie/手机号）
  → 从浏览器获取 Cookie
  → 将 Cookie 注入 httpx 客户端
  → 根据 CRAWLER_TYPE 执行对应爬取模式
  → 数据通过 Store 层写入 MySQL（upsert 去重）
```

### 文件结构（每个平台）

```
media_platform/{platform}/
├── core.py       — 爬虫主类（继承 AbstractCrawler）
├── client.py     — HTTP API 客户端（请求签名 + 接口封装）
├── field.py      — 枚举常量（搜索类型、排序方式）
├── help.py       — 数据提取/转换器
├── login.py      — 登录实现（继承 AbstractLogin）
└── __init__.py

config/{platform}_config.py   — 平台特有配置项
model/m_{platform}.py         — Pydantic/SQLAlchemy 数据模型
store/{platform}/             — 存储实现（DB upsert / CSV / JSON）
```

### 通用配置（base_config.py）

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| CRAWLER_MAX_NOTES_COUNT | 50 | 最大爬取文章/帖子数（翻页上限） |
| CRAWLER_MAX_COMMENTS_COUNT_SINGLENOTES | 50 | 每篇帖子最大一级评论数 |
| MAX_CONCURRENCY_NUM | 3 | 并发请求数 |
| CRAWLER_MAX_SLEEP_SEC | (1, 3) | 请求间随机睡眠范围（秒） |
| ENABLE_CDP_MODE | True | 是否使用 CDP 模式（连接用户真实浏览器） |
| SAVE_DATA_OPTION | "db" | 存储方式（db/csv/json/jsonl） |
| CRAWLER_TYPE | "search" | 爬取模式（search/detail/creator） |

### 三种爬取模式

| 模式 | 触发方式 | 数据来源 |
|------|----------|----------|
| search | 关键词搜索 | 平台搜索 API，按关键词翻页 |
| detail | 指定 ID | 直接访问帖子详情页/API |
| creator | 创作者主页 | 获取指定用户的所有内容 |

---

## 各平台详细分析

---

### 1. 小红书 (xhs)

| 维度 | 说明 |
|------|------|
| 入口 URL | `https://www.xiaohongshu.com` / `https://www.rednote.com`（海外版） |
| API 基础地址 | `https://edith.xiaohongshu.com` / `https://webapi.rednote.com` |
| 数据获取方式 | **Playwright 获取 Cookie → httpx 调 API** |
| 请求签名 | 需要 `x-s`、`x-t` 签名（通过浏览器端 JS 生成，Playwright 执行页面脚本获取） |
| 登录方式 | QR 码扫码 / Cookie 注入 / 手机号 |
| 搜索分页 | `/api/sns/web/v1/search/notes`，每页 20 条，支持排序 |
| 评论获取 | `/api/sns/web/v2/comment/page`，游标翻页 |
| 反爬特点 | x-s 签名算法加密复杂，需要定期更新；IP 封禁严格 |
| 特殊处理 | 搜索需要 `search_id` 参数（从页面 JS 解析获取） |

**数据模型字段：** note_id, title, desc, liked_count, collected_count, comment_count, share_count, create_time, user_id, nickname, avatar, image_list, tag_list, source_keyword

---

### 2. 抖音 (douyin / dy)

| 维度 | 说明 |
|------|------|
| 入口 URL | `https://www.douyin.com` |
| API 基础地址 | `https://www.douyin.com`（同域 API） |
| 数据获取方式 | **Playwright 获取 Cookie + ttwid → httpx 调 API** |
| 请求签名 | 需要 `a_bogus` 参数（通过 Playwright 执行页面 JS 生成签名） |
| 登录方式 | QR 码扫码 / Cookie 注入 |
| 搜索分页 | `/aweme/v1/web/search/item/`，offset 翻页 |
| 评论获取 | `/aweme/v1/web/comment/list/`，cursor 翻页 |
| 反爬特点 | a_bogus 签名极其复杂（需调用页面 JS 运行时）；ttwid Cookie 必需 |
| 特殊处理 | 必须先访问首页获取 ttwid；签名通过 `page.evaluate()` 在浏览器中执行 |

**数据模型字段：** aweme_id, aweme_type, title, desc, create_time, user_id, nickname, avatar, liked_count, collected_count, comment_count, share_count, video_play_count, source_keyword

---

### 3. 快手 (kuaishou / ks)

| 维度 | 说明 |
|------|------|
| 入口 URL | `https://www.kuaishou.com` |
| API 基础地址 | `https://www.kuaishou.com/graphql`（**GraphQL 接口**） |
| 数据获取方式 | **Playwright 获取 Cookie → httpx 发 GraphQL 请求** |
| 请求签名 | Cookie 中的 `did` + `kpn` 等字段即可，无额外签名 |
| 登录方式 | QR 码扫码 / Cookie 注入 |
| 搜索分页 | GraphQL query: `search_query`，pcursor 翻页 |
| 评论获取 | GraphQL query: `commentListQuery`，pcursor 翻页 |
| 反爬特点 | GraphQL 接口相对稳定；但频率控制严格，需较长 sleep |
| 特殊处理 | 所有请求走 GraphQL POST，query 字符串预定义在 `graphql.py` 中 |

**数据模型字段：** video_id, video_type, title, desc, create_time, user_id, nickname, avatar, liked_count, viewCount, video_url, video_cover_url, source_keyword

---

### 4. B站 (bilibili / bili)

| 维度 | 说明 |
|------|------|
| 入口 URL | `https://www.bilibili.com` |
| API 基础地址 | `https://api.bilibili.com` |
| 数据获取方式 | **Playwright 获取 Cookie → httpx 调 REST API** |
| 请求签名 | 需要 `wbi` 签名（从 nav API 获取 img_key + sub_key，MD5 混淆生成 w_rid） |
| 登录方式 | QR 码扫码 / Cookie 注入 |
| 搜索分页 | `/x/web-interface/wbi/search/type`，page 参数翻页，每页 20 |
| 评论获取 | `/x/v2/reply/main`，游标翻页 |
| 反爬特点 | wbi 签名算法需定期更新（B站会换 mixin_key 的字符映射表） |
| 特殊处理 | 搜索前需先调 `/x/web-interface/nav` 获取签名密钥对 |

**数据模型字段：** video_id, video_url, title, desc, create_time, user_id, nickname, avatar, liked_count, video_play_count, video_favorite_count, video_share_count, video_coin_count, video_danmaku, video_comment, video_cover_url, source_keyword

---

### 5. 微博 (weibo / wb)

| 维度 | 说明 |
|------|------|
| 入口 URL | `https://www.weibo.com`（PC）/ `https://m.weibo.cn`（移动端） |
| API 基础地址 | `https://m.weibo.cn`（**使用移动端 API，更稳定**） |
| 数据获取方式 | **Playwright 获取 Cookie → httpx 调移动端 API** |
| 请求签名 | 无额外签名，Cookie 认证即可 |
| 登录方式 | QR 码扫码 / Cookie 注入 / 手机号 |
| 搜索分页 | `/api/container/getIndex`，page 参数翻页 |
| 评论获取 | `/api/comments/show`，max_id 翻页 |
| 反爬特点 | IP 限流严格；移动端 API 比 PC 端稳定；需 mobile UA |
| 特殊处理 | 使用 mobile user-agent 访问 m.weibo.cn；搜索结果需要 `filter_search_result_card` 过滤广告卡片 |

**数据模型字段：** note_id, content, create_time, create_date_time, user_id, nickname, avatar, liked_count, comments_count, shared_count, source_keyword, note_url

---

### 6. 百度贴吧 (tieba)

| 维度 | 说明 |
|------|------|
| 入口 URL | `https://tieba.baidu.com` |
| API 基础地址 | `https://tieba.baidu.com`（同域，Web API） |
| 数据获取方式 | **Playwright 获取 Cookie → httpx 调贴吧 Web API** |
| 请求签名 | 无额外签名，BDUSS Cookie 认证 |
| 登录方式 | QR 码扫码 / Cookie 注入 |
| 搜索分页 | `/f/search/res`，pn 参数翻页，每页 10 条固定 |
| 帖子详情 | HTML 页面解析 + `/p/{tid}` 接口 |
| 评论获取 | 帖子内容本身就是楼层回复，按页翻取 |
| 反爬特点 | 反检测较轻，但需注入 anti-detection 脚本；需从百度首页跳转进入避免验证 |
| 特殊处理 | 先访问 `baidu.com` 再跳转 `tieba.baidu.com`（`_navigate_to_tieba_via_baidu`）；帖子内容是 HTML 需要 `TieBaExtractor` 解析 |

**数据模型字段：** note_id, title, desc, note_url, user_id, nickname, avatar, liked_count, reply_count, tieba_name, tieba_id, create_time, source_keyword

**评论字段：** comment_id, content, user_id, nickname, avatar, like_count, reply_count, create_time, sub_comment_count, note_id, note_url

---

### 7. 知乎 (zhihu)

| 维度 | 说明 |
|------|------|
| 入口 URL | `https://www.zhihu.com` |
| API 基础地址 | `https://www.zhihu.com/api/v4`（同域 API） |
| 数据获取方式 | **Playwright 获取 Cookie + d_c0 → httpx 调 API（带签名）** |
| 请求签名 | 需要 `x-zst-81` + `x-zse-96` 双签名（基于 d_c0 Cookie + URL 的 HMAC） |
| 登录方式 | QR 码扫码 / Cookie 注入 |
| 搜索分页 | `/api/v4/search_v3`，offset 翻页，支持排序和时间筛选 |
| 评论获取 | `/api/v4/comment_v5/answers/{id}/root_comment` 或 `/articles/{id}/root_comment` |
| 反爬特点 | 签名算法 `x-zse-96` 通过 JS 运行时计算；必须先访问搜索页获取搜索相关 Cookie |
| 特殊处理 | 启动后必须导航到搜索页等待 5 秒获取搜索 Cookie（`/search?q=python`）；支持三种内容类型（问答/文章/视频） |

**数据模型字段：** content_id, content_type (answer/article/video), content_url, title, desc, create_time, user_id, nickname, avatar, liked_count, source_keyword

**评论字段：** comment_id, content, create_time, user_id, nickname, avatar, like_count, sub_comment_count, note_id

**配置项：**
- `ZHIHU_SORT`: 排序（created_time/upvoted_count）
- `ZHIHU_SEARCH_TIME`: 时间范围（a_day/a_week/a_month）

---

## 横向对比

| 平台 | API 类型 | 签名复杂度 | 反爬强度 | 评论翻页方式 | 特殊依赖 |
|------|----------|------------|----------|--------------|----------|
| 小红书 | REST | ★★★★★ | 高 | 游标 cursor | x-s 签名算法（浏览器 JS） |
| 抖音 | REST | ★★★★★ | 高 | 游标 cursor | a_bogus（浏览器 JS 运行时） |
| 快手 | GraphQL | ★★☆☆☆ | 中 | pcursor | GraphQL query 预定义 |
| B站 | REST | ★★★☆☆ | 中 | 游标 next | wbi 签名（MD5，定期换 key） |
| 微博 | REST | ★☆☆☆☆ | 中 | max_id | 移动端 API + mobile UA |
| 贴吧 | Web/REST | ★☆☆☆☆ | 低 | page number | HTML 解析 + 百度跳转 |
| 知乎 | REST | ★★★★☆ | 中高 | offset | x-zse-96 签名 + 搜索页预热 |

---

## 存储层设计

所有平台共享统一的存储模式：

```python
# Upsert 模式（按唯一 ID 去重）
async def store_content(self, content_item: Dict):
    stmt = select(Model).where(Model.note_id == note_id)
    existing = await session.execute(stmt)
    if existing:
        # 更新已有记录
        for key, value in content_item.items():
            setattr(existing, key, value)
    else:
        # 新增记录
        content_item["add_ts"] = get_current_timestamp()
        session.add(Model(**content_item))
    await session.commit()
```

**去重策略：** 基于平台原生 ID（note_id / video_id / content_id），同一内容多次爬取只更新不新增。

**注意：** 去重只防止重复插入，不过滤老数据。爬虫拿到什么就存什么，无日期过滤机制。

---

## Go 后端集成点

爬虫数据存入 MySQL 后，Go 后端通过以下方式消费：

| 组件 | 文件 | 职责 |
|------|------|------|
| platform_data.go | `backend/src/repository/` | 按平台查询帖子/文章，映射到统一 Article 模型 |
| platform_comment.go | `backend/src/repository/` | 按平台查询评论，映射到统一 ArticleComment 模型 |
| platform_sync 节点 | `backend/src/service/workflow/nodes/` | 工作流中将平台源表数据同步到中心 articles 表 |

每新增一个平台需要在这三处增加 `case "platform_code"` 分支。

---

## 新平台接入清单

接入一个新平台需要完成以下步骤：

### Python 爬虫层
- [ ] `config/{platform}_config.py` — 平台配置
- [ ] `model/m_{platform}.py` — 数据模型
- [ ] `media_platform/{platform}/core.py` — 主爬虫类
- [ ] `media_platform/{platform}/client.py` — API 客户端
- [ ] `media_platform/{platform}/field.py` — 枚举常量
- [ ] `media_platform/{platform}/help.py` — 数据提取器
- [ ] `media_platform/{platform}/login.py` — 登录逻辑
- [ ] `store/{platform}/_store_impl.py` — 存储实现
- [ ] `database/models.py` — 增加表定义
- [ ] `main.py` — 注册平台爬虫类

### Go 后端层
- [ ] `repository/platform_data.go` — 新增平台查询分支
- [ ] `repository/platform_comment.go` — 新增评论查询分支
- [ ] 工作流节点配置 — 平台列表增加新选项
- [ ] 前端下拉菜单 — 平台选择器增加新项
