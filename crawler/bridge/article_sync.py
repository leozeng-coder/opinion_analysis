#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
数据桥接模块 - 将 MindSpider 数据同步到 opinion_analysis.articles 表
情感分析：批量调用 DeepSeek LLM，失败时回退到 SnowNLP，再失败置 neutral。
"""

import sys
import json
import re
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

# 关键词提取（本地、便宜）
try:
    import jieba.analyse
    JIEBA_AVAILABLE = True
except ImportError as _e:
    JIEBA_AVAILABLE = False
    print(f"[警告] jieba 未安装 ({_e})，关键词将为空数组。", flush=True)

# SnowNLP 作为 DeepSeek 失败时的兜底
try:
    from snownlp import SnowNLP
    SNOWNLP_AVAILABLE = True
except ImportError:
    SNOWNLP_AVAILABLE = False

# DeepSeek 客户端（按需懒加载，避免缺 key 时模块导入失败）
_deepseek_client = None
_deepseek_disabled = False


def _get_deepseek_client():
    """懒加载 DeepSeek 客户端。失败后置为禁用，不重复重试。"""
    global _deepseek_client, _deepseek_disabled
    if _deepseek_disabled:
        return None
    if _deepseek_client is not None:
        return _deepseek_client
    api_key = getattr(config, "DEEPSEEK_API_KEY", "") or ""
    if not api_key or api_key.startswith("your_") or api_key.startswith("sk-xxx"):
        print("[警告] config.DEEPSEEK_API_KEY 未配置，将退回 SnowNLP/neutral。", flush=True)
        _deepseek_disabled = True
        return None
    try:
        from openai import OpenAI
        _deepseek_client = OpenAI(api_key=api_key, base_url="https://api.deepseek.com")
        return _deepseek_client
    except Exception as e:
        print(f"[警告] DeepSeek 客户端初始化失败: {e}", flush=True)
        _deepseek_disabled = True
        return None


# 标签 -> 分数 的统一映射，方便落库
_LABEL_SCORE = {"positive": 0.6, "neutral": 0.0, "negative": -0.6}


def classify_sentiments_deepseek(texts: List[str], batch_size: int = 40) -> List[dict]:
    """批量情感分类。返回与 texts 等长的列表，每项 {label, score}。
    label ∈ {positive, neutral, negative}；调用失败的条目 label 为空字符串。"""
    n = len(texts)
    results: List[dict] = [{"label": "", "score": 0.0} for _ in range(n)]
    client = _get_deepseek_client()
    if client is None:
        return results

    for start in range(0, n, batch_size):
        chunk = texts[start:start + batch_size]
        numbered = "\n".join(f"{i+1}. {t}" for i, t in enumerate(chunk))
        prompt = (
            "你是一名舆情情感分析助手。请对以下中文新闻/帖子标题逐条判断情感倾向，"
            "只能输出三种标签之一：positive（正面）/ negative（负面）/ neutral（中性）。\n"
            "同时给出 -1~1 之间的分数：正面取 0~1，负面取 -1~0，中性取 -0.2~0.2。\n"
            "事故、灾难、欺诈、批评、伤亡、暴力等属于 negative；庆祝、突破、嘉奖、利好等属于 positive；"
            "纯陈述、客观播报、未带明显倾向属于 neutral。\n\n"
            f"待分类条目（共{len(chunk)}条）：\n{numbered}\n\n"
            "请严格按 JSON 数组输出，元素顺序对应上面编号，不要包含其它文字：\n"
            '[{"i":1,"label":"positive","score":0.7}, ...]'
        )
        try:
            resp = client.chat.completions.create(
                model="deepseek-chat",
                messages=[
                    {"role": "system", "content": "你是专业的中文舆情情感分类器，只输出严格的 JSON。"},
                    {"role": "user", "content": prompt},
                ],
                max_tokens=2000,
                temperature=0.0,
                response_format={"type": "json_object"},
            )
            raw = resp.choices[0].message.content or ""
            parsed = _parse_sentiment_response(raw, len(chunk))
            for j, item in enumerate(parsed):
                if item is None:
                    continue
                results[start + j] = item
        except Exception as e:
            print(f"[警告] DeepSeek 情感分类批次失败 (offset={start}): {e}", flush=True)
            # 该批留空，由调用方按需回退
            continue

    return results


def _parse_sentiment_response(raw: str, expected_len: int) -> List[Optional[dict]]:
    """解析 LLM 输出。返回长度 = expected_len 的列表，失败项置 None。"""
    out: List[Optional[dict]] = [None] * expected_len
    text = raw.strip()
    # response_format=json_object 时模型可能用 {"results":[...]} 或直接给数组
    arr = None
    try:
        data = json.loads(text)
        if isinstance(data, list):
            arr = data
        elif isinstance(data, dict):
            for k in ("results", "data", "items"):
                if isinstance(data.get(k), list):
                    arr = data[k]
                    break
            if arr is None:
                # 退化：dict 里直接是 {"1": {...}, "2": {...}}
                arr = [data[str(i + 1)] for i in range(expected_len) if str(i + 1) in data]
    except json.JSONDecodeError:
        m = re.search(r"\[.*\]", text, re.DOTALL)
        if m:
            try:
                arr = json.loads(m.group(0))
            except Exception:
                arr = None

    if not isinstance(arr, list):
        return out

    for item in arr:
        if not isinstance(item, dict):
            continue
        idx = item.get("i") or item.get("index") or item.get("id")
        try:
            idx = int(idx) - 1
        except (TypeError, ValueError):
            continue
        if idx < 0 or idx >= expected_len:
            continue
        label = str(item.get("label", "")).strip().lower()
        if label not in ("positive", "negative", "neutral"):
            continue
        try:
            score = float(item.get("score", _LABEL_SCORE[label]))
        except (TypeError, ValueError):
            score = _LABEL_SCORE[label]
        score = max(-1.0, min(1.0, score))
        out[idx] = {"label": label, "score": round(score, 4)}
    return out


def _snownlp_fallback(text: str) -> dict:
    if not SNOWNLP_AVAILABLE or not text.strip():
        return {"label": "neutral", "score": 0.0}
    try:
        raw = SnowNLP(text).sentiments
        score = round(raw * 2 - 1, 4)
        if raw > 0.6:
            return {"label": "positive", "score": score}
        if raw < 0.4:
            return {"label": "negative", "score": score}
        return {"label": "neutral", "score": score}
    except Exception:
        return {"label": "neutral", "score": 0.0}


def _extract_keywords(text: str, topk: int = 10) -> str:
    if not JIEBA_AVAILABLE or not text.strip():
        return "[]"
    try:
        kws = jieba.analyse.extract_tags(text, topK=topk)
        return json.dumps(kws, ensure_ascii=False)
    except Exception:
        return "[]"


def analyse_sentiment(title: str, content: str) -> tuple:
    """单条情感分析（向后兼容入口；批量请用 classify_sentiments_deepseek）。
    返回 (label, score, keywords_json)。"""
    text = (title + "。" + content).strip()
    if not text or text == "。":
        return "neutral", 0.0, "[]"

    batch = classify_sentiments_deepseek([text])
    result = batch[0] if batch else {"label": "", "score": 0.0}
    if not result.get("label"):
        result = _snownlp_fallback(text)
    return result["label"], result["score"], _extract_keywords(text)


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

        # 先去重，仅对需要新增的条目调用 LLM
        pending = []
        for news in news_items:
            origin_url = news['url'] or ''
            if origin_url:
                target_cursor.execute("SELECT id FROM articles WHERE origin_url = %s", (origin_url,))
                if target_cursor.fetchone():
                    continue
            pending.append(news)

        if not pending:
            print("所有新闻已存在，无需同步")
            return 0

        # 批量情感分类
        titles = [(n['title'] or '').strip() for n in pending]
        print(f"调用 DeepSeek 批量分类 {len(titles)} 条新闻情感...")
        sentiments = classify_sentiments_deepseek(titles)
        # 回退：未拿到标签的走 SnowNLP，再不行就 neutral
        for i, item in enumerate(sentiments):
            if not item.get("label"):
                sentiments[i] = _snownlp_fallback(titles[i])

        synced_count = 0
        for news, sent in zip(pending, sentiments):
            try:
                origin_url = news['url'] or ''
                platform = news['source_platform']
                source_name = platform_names.get(platform, platform)
                source_id = self.get_or_create_source('news', source_name)

                title = news['title'] or ''
                sentiment = sent["label"]
                sent_score = sent["score"]
                keywords_json = _extract_keywords(title)

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
                    '',          # content（daily_news 无正文）
                    '',          # author
                    origin_url,
                    platform,
                    sentiment,
                    sent_score,
                    keywords_json,
                    published_at,
                    now,
                    now,
                ))
                synced_count += 1
            except Exception as e:
                print(f"同步单条新闻失败: {e}")
                continue

        # 统计分布，便于排查
        from collections import Counter
        dist = Counter(s["label"] for s in sentiments)
        print(f"情感分布: {dict(dist)}")
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
