import logging
import os
import sys

os.environ.setdefault("SCRAPY_SETTINGS_MODULE", "settings")
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from scrapy.utils.reactor import install_reactor

install_reactor("twisted.internet.asyncioreactor.AsyncioSelectorReactor")

from scrapy.settings import Settings
from scrapy.crawler import CrawlerRunner
from twisted.internet import reactor
from apscheduler.schedulers.twisted import TwistedScheduler
from apscheduler.triggers.interval import IntervalTrigger
from apscheduler.jobstores.base import JobLookupError

from spiders.rss import RSSSpider
from spiders.zhihu import ZhihuSpider
from spiders.tieba import TiebaSpider
from models import CrawlerSpiderConfig
from db import SessionLocal

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(name)s %(levelname)s %(message)s",
)
logger = logging.getLogger(__name__)

settings = Settings()
settings.setmodule("settings")

runner = CrawlerRunner(settings)

DEFAULT_INTERVALS = {
    "rss": (30, True),
    "zhihu": (60, True),
    "tieba": (120, True),
}

SPIDER_CLASSES = {
    "rss": RSSSpider,
    "zhihu": ZhihuSpider,
    "tieba": TiebaSpider,
}


def crawl_spider(key: str) -> None:
    logger.info("Starting scheduled crawl: %s", key)
    runner.crawl(SPIDER_CLASSES[key])


def load_spider_config_from_db() -> dict[str, tuple[int, bool]]:
    session = SessionLocal()
    try:
        rows = session.query(CrawlerSpiderConfig).order_by(CrawlerSpiderConfig.id).all()
        if not rows:
            return dict(DEFAULT_INTERVALS)
        return {
            str(r.spider_key): (int(r.interval_minutes), bool(r.enabled))
            for r in rows
        }
    except Exception as e:
        logger.warning("Failed to read crawler_spider_configs, using defaults: %s", e)
        return dict(DEFAULT_INTERVALS)
    finally:
        session.close()


def apply_schedule(scheduler: TwistedScheduler, cfgs: dict[str, tuple[int, bool]]) -> None:
    for key in ("rss", "zhihu", "tieba"):
        minutes, enabled = cfgs.get(key, DEFAULT_INTERVALS[key])
        minutes = max(1, min(int(minutes), 10080))
        try:
            scheduler.remove_job(key)
        except JobLookupError:
            pass
        if not enabled:
            logger.info("Spider %s disabled (no interval job)", key)
            continue
        scheduler.add_job(
            crawl_spider,
            IntervalTrigger(minutes=minutes),
            args=[key],
            id=key,
            replace_existing=True,
        )
        logger.info("Scheduled %s every %s minutes", key, minutes)


def sync_schedule(scheduler: TwistedScheduler) -> dict[str, tuple[int, bool]]:
    cfgs = load_spider_config_from_db()
    apply_schedule(scheduler, cfgs)
    return cfgs


if __name__ == "__main__":
    scheduler = TwistedScheduler()
    scheduler.start()

    cfgs = sync_schedule(scheduler)
    scheduler.add_job(
        lambda: sync_schedule(scheduler),
        IntervalTrigger(minutes=2),
        id="__reload__",
        replace_existing=True,
    )

    for key in ("rss", "zhihu", "tieba"):
        _, en = cfgs.get(key, DEFAULT_INTERVALS[key])
        if en:
            crawl_spider(key)

    reactor.run()
