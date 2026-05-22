# MediaCrawler MySQL 配置完成总结

## ✅ 配置状态

MediaCrawler 已成功配置为使用 MySQL 数据库 `opinion_analysis` 存储爬取数据。

### 数据库信息
- **数据库名**: opinion_analysis
- **主机**: 127.0.0.1:3306
- **用户**: root
- **已创建表**: 41 个（包含所有平台的数据表）

### 已验证功能
✓ 数据库连接正常
✓ 表结构创建成功
✓ 数据写入功能正常
✓ 数据读取功能正常

## 📊 数据库表结构

### 小红书 (XHS)
- `xhs_note` - 笔记内容
- `xhs_note_comment` - 笔记评论
- `xhs_creator` - 创作者信息

### 抖音 (Douyin)
- `douyin_aweme` - 视频内容
- `douyin_aweme_comment` - 视频评论
- `dy_creator` - 创作者信息

### B站 (Bilibili)
- `bilibili_video` - 视频内容
- `bilibili_video_comment` - 视频评论
- `bilibili_up_info` - UP主信息
- `bilibili_up_dynamic` - UP主动态
- `bilibili_contact_info` - 联系信息

### 微博 (Weibo)
- `weibo_note` - 微博内容
- `weibo_note_comment` - 微博评论
- `weibo_creator` - 创作者信息

### 快手 (Kuaishou)
- `kuaishou_video` - 视频内容
- `kuaishou_video_comment` - 视频评论

### 贴吧 (Tieba)
- `tieba_note` - 帖子内容
- `tieba_comment` - 帖子评论
- `tieba_creator` - 创作者信息

### 知乎 (Zhihu)
- `zhihu_content` - 内容（问题/回答/文章）
- `zhihu_comment` - 评论
- `zhihu_creator` - 创作者信息

### 其他业务表
- `articles` - 文章
- `topics` - 话题
- `daily_news` - 每日新闻
- `daily_topics` - 每日话题
- `alert_rules` - 告警规则
- `alert_records` - 告警记录
- `crawler_spider_configs` - 爬虫配置
- `crawler_run_logs` - 爬虫运行日志
- 等等...

## 🚀 快速使用

### 1. 配置爬虫参数

编辑 `config/base_config.py`:

```python
PLATFORM = "xhs"              # 平台: xhs | dy | ks | bili | wb | tieba | zhihu
KEYWORDS = "编程副业,编程兼职"  # 搜索关键词
CRAWLER_TYPE = "search"       # 爬取类型: search | detail | creator
SAVE_DATA_OPTION = "db"       # 保存到 MySQL 数据库
CRAWLER_MAX_NOTES_COUNT = 15  # 爬取数量
ENABLE_GET_COMMENTS = True    # 是否爬取评论
```

### 2. 运行爬虫

```bash
# 进入 MediaCrawler 目录
cd E:\Java\opinion_analysis\MediaCrawler

# 运行爬虫
python main.py
```

### 3. 查询数据

使用提供的查询脚本：

```bash
python query_data_example.py
```

或使用 SQL 直接查询：

```sql
-- 查询小红书笔记
SELECT title, nickname, liked_count, comment_count
FROM xhs_note
ORDER BY add_ts DESC
LIMIT 10;

-- 查询评论
SELECT nickname, content, like_count
FROM xhs_note_comment
ORDER BY add_ts DESC
LIMIT 10;
```

## 📁 相关文件

- `config/db_config.py` - 数据库配置
- `config/base_config.py` - 爬虫基础配置
- `database/models.py` - 数据库模型定义
- `store/xhs/_store_impl.py` - 小红书存储实现（其他平台类似）
- `test_mysql_connection.py` - MySQL 连接测试脚本
- `query_data_example.py` - 数据查询示例脚本
- `MYSQL_SETUP_CN.md` - 详细配置文档

## 💡 使用建议

1. **首次运行**: 需要扫码登录，登录状态会自动保存
2. **数据去重**: 使用 `db` 模式会自动去重，相同 ID 的数据会更新
3. **爬取频率**: 建议设置合理的爬取间隔，避免被平台限制
4. **数据备份**: 定期备份 MySQL 数据库

## ⚠️ 注意事项

- 仅用于学习研究，不得用于商业用途
- 遵守平台使用条款和 robots.txt 规则
- 合理控制请求频率，避免对平台造成压力
- 不得进行大规模爬取或运营干扰

## 🔧 故障排查

如遇到问题，请检查：
1. MySQL 服务是否启动
2. 数据库配置是否正确（`config/db_config.py`）
3. 依赖是否完整安装（`pip install -r requirements.txt`）
4. 查看详细文档：`MYSQL_SETUP_CN.md`

---

配置完成时间: 2026-05-22
