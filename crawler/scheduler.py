#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
MindSpider - 定时调度器

替换原 crawler/scheduler.py，作为长期运行的调度守护进程。
从 opinion_analysis.crawler_spider_configs 表读取调度配置。

Spider 键：
  broad-topic    - Stage 1: 13个平台新闻收集 + AI关键词提取
  deep-sentiment - Stage 2: Playwright深度爬取（需安装浏览器）
"""
import logging
import os
import sys
from pathlib import Path
from datetime import datetime

# 添加项目根目录到路径
project_root = Path(__file__).parent
sys.path.insert(0, str(project_root))
sys.path.insert(0, str(project_root.parent))

import pymysql
from pymysql.cursors import DictCursor

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(name)s %(levelname)s %(message)s",
)
logger = logging.getLogger("mindspider.scheduler")

DATABASE_DSN = os.environ.get(
    "DATABASE_DSN",
    "root:123456@tcp(127.0.0.1:3306)/opinion_analysis"
)

DEFAULT_INTERVALS = {
    "broad-topic": (60, True),      # 每60分钟运行一次
    "deep-sentiment": (180, False), # 默认关闭，需手动启用
}


def parse_dsn(dsn: str) -> dict:
    """解析 GORM 格式 DSN"""
    import re
    m = re.match(r"(.+):(.+)@tcp\((.+):(\d+)\)/(\w+)", dsn)
    if m:
        user, pwd, host, port, db = m.groups()
        return {'user': user, 'password': pwd, 'host': host, 'port': int(port), 'database': db}
    raise ValueError(f"无法解析 DSN: {dsn}")


def get_db_connection():
    """获取数据库连接"""
    params = parse_dsn(DATABASE_DSN)
    return pymysql.connect(
        host=params['host'],
        port=params['port'],
        user=params['user'],
        password=params['password'],
        database=params['database'],
        charset='utf8mb4',
        autocommit=True,
        cursorclass=DictCursor
    )


def load_spider_configs() -> dict:
    """从数据库加载爬虫配置"""
    try:
        conn = get_db_connection()
        cursor = conn.cursor()
        cursor.execute(
            "SELECT spider_key, interval_minutes, enabled FROM crawler_spider_configs ORDER BY id"
        )
        rows = cursor.fetchall()
        conn.close()

        if not rows:
            logger.warning("crawler_spider_configs 表为空，使用默认配置")
            return dict(DEFAULT_INTERVALS)

        return {
            row['spider_key']: (int(row['interval_minutes']), bool(row['enabled']))
            for row in rows
        }

    except Exception as e:
        logger.warning(f"读取爬虫配置失败，使用默认配置: {e}")
        return dict(DEFAULT_INTERVALS)


def run_spider(key: str) -> None:
    """运行指定爬虫"""
    logger.info(f"开始定时执行: {key}")
    import subprocess

    # 规范化 key
    alias_map = {"rss": "broad-topic", "zhihu": "broad-topic",
                 "tieba": "broad-topic", "search": "deep-sentiment"}
    normalized_key = alias_map.get(key, key)

    script = str(project_root / "run_once.py")
    cmd = [sys.executable, script, "--spiders", normalized_key]

    env = os.environ.copy()
    env["PYTHONIOENCODING"] = "utf-8"

    try:
        result = subprocess.run(
            cmd,
            cwd=str(project_root),
            env=env,
            capture_output=False,
            timeout=1800  # 30分钟超时
        )
        if result.returncode == 0:
            logger.info(f"定时任务完成: {key}")
        else:
            logger.error(f"定时任务失败 (code={result.returncode}): {key}")
    except subprocess.TimeoutExpired:
        logger.error(f"定时任务超时: {key}")
    except Exception as e:
        logger.error(f"定时任务异常: {key} - {e}")


def apply_schedule(scheduler, cfgs: dict) -> None:
    """应用调度配置"""
    from apscheduler.triggers.interval import IntervalTrigger
    try:
        from apscheduler.jobstores.base import JobLookupError
    except ImportError:
        JobLookupError = Exception

    for key in list(DEFAULT_INTERVALS.keys()):
        minutes, enabled = cfgs.get(key, DEFAULT_INTERVALS[key])
        minutes = max(1, min(int(minutes), 10080))

        # 删除现有任务
        try:
            scheduler.remove_job(key)
        except Exception:
            pass

        if not enabled:
            logger.info(f"Spider {key} 已禁用")
            continue

        scheduler.add_job(
            run_spider,
            IntervalTrigger(minutes=minutes),
            args=[key],
            id=key,
            replace_existing=True,
        )
        logger.info(f"已调度 {key}，间隔 {minutes} 分钟")


def sync_schedule(scheduler) -> dict:
    """重新加载并应用调度配置"""
    cfgs = load_spider_configs()
    apply_schedule(scheduler, cfgs)
    return cfgs


if __name__ == "__main__":
    # 检查 APScheduler 是否已安装
    try:
        from apscheduler.schedulers.background import BackgroundScheduler
        from apscheduler.triggers.interval import IntervalTrigger
    except ImportError:
        logger.error("APScheduler 未安装，请运行: pip install APScheduler")
        sys.exit(1)

    logger.info("MindSpider 调度器启动...")
    logger.info(f"项目目录: {project_root}")
    logger.info(f"数据库: {DATABASE_DSN.split('@')[-1] if '@' in DATABASE_DSN else DATABASE_DSN}")

    scheduler = BackgroundScheduler()
    scheduler.start()

    # 加载初始配置
    cfgs = sync_schedule(scheduler)

    # 每2分钟重新加载配置
    scheduler.add_job(
        lambda: sync_schedule(scheduler),
        IntervalTrigger(minutes=2),
        id="__reload__",
        replace_existing=True,
    )

    # 立即运行一次已启用的爬虫
    for key, (minutes, enabled) in cfgs.items():
        if enabled:
            logger.info(f"启动时立即执行: {key}")
            import threading
            threading.Thread(target=run_spider, args=(key,), daemon=True).start()

    # 保持进程运行
    logger.info("调度器运行中 (Ctrl+C 停止)...")
    try:
        import time
        while True:
            time.sleep(60)
            logger.debug(f"调度器心跳: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    except (KeyboardInterrupt, SystemExit):
        logger.info("调度器停止")
        scheduler.shutdown()
