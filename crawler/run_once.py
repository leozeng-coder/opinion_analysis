"""One-shot Scrapy run invoked by the Go API (run_once.py --spiders rss,zhihu).

环境变量过滤条件（二选一，优先文件）：
  CRAWLER_FILTER_FILE — UTF-8 JSON 文件路径（Windows 含中文关键词时推荐，由 Go 后端写入）
  CRAWLER_FILTER — JSON 字符串（兼容）
"""
from __future__ import annotations

import argparse
import json
import os
import sys

os.environ.setdefault("SCRAPY_SETTINGS_MODULE", "settings")
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from scrapy.utils.reactor import install_reactor

install_reactor("twisted.internet.asyncioreactor.AsyncioSelectorReactor")

from scrapy.crawler import CrawlerRunner
from scrapy.settings import Settings
from twisted.internet import defer, reactor

from sqlalchemy import text

from db import engine
from spiders.rss import RSSSpider
from spiders.zhihu import ZhihuSpider
from spiders.tieba import TiebaSpider
from spiders.search import SearchSpider

SPIDERS = {
    "rss": RSSSpider,
    "zhihu": ZhihuSpider,
    "tieba": TiebaSpider,
    "search": SearchSpider,
}


def _load_filter() -> dict:
    path = (os.environ.get("CRAWLER_FILTER_FILE") or "").strip()
    if path and os.path.isfile(path):
        try:
            with open(path, encoding="utf-8") as fp:
                data = json.load(fp)
            if isinstance(data, dict):
                return data
        except (OSError, json.JSONDecodeError) as e:
            print(f"CRAWLER_FILTER_FILE {path!r}: {e}", file=sys.stderr)
            return {}
    raw = os.environ.get("CRAWLER_FILTER")
    if not raw:
        return {}
    try:
        data = json.loads(raw)
    except json.JSONDecodeError as e:
        print(f"invalid CRAWLER_FILTER: {e}", file=sys.stderr)
        return {}
    if not isinstance(data, dict):
        return {}
    return data


def _spider_kwargs(flt: dict) -> dict:
    kws = [s for s in flt.get("keywords", []) or [] if isinstance(s, str)]
    topics = [s for s in flt.get("topics", []) or [] if isinstance(s, str)]
    start_at = flt.get("startAt") or None
    end_at = flt.get("endAt") or None
    out: dict = {}
    if kws:
        out["search_keywords"] = kws
    if topics:
        out["topic_keywords"] = topics
    if start_at:
        out["start_at"] = start_at
    if end_at:
        out["end_at"] = end_at
    return out


def _bootstrap_progress_row(run_id: int, spider_keys: list[str]) -> None:
    """进程启动后即写入阶段信息；不拉高 progress（避免先发 2% 再被首个 spider_opened 写回 0%）。"""
    detail = {
        "phase": "starting",
        "currentSpider": None,
        "completedSpiders": [],
        "totalSpiders": len(spider_keys),
        "itemsInSpider": 0,
    }
    try:
        payload = json.dumps(detail, ensure_ascii=False)
        with engine.begin() as conn:
            conn.execute(
                text(
                    "UPDATE crawler_run_logs SET progress_detail = :d "
                    "WHERE id = :id"
                ),
                {"d": payload, "id": run_id},
            )
    except Exception as e:
        print(f"bootstrap progress: {e}", file=sys.stderr)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--spiders",
        default="all",
        help="Comma-separated: rss,zhihu,tieba,search or 'all'",
    )
    args = parser.parse_args()
    raw = args.spiders.strip().lower()
    if raw == "all":
        keys = list(SPIDERS)
    else:
        keys = []
        for part in args.spiders.split(","):
            kk = part.strip().lower()
            if not kk:
                continue
            if kk not in SPIDERS:
                print(f"unknown spider: {part!r}", file=sys.stderr)
                sys.exit(2)
            if kk not in keys:
                keys.append(kk)
    if not keys:
        print("no spiders to run", file=sys.stderr)
        sys.exit(2)

    flt = _load_filter()
    kwargs = _spider_kwargs(flt)
    if kwargs:
        print(f"run_once filter: {kwargs}", flush=True)

    rid_env = (os.environ.get("CRAWLER_RUN_LOG_ID") or "").strip()
    ff_env = (os.environ.get("CRAWLER_FILTER_FILE") or "").strip()
    if rid_env.isdigit():
        print(
            f"[crawler task] python_start run_id={rid_env} spiders={keys!r} "
            f"filter_file_set={bool(ff_env)} filter_json_keys={list(flt)!r}",
            file=sys.stderr,
            flush=True,
        )

    st = Settings()
    st.setmodule("settings")
    # 单次 API 触发：避免外网卡死无限等待（定时 scheduler 仍用默 settings）
    st.set("DOWNLOAD_TIMEOUT", 15)
    st.set("CLOSESPIDER_TIMEOUT", 120)
    st.set("RETRY_TIMES", 0)

    if rid_env.isdigit():
        _r = int(rid_env)
        _bootstrap_progress_row(_r, keys)
        st.set("CRAWLER_RUN_LOG_ID", _r)
        st.set("CRAWLER_SPIDER_NAMES", ",".join(keys))
        st.set("EXTENSIONS", {"run_progress.RunProgressExtension": 0})

    runner = CrawlerRunner(st)
    crawl_defers: list[defer.Deferred] = [
        runner.crawl(SPIDERS[k], **kwargs) for k in keys
    ]

    def _stop_reactor(_ignored: object = None) -> None:
        if reactor.running:
            reactor.stop()

    defer.DeferredList(crawl_defers, consumeErrors=True).addBoth(_stop_reactor)
    # CrawlerProcess + AsyncioSelectorReactor 在部分环境下蜘蛛结束后 reactor 不收尾，进程一直不退出；
    # CrawlerRunner + 全部 Deferred 完成后显式 stop，供 Go 子进程正常返回。
    reactor.run(installSignalHandlers=True)

    if rid_env.isdigit():
        print(f"[crawler task] python_exit run_id={rid_env}", file=sys.stderr, flush=True)


if __name__ == "__main__":
    main()
