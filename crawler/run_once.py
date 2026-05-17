#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
舆情分析 - Go 后端集成入口点

替换原 crawler/run_once.py，供 Go 后端通过子进程调用。

接口兼容性：
  CLI: python run_once.py --spiders broad-topic,deep-sentiment
  环境变量:
    DATABASE_DSN        - 目标数据库连接 (GORM格式: user:pass@tcp(host:port)/db)
    CRAWLER_RUN_LOG_ID  - 运行日志ID，用于进度追踪
    CRAWLER_SPIDER_NAMES - 逗号分隔的爬虫名称
    CRAWLER_FILTER_FILE - JSON过滤参数文件路径 (关键词/话题/时间范围)

Spider 键：
  broad-topic    - Stage 1: 收集13个平台新闻 + AI关键词提取 (替代原 rss/zhihu/tieba)
  deep-sentiment - Stage 2: 基于关键词的Playwright深度爬取 (替代原 search)

向后兼容: rss/zhihu/tieba 映射到 broad-topic, search 映射到 deep-sentiment
"""
from __future__ import annotations

import argparse
import asyncio
import json
import os
import sys
from datetime import date, datetime
from pathlib import Path
from typing import Dict, List, Optional

# 添加项目根目录到路径
project_root = Path(__file__).parent
sys.path.insert(0, str(project_root))
sys.path.insert(0, str(project_root.parent))

# 导入爬虫模块
try:
    from BroadTopicExtraction.main import BroadTopicExtraction
    from bridge.article_sync import ArticleSyncBridge
except ImportError as e:
    print(f"[run_once] 模块导入失败: {e}", file=sys.stderr)
    sys.exit(1)

# Spider 键规范化映射（向后兼容）
SPIDER_KEY_ALIASES = {
    "rss": "broad-topic",
    "zhihu": "broad-topic",
    "tieba": "broad-topic",
    "search": "deep-sentiment",
}

VALID_SPIDER_KEYS = {"broad-topic", "deep-sentiment"}


def load_filter() -> Dict:
    """从环境变量或文件加载过滤参数"""
    path = (os.environ.get("CRAWLER_FILTER_FILE") or "").strip()
    if path and os.path.isfile(path):
        try:
            with open(path, encoding="utf-8") as fp:
                data = json.load(fp)
            if isinstance(data, dict):
                return data
        except (OSError, json.JSONDecodeError) as e:
            print(f"[run_once] CRAWLER_FILTER_FILE 读取失败 {path!r}: {e}", file=sys.stderr)

    raw = os.environ.get("CRAWLER_FILTER")
    if not raw:
        return {}
    try:
        data = json.loads(raw)
        return data if isinstance(data, dict) else {}
    except json.JSONDecodeError:
        return {}


def get_target_dsn() -> str:
    """获取目标数据库连接字符串"""
    return (os.environ.get("DATABASE_DSN") or "").strip() or \
           "root:123456@tcp(127.0.0.1:3306)/opinion_analysis"


def get_run_id() -> Optional[int]:
    """获取运行日志ID"""
    rid = (os.environ.get("CRAWLER_RUN_LOG_ID") or "").strip()
    return int(rid) if rid.isdigit() else None


def update_run_progress(bridge: ArticleSyncBridge, run_id: Optional[int],
                        progress: int, phase: str, extra: Dict = None) -> None:
    """更新运行进度到 crawler_run_logs 表"""
    if run_id is None:
        return

    detail = {"phase": phase, **(extra or {})}
    detail_json = json.dumps(detail, ensure_ascii=False)

    try:
        cursor = bridge.target_conn.cursor()
        cursor.execute(
            "UPDATE crawler_run_logs SET progress = %s, progress_detail = %s WHERE id = %s",
            (progress, detail_json, run_id)
        )
    except Exception as e:
        print(f"[run_once] 进度更新失败: {e}", file=sys.stderr)


def bootstrap_progress(bridge: ArticleSyncBridge, run_id: Optional[int],
                       spider_keys: List[str]) -> None:
    """初始化进度信息"""
    if run_id is None:
        return

    detail = {
        "phase": "starting",
        "spiders": spider_keys,
        "totalSpiders": len(spider_keys),
        "currentSpider": None,
        "completedSpiders": [],
    }

    try:
        cursor = bridge.target_conn.cursor()
        cursor.execute(
            "UPDATE crawler_run_logs SET progress_detail = %s WHERE id = %s",
            (json.dumps(detail, ensure_ascii=False), run_id)
        )
    except Exception as e:
        print(f"[run_once] 初始化进度失败: {e}", file=sys.stderr)


def _save_keywords_to_db(source_conn, keywords: List[str], target_date: date) -> int:
    """将用户提供的关键词写入 opinion_analysis.daily_topics，供 KeywordManager 读取"""
    if not keywords:
        return 0
    import hashlib
    import time
    now_ts = int(time.time() * 1000)
    cursor = source_conn.cursor()
    saved = 0
    for kw in keywords:
        topic_id = hashlib.md5(f"user:{kw}:{target_date}".encode()).hexdigest()[:32]
        try:
            cursor.execute(
                """INSERT INTO daily_topics
                   (topic_id, topic_name, keywords, extract_date, relevance_score,
                    processing_status, add_ts, last_modify_ts)
                   VALUES (%s, %s, %s, %s, 1.0, 'completed', %s, %s)
                   ON DUPLICATE KEY UPDATE
                     topic_name = VALUES(topic_name),
                     last_modify_ts = VALUES(last_modify_ts)""",
                (topic_id, kw, json.dumps([kw], ensure_ascii=False), target_date, now_ts, now_ts)
            )
            saved += 1
        except Exception as e:
            print(f"[run_once] 关键词写入失败 {kw!r}: {e}", file=sys.stderr)
    return saved


async def run_broad_topic(bridge: ArticleSyncBridge, run_id: Optional[int],
                          filter_data: Dict) -> bool:
    """
    运行 Stage 1: BroadTopicExtraction
    收集新闻 → AI关键词提取 → 同步到 opinion_analysis.articles
    """
    keywords = filter_data.get('keywords', []) or filter_data.get('topics', [])
    start_at = filter_data.get('startAt', '')
    end_at = filter_data.get('endAt', '')

    print("[run_once] 启动 BroadTopicExtraction (Stage 1)...")
    if keywords:
        print(f"[run_once] 关键词过滤: {keywords}")
    if start_at or end_at:
        print(f"[run_once] 时间过滤: {start_at or '不限'} ~ {end_at or '不限'}")
    update_run_progress(bridge, run_id, 5, "collecting_news",
                        {"description": "正在收集13个平台新闻"})

    try:
        async with BroadTopicExtraction() as extractor:
            # Stage 1: 收集新闻 + 提取关键词 + 保存到 opinion_analysis DB
            result = await extractor.run_daily_extraction(max_keywords=100, keywords=keywords or None)

            if not result['success']:
                print(f"[run_once] BroadTopicExtraction 失败: {result.get('error')}")
                update_run_progress(bridge, run_id, 100, "failed",
                                    {"error": str(result.get('error', '新闻收集失败'))})
                return False

            news_count = result.get('news_collection', {}).get('total_news', 0)
            keywords_count = result.get('topic_extraction', {}).get('keywords_count', 0)

            print(f"[run_once] Stage 1 完成: {news_count} 条新闻, {keywords_count} 个关键词")
            update_run_progress(bridge, run_id, 60, "syncing_articles", {
                "newsCount": news_count,
                "keywordsCount": keywords_count
            })

        # Stage 2: 同步到 opinion_analysis.articles（按关键词过滤）
        print("[run_once] 同步新闻到 opinion_analysis.articles...")
        synced = bridge.sync_daily_news_to_articles(
            date.today(),
            start_at=start_at or None,
            end_at=end_at or None,
        )

        update_run_progress(bridge, run_id, 90, "done", {
            "newsCount": news_count,
            "keywordsCount": keywords_count,
            "syncedArticles": synced
        })

        print(f"[run_once] 同步完成: {synced} 条文章写入 articles 表")
        return True

    except Exception as e:
        print(f"[run_once] broad-topic 执行失败: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc(file=sys.stderr)
        update_run_progress(bridge, run_id, 100, "failed", {"error": str(e)})
        return False


def run_deep_sentiment(bridge: ArticleSyncBridge, run_id: Optional[int],
                       filter_data: Dict) -> bool:
    """
    运行 Stage 2: DeepSentimentCrawling
    基于关键词的Playwright深度爬取
    注意：需要先安装 Playwright 浏览器 (playwright install)
    """
    import subprocess

    keywords = filter_data.get('keywords', []) or filter_data.get('topics', [])
    print(f"[run_once] 启动 DeepSentimentCrawling (Stage 2), 关键词: {keywords or '使用今日关键词'}")
    update_run_progress(bridge, run_id, 5, "deep_sentiment_starting",
                        {"keywords": keywords[:5] if keywords else []})

    # 将用户提供的关键词写入 opinion_analysis.daily_topics，使 KeywordManager 能读取
    if keywords:
        saved = _save_keywords_to_db(bridge.source_conn, keywords, date.today())
        print(f"[run_once] 已写入 {saved} 个用户关键词到 daily_topics")

    try:
        cmd = [sys.executable, str(project_root / 'main.py'), '--deep-sentiment', '--test']
        result = subprocess.run(
            cmd,
            cwd=str(project_root),
            capture_output=True,
            text=True,
            encoding='utf-8',
            timeout=600
        )

        if result.returncode != 0:
            print(f"[run_once] deep-sentiment 失败 (code={result.returncode})", file=sys.stderr)
            if result.stderr:
                print(result.stderr[:2000], file=sys.stderr)
            update_run_progress(bridge, run_id, 100, "deep_sentiment_failed",
                                {"exitCode": result.returncode,
                                 "stderr": result.stderr[:500] if result.stderr else ""})
            return False

        update_run_progress(bridge, run_id, 90, "deep_sentiment_done")
        return True

    except subprocess.TimeoutExpired:
        print("[run_once] deep-sentiment 超时 (10分钟)", file=sys.stderr)
        update_run_progress(bridge, run_id, 100, "deep_sentiment_failed", {"error": "timeout"})
        return False
    except Exception as e:
        print(f"[run_once] deep-sentiment 执行失败: {e}", file=sys.stderr)
        update_run_progress(bridge, run_id, 100, "deep_sentiment_failed", {"error": str(e)})
        return False


def normalize_keys(raw_keys: List[str]) -> List[str]:
    """规范化 spider key（合并别名，去重）"""
    seen = set()
    normalized = []
    for key in raw_keys:
        k = SPIDER_KEY_ALIASES.get(key, key)
        if k in VALID_SPIDER_KEYS and k not in seen:
            seen.add(k)
            normalized.append(k)
    return normalized


def main() -> None:
    parser = argparse.ArgumentParser(description="舆情分析 run_once 入口点")
    parser.add_argument(
        "--spiders",
        default="broad-topic",
        help="逗号分隔的爬虫键: broad-topic, deep-sentiment (或别名 rss/zhihu/tieba/search)"
    )
    args = parser.parse_args()

    # 解析 spider 键
    raw_input = args.spiders.strip().lower()
    if raw_input == "all":
        raw_keys = ["broad-topic"]
    else:
        raw_keys = [k.strip() for k in raw_input.split(",") if k.strip()]

    keys = normalize_keys(raw_keys)
    if not keys:
        print(f"[run_once] 无效的 spider 键: {raw_input}", file=sys.stderr)
        sys.exit(2)

    # 加载过滤参数
    filter_data = load_filter()

    # 获取运行参数
    run_id = get_run_id()
    target_dsn = get_target_dsn()

    print(f"[run_once] 启动 run_id={run_id} spiders={keys} has_filter={bool(filter_data)}", flush=True)

    # 连接目标数据库
    try:
        bridge = ArticleSyncBridge(target_dsn)
    except Exception as e:
        print(f"[run_once] 数据库连接失败: {e}", file=sys.stderr)
        sys.exit(1)

    try:
        bootstrap_progress(bridge, run_id, keys)

        success = True
        for key in keys:
            print(f"\n[run_once] === 运行 {key} ===", flush=True)
            update_run_progress(bridge, run_id, 2, "running",
                                {"currentSpider": key})

            if key == "broad-topic":
                ok = asyncio.run(run_broad_topic(bridge, run_id, filter_data))
            elif key == "deep-sentiment":
                ok = run_deep_sentiment(bridge, run_id, filter_data)
            else:
                print(f"[run_once] 未知的 spider 键: {key}", file=sys.stderr)
                ok = False

            if not ok:
                success = False
                print(f"[run_once] {key} 执行失败", file=sys.stderr)

        if run_id:
            print(f"[run_once] 完成 run_id={run_id} success={success}", file=sys.stderr, flush=True)

    finally:
        bridge.close()

    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
