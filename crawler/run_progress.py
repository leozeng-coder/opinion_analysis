"""Scrapy extension：把单次 run 的进度写入 MySQL crawler_run_logs（供 GET /api/crawler/progress/:id）。"""

from __future__ import annotations

import json
import logging
import threading
import time
from typing import Any

from scrapy import signals
from scrapy.exceptions import NotConfigured
from sqlalchemy import text

from db import engine

logger = logging.getLogger(__name__)

_lock = threading.RLock()
"""同一 run_once 进程中可能并发跑多个Spider，各占一个 Crawler / Extension 实例，
需在进程内聚合「已完成 spider」，否则每个扩展只看到自己那只蜘蛛，progress/detail 错乱。"""


# run_id -> 共享聚合状态（进程结束时随子进程销毁）
_run_aggregate: dict[int, dict[str, Any]] = {}


def _agg_for(run_id: int, spider_names: list[str]) -> dict[str, Any]:
    with _lock:
        if run_id not in _run_aggregate:
            plan = tuple(spider_names)
            _run_aggregate[run_id] = {
                "plan": plan,
                "completed": set(),
                "current": None,
                "items_this_spider": 0,
            }
        return _run_aggregate[run_id]


def _update_row(run_id: int, progress: int, detail: dict[str, Any]) -> None:
    try:
        payload = json.dumps(detail, ensure_ascii=False)
        with engine.begin() as conn:
            # 不按 status 过滤：避免与 Go 更新竞态导致一整轮零更新；进程退出后不再写入
            conn.execute(
                text(
                    "UPDATE crawler_run_logs SET progress = :p, progress_detail = :d WHERE id = :id"
                ),
                {"p": progress, "d": payload, "id": run_id},
            )
    except Exception as e:
        logger.warning("progress update failed: %s", e)


class RunProgressExtension:
    def __init__(self, run_id: int, spider_names: list[str]):
        self.run_id = run_id
        self.spider_names = spider_names
        self._last_flush = 0.0

    @classmethod
    def from_crawler(cls, crawler):
        run_id = crawler.settings.getint("CRAWLER_RUN_LOG_ID", 0)
        if not run_id:
            raise NotConfigured
        raw = crawler.settings.get("CRAWLER_SPIDER_NAMES") or ""
        names = [x.strip() for x in raw.split(",") if x.strip()]
        ext = cls(run_id, names)
        _ = _agg_for(run_id, names)
        crawler.signals.connect(ext.spider_opened, signal=signals.spider_opened)
        crawler.signals.connect(ext.spider_closed, signal=signals.spider_closed)
        crawler.signals.connect(ext.item_scraped, signal=signals.item_scraped)
        return ext

    def _detail(self, phase: str, st: dict[str, Any]) -> dict[str, Any]:
        plan: tuple[str, ...] = st["plan"]
        completed = st["completed"]
        ordered_done = [n for n in plan if n in completed]
        return {
            "phase": phase,
            "currentSpider": st["current"],
            "completedSpiders": ordered_done,
            "totalSpiders": len(plan),
            "itemsInSpider": int(st["items_this_spider"]),
        }

    def _compute_progress(self, st: dict[str, Any]) -> int:
        plan: tuple[str, ...] = st["plan"]
        n = len(plan)
        if n <= 0:
            return 0
        completed = st["completed"]
        done = len(completed.intersection(set(plan)))
        # 爬虫侧最高 99%：最终 100% 与 status 由 Go 子进程退出时统一写入，避免「已完成」仍显示运行中
        if done >= n:
            return 99
        base = int(100 * done / n)
        chunk = 100.0 / n
        tail = min(0.85, float(st["items_this_spider"]) / 80.0) * chunk
        return int(min(98, base + tail))

    def _flush(self, progress: int, detail: dict[str, Any]) -> None:
        self._last_flush = time.monotonic()
        _update_row(self.run_id, progress, detail)

    def spider_opened(self, spider):
        with _lock:
            st = _agg_for(self.run_id, self.spider_names)
            st["current"] = spider.name
            st["items_this_spider"] = 0
            pct = self._compute_progress(st)
            detail = self._detail("running", st)
        self._flush(pct, detail)

    def spider_closed(self, spider, reason):
        _ = reason
        with _lock:
            st = _agg_for(self.run_id, self.spider_names)
            st["completed"].add(spider.name)
            st["current"] = None
            st["items_this_spider"] = 0
            plan: tuple[str, ...] = st["plan"]
            done = len(st["completed"].intersection(set(plan)))
            nsp = len(plan)
            pct = self._compute_progress(st)
            phase = "closing" if (nsp > 0 and done >= nsp) else "running"
            detail = self._detail(phase, st)
        self._flush(pct, detail)

    def item_scraped(self, item, response, spider):
        _ = item, response, spider
        now = time.monotonic()
        with _lock:
            st = _agg_for(self.run_id, self.spider_names)
            st["items_this_spider"] = int(st["items_this_spider"]) + 1
            m = int(st["items_this_spider"])
            if now - self._last_flush < 1.2 and m % 15 != 0:
                return
            pct = self._compute_progress(st)
            detail = self._detail("running", st)
        self._flush(pct, detail)
