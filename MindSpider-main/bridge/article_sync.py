#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
数据桥接模块 - 将 MindSpider 数据同步到 opinion_analysis.articles 表
"""

import sys
import json
from datetime import datetime, date, timezone
from pathlib import Path
from typing import List, Dict, Optional
import pymysql
from pymysql.cursors import DictCursor

# 添加项目根目录到路径
project_root = Path(__file__).parent.parent
sys.path.append(str(project_root))

try:
    import config
except ImportError:
    raise ImportError("无法导入config.py配置文件")

# 简单的情感分析（使用 SnowNLP 和 jieba）
try:
    import jieba.analyse
    from snownlp import SnowNLP
    SENTIMENT_AVAILABLE = True
except ImportError:
    SENTIMENT_AVAILABLE = False
    print("警告: SnowNLP 或 jieba 未安装，将使用简化的情感分析")


def analyse_sentiment(title: str, content: str) -> tuple:
    """
    情感分析

    Returns:
        (sentiment_label, sent_score, keywords_json)
    """
    if not SENTIMENT_AVAILABLE:
        # 简化版本：返回中性情感
        return "neutral", 0.0, "[]"

    text = (title + "。" + content).strip()
    if not text or text == "。":
        return "neutral", 0.0, "[]"

    try:
        # 使用 SnowNLP 计算情感分数
        raw = SnowNLP(text).sentiments  # 0~1
        sent_score = round(raw * 2 - 1, 4)  # 映射到 -1~1

        # 判断情感标签
        if raw > 0.6:
            label = "positive"
        elif raw < 0.4:
            label = "negative"
        else:
            label = "neutral"

        # 提取关键词
        keywords = jieba.analyse.extract_tags(text, topK=10)
        keywords_json = json.dumps(keywords, ensure_ascii=False)

        return label, sent_score, keywords_json
    except Exception as e:
        print(f"情感分析失败: {e}")
        return "neutral", 0.0, "[]"


class ArticleSyncBridge:
    """文章同步桥接器"""

    def __init__(self, target_dsn: str):
        """
        初始化桥接器

        Args:
            target_dsn: 目标数据库连接字符串 (GORM格式或标准格式)
        """
        self.target_conn = None
        self.source_conn = None
        self.target_dsn = target_dsn
        self.connect()

    def connect(self):
        """连接数据库"""
        # 连接目标数据库 (opinion_analysis)
        target_config = self._parse_dsn(self.target_dsn)
        try:
            self.target_conn = pymysql.connect(
                host=target_config['host'],
                port=target_config['port'],
                user=target_config['user'],
                password=target_config['password'],
                database=target_config['database'],
                charset='utf8mb4',
                autocommit=True,
                cursorclass=DictCursor
            )
            print(f"成功连接到目标数据库: {target_config['database']}")
        except Exception as e:
            print(f"目标数据库连接失败: {e}")
            raise

        # 连接源数据库 (mindspider)
        try:
            self.source_conn = pymysql.connect(
                host=config.DB_HOST,
                port=config.DB_PORT,
                user=config.DB_USER,
                password=config.DB_PASSWORD,
                database=config.DB_NAME,
                charset=config.DB_CHARSET,
                autocommit=True,
                cursorclass=DictCursor
            )
            print(f"成功连接到源数据库: {config.DB_NAME}")
        except Exception as e:
            print(f"源数据库连接失败: {e}")
            raise

    def _parse_dsn(self, dsn: str) -> Dict:
        """解析 DSN 连接字符串"""
        import re

        # GORM 格式: user:pass@tcp(host:port)/db
        m = re.match(r"(.+):(.+)@tcp\((.+):(\d+)\)/(\w+)", dsn)
        if m:
            user, pwd, host, port, db = m.groups()
            return {
                'user': user,
                'password': pwd,
                'host': host,
                'port': int(port),
                'database': db
            }

        raise ValueError(f"无法解析 DSN 格式: {dsn}")

    def close(self):
        """关闭数据库连接"""
        if self.target_conn:
            self.target_conn.close()
            print("目标数据库连接已关闭")
        if self.source_conn:
            self.source_conn.close()
            print("源数据库连接已关闭")

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()

    def get_or_create_source(self, platform: str, source_name: str) -> int:
        """
        获取或创建数据源

        Args:
            platform: 平台类型 (weibo/zhihu/news等)
            source_name: 数据源名称

        Returns:
            数据源ID
        """
        cursor = self.target_conn.cursor()

        # 查找现有数据源
        query = "SELECT id FROM data_sources WHERE name = %s AND type = %s"
        cursor.execute(query, (source_name, platform))
        result = cursor.fetchone()

        if result:
            return result['id']

        # 创建新数据源
        insert_query = """
            INSERT INTO data_sources (name, type, status, created_at, updated_at)
            VALUES (%s, %s, 1, %s, %s)
        """
        now = datetime.now()
        cursor.execute(insert_query, (source_name, platform, now, now))

        return cursor.lastrowid

    def sync_daily_news_to_articles(self, crawl_date: date = None, keywords: list = None,
                                    start_at: str = None, end_at: str = None) -> int:
        """
        同步每日新闻到 articles 表

        Args:
            crawl_date: 爬取日期，默认为今天；当 start_at/end_at 指定时忽略此参数
            keywords: 关键词列表，非空时只同步标题包含任一关键词的新闻
            start_at: ISO 8601 起始时间字符串，按 crawl_date 过滤
            end_at: ISO 8601 结束时间字符串，按 crawl_date 过滤

        Returns:
            同步的文章数量
        """
        if not crawl_date:
            crawl_date = date.today()

        # 解析时间范围（取日期部分与 crawl_date 比较）
        start_date = None
        end_date = None
        if start_at:
            try:
                start_date = datetime.fromisoformat(start_at.replace('Z', '+00:00')).date()
            except (ValueError, AttributeError):
                start_date = None
        if end_at:
            try:
                end_date = datetime.fromisoformat(end_at.replace('Z', '+00:00')).date()
            except (ValueError, AttributeError):
                end_date = None

        print(f"\n开始同步新闻到 articles 表...")

        # 从 mindspider.daily_news 读取数据
        source_cursor = self.source_conn.cursor()
        conditions = []
        params = []

        if start_date and end_date:
            conditions.append("crawl_date BETWEEN %s AND %s")
            params.extend([start_date, end_date])
        elif start_date:
            conditions.append("crawl_date >= %s")
            params.append(start_date)
        elif end_date:
            conditions.append("crawl_date <= %s")
            params.append(end_date)
        else:
            conditions.append("crawl_date = %s")
            params.append(crawl_date)

        if keywords:
            kw_conditions = " OR ".join(["title LIKE %s"] * len(keywords))
            conditions.append(f"({kw_conditions})")
            params.extend([f"%{kw}%" for kw in keywords])
            print(f"关键词过滤: {keywords}")

        where_clause = " AND ".join(conditions)
        query = f"""
            SELECT news_id, source_platform, title, url, crawl_date, rank_position
            FROM daily_news
            WHERE {where_clause}
            ORDER BY rank_position ASC
        """
        source_cursor.execute(query, params)
        news_items = source_cursor.fetchall()

        if not news_items:
            print(f"没有找到 {crawl_date} 的新闻数据")
            return 0

        print(f"找到 {len(news_items)} 条新闻，开始同步...")

        # 平台名称映射
        platform_names = {
            "weibo": "微博热搜",
            "zhihu": "知乎热榜",
            "bilibili-hot-search": "B站热搜",
            "toutiao": "今日头条",
            "douyin": "抖音热榜",
            "github-trending-today": "GitHub趋势",
            "coolapk": "酷安热榜",
            "tieba": "百度贴吧",
            "wallstreetcn": "华尔街见闻",
            "thepaper": "澎湃新闻",
            "cls-hot": "财联社",
            "xueqiu": "雪球热榜",
            "kuaishou": "快手热榜"
        }

        target_cursor = self.target_conn.cursor()
        synced_count = 0

        for news in news_items:
            try:
                origin_url = news['url'] or ''

                # 检查是否已存在（根据 URL 去重）
                if origin_url:
                    check_query = "SELECT id FROM articles WHERE origin_url = %s"
                    target_cursor.execute(check_query, (origin_url,))
                    if target_cursor.fetchone():
                        continue  # 已存在，跳过

                # 获取或创建数据源
                platform = news['source_platform']
                source_name = platform_names.get(platform, platform)
                source_id = self.get_or_create_source('news', source_name)

                # 情感分析
                title = news['title'] or ''
                content = ''  # daily_news 表没有 content 字段
                sentiment, sent_score, keywords = analyse_sentiment(title, content)

                # 插入到 articles 表
                insert_query = """
                    INSERT INTO articles (
                        source_id, title, content, author, origin_url, platform,
                        sentiment, sent_score, keywords, published_at, created_at, updated_at
                    ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
                """

                now = datetime.now()
                published_at = datetime.combine(news['crawl_date'], datetime.min.time())

                target_cursor.execute(insert_query, (
                    source_id,
                    title,
                    content,
                    '',  # author
                    origin_url,
                    platform,
                    sentiment,
                    sent_score,
                    keywords,
                    published_at,
                    now,
                    now
                ))

                synced_count += 1

            except Exception as e:
                print(f"同步单条新闻失败: {e}")
                continue

        print(f"成功同步 {synced_count} 条新闻到 articles 表")
        return synced_count


if __name__ == "__main__":
    # 测试同步
    import os

    target_dsn = os.environ.get(
        "DATABASE_DSN",
        "root:123456@tcp(127.0.0.1:3306)/opinion_analysis"
    )

    with ArticleSyncBridge(target_dsn) as bridge:
        count = bridge.sync_daily_news_to_articles()
        print(f"\n同步完成，共 {count} 条文章")
