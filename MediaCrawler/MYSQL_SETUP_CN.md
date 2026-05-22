# MediaCrawler MySQL 配置说明

## 📋 配置概览

MediaCrawler 已配置为使用本地 MySQL 数据库 `opinion_analysis` 存储爬取的数据。

### 数据库配置信息

- **数据库类型**: MySQL
- **主机地址**: 127.0.0.1
- **端口**: 3306
- **数据库名**: opinion_analysis
- **用户名**: root
- **密码**: 123456

配置文件位置: `config/db_config.py`

## 🚀 快速开始

### 1. 安装依赖

```bash
# 使用 pip 安装
pip install -r requirements.txt

# 或使用 uv（推荐）
uv pip install -r requirements.txt
```

### 2. 初始化数据库表结构

```bash
# 初始化 MySQL 数据库表
python main.py --init_db mysql
```

这将自动创建以下表：
- `xhs_note` - 小红书笔记
- `xhs_note_comment` - 小红书评论
- `xhs_creator` - 小红书创作者
- `douyin_aweme` - 抖音视频
- `douyin_aweme_comment` - 抖音评论
- `bilibili_video` - B站视频
- `bilibili_video_comment` - B站评论
- `weibo_note` - 微博笔记
- `weibo_note_comment` - 微博评论
- `kuaishou_video` - 快手视频
- `kuaishou_video_comment` - 快手评论
- `tieba_note` - 贴吧帖子
- `tieba_comment` - 贴吧评论
- `zhihu_content` - 知乎内容
- `zhihu_comment` - 知乎评论

### 3. 配置爬虫参数

编辑 `config/base_config.py`:

```python
# 选择平台
PLATFORM = "xhs"  # xhs | dy | ks | bili | wb | tieba | zhihu

# 搜索关键词
KEYWORDS = "编程副业,编程兼职"

# 爬取类型
CRAWLER_TYPE = "search"  # search | detail | creator

# 数据保存方式
SAVE_DATA_OPTION = "db"  # 使用 MySQL 数据库

# 爬取数量
CRAWLER_MAX_NOTES_COUNT = 15

# 是否爬取评论
ENABLE_GET_COMMENTS = True

# 每个帖子爬取的评论数
CRAWLER_MAX_COMMENTS_COUNT_SINGLENOTES = 10
```

### 4. 运行爬虫

```bash
python main.py
```

## 🧪 测试 MySQL 连接

运行测试脚本验证数据库配置：

```bash
python test_mysql_connection.py
```

测试脚本会：
1. ✓ 验证数据库连接
2. ✓ 创建数据库和表结构
3. ✓ 测试数据写入
4. ✓ 测试数据读取
5. ✓ 显示表结构信息

## 📊 数据表结构

### 小红书笔记表 (xhs_note)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | Integer | 主键ID |
| note_id | String(255) | 笔记ID（唯一） |
| user_id | String(255) | 用户ID |
| nickname | Text | 用户昵称 |
| title | Text | 笔记标题 |
| desc | Text | 笔记描述 |
| liked_count | Text | 点赞数 |
| collected_count | Text | 收藏数 |
| comment_count | Text | 评论数 |
| share_count | Text | 分享数 |
| note_url | Text | 笔记URL |
| source_keyword | Text | 来源关键词 |
| create_time | BigInteger | 创建时间戳 |

### 小红书评论表 (xhs_note_comment)

| 字段 | 类型 | 说明 |
|------|------|------|
| id | Integer | 主键ID |
| comment_id | String(255) | 评论ID |
| note_id | String(255) | 笔记ID |
| user_id | String(255) | 用户ID |
| nickname | Text | 用户昵称 |
| content | Text | 评论内容 |
| like_count | Text | 点赞数 |
| sub_comment_count | Integer | 子评论数 |
| parent_comment_id | String(255) | 父评论ID |
| create_time | BigInteger | 创建时间戳 |

## 🔧 高级配置

### 修改数据库配置

如需修改数据库配置，编辑 `config/db_config.py`:

```python
# MySQL 配置
MYSQL_DB_PWD = "your_password"
MYSQL_DB_USER = "your_username"
MYSQL_DB_HOST = "127.0.0.1"
MYSQL_DB_PORT = 3306
MYSQL_DB_NAME = "opinion_analysis"
```

或使用环境变量：

```bash
export MYSQL_DB_PWD="your_password"
export MYSQL_DB_USER="your_username"
export MYSQL_DB_HOST="127.0.0.1"
export MYSQL_DB_PORT=3306
export MYSQL_DB_NAME="opinion_analysis"
```

### 切换存储方式

在 `config/base_config.py` 中修改 `SAVE_DATA_OPTION`:

```python
SAVE_DATA_OPTION = "db"      # MySQL 数据库
# SAVE_DATA_OPTION = "sqlite"  # SQLite 数据库
# SAVE_DATA_OPTION = "json"    # JSON 文件
# SAVE_DATA_OPTION = "jsonl"   # JSONL 文件
# SAVE_DATA_OPTION = "csv"     # CSV 文件
# SAVE_DATA_OPTION = "excel"   # Excel 文件
```

## 📝 使用示例

### 示例 1: 爬取小红书关键词搜索结果

```python
# config/base_config.py
PLATFORM = "xhs"
KEYWORDS = "Python编程,机器学习"
CRAWLER_TYPE = "search"
SAVE_DATA_OPTION = "db"
CRAWLER_MAX_NOTES_COUNT = 20
ENABLE_GET_COMMENTS = True
```

运行：
```bash
python main.py
```

### 示例 2: 爬取指定小红书笔记详情

```python
# config/xhs_config.py
XHS_SPECIFIED_ID_LIST = [
    "6411cf2d000000001300d6db",
    "64116c0b000000001300d6db"
]

# config/base_config.py
PLATFORM = "xhs"
CRAWLER_TYPE = "detail"
SAVE_DATA_OPTION = "db"
```

### 示例 3: 爬取创作者主页

```python
# config/xhs_config.py
XHS_CREATOR_URL_LIST = [
    "https://www.xiaohongshu.com/user/profile/5ff0e6410000000001008400"
]

# config/base_config.py
PLATFORM = "xhs"
CRAWLER_TYPE = "creator"
SAVE_DATA_OPTION = "db"
```

## 🔍 查询数据

### 使用 Python 查询

```python
import asyncio
from database.db_session import get_session
from database.models import XhsNote
from sqlalchemy import select

async def query_notes():
    async with get_session() as session:
        # 查询所有笔记
        stmt = select(XhsNote).limit(10)
        result = await session.execute(stmt)
        notes = result.scalars().all()
        
        for note in notes:
            print(f"标题: {note.title}")
            print(f"点赞数: {note.liked_count}")
            print(f"评论数: {note.comment_count}")
            print("-" * 50)

asyncio.run(query_notes())
```

### 使用 SQL 查询

```sql
-- 查询点赞数最高的笔记
SELECT title, nickname, liked_count, comment_count
FROM xhs_note
ORDER BY CAST(liked_count AS UNSIGNED) DESC
LIMIT 10;

-- 查询某个关键词的笔记
SELECT title, desc, liked_count
FROM xhs_note
WHERE source_keyword = '编程副业'
ORDER BY add_ts DESC;

-- 统计评论数
SELECT COUNT(*) as total_comments
FROM xhs_note_comment;
```

## ⚠️ 注意事项

1. **首次运行**: 必须先执行 `python main.py --init_db mysql` 初始化表结构
2. **登录状态**: 首次运行需要扫码登录，登录状态会保存在 `{platform}_user_data_dir` 目录
3. **爬取频率**: 请合理控制爬取频率，避免对平台造成压力
4. **数据去重**: 使用 `db` 模式会自动去重，相同 ID 的数据会更新而不是重复插入
5. **合法使用**: 仅用于学习研究，不得用于商业用途

## 🐛 常见问题

### 1. 连接数据库失败

检查：
- MySQL 服务是否启动
- 数据库配置是否正确
- 用户权限是否足够

### 2. 表不存在

运行初始化命令：
```bash
python main.py --init_db mysql
```

### 3. 依赖安装失败

尝试使用国内镜像：
```bash
pip install -r requirements.txt -i https://pypi.tuna.tsinghua.edu.cn/simple
```

### 4. 爬虫无法登录

- 关闭无头模式: `HEADLESS = False`
- 手动完成验证码
- 检查浏览器驱动是否正常

## 📚 相关文档

- [MediaCrawler 主文档](README.md)
- [数据库模型定义](database/models.py)
- [存储实现](store/)
- [配置说明](config/)

## 🤝 支持

如有问题，请提交 Issue 或查看项目文档。
