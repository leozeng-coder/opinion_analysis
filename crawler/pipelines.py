import logging
from datetime import datetime, timezone

from scrapy.exceptions import DropItem

from sentiment import analyse
from db import SessionLocal
from models import Article

logger = logging.getLogger(__name__)


def _parse_iso(value):
    if not value:
        return None
    if isinstance(value, datetime):
        return value
    s = str(value).strip()
    if not s:
        return None
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    try:
        dt = datetime.fromisoformat(s)
    except ValueError:
        return None
    if dt.tzinfo is not None:
        dt = dt.astimezone(timezone.utc).replace(tzinfo=None)
    return dt


def _as_naive(dt):
    if isinstance(dt, datetime) and dt.tzinfo is not None:
        return dt.astimezone(timezone.utc).replace(tzinfo=None)
    return dt


class SentimentAndDBPipeline:
    def __init__(self):
        self._crawler = None

    @classmethod
    def from_crawler(cls, crawler):
        pipe = cls()
        pipe._crawler = crawler
        return pipe

    def _passes_filter(self, item) -> bool:
        spider = self._crawler.spider if self._crawler else None
        kws = getattr(spider, "search_keywords", None) or []
        topics = getattr(spider, "topic_keywords", None) or []
        terms = [t for t in [*kws, *topics] if t]

        if terms:
            text = f"{item.get('title') or ''}\n{item.get('content') or ''}".lower()
            if not any(t.lower() in text for t in terms):
                return False

        start_at = _parse_iso(getattr(spider, "start_at", None)) if spider else None
        end_at = _parse_iso(getattr(spider, "end_at", None)) if spider else None
        pub = _as_naive(item.get("published_at"))
        if isinstance(pub, datetime):
            if start_at and pub < start_at:
                return False
            if end_at and pub > end_at:
                return False
        return True

    def process_item(self, item):
        origin_url = item.get("origin_url", "")
        if not origin_url:
            return item

        if not self._passes_filter(item):
            raise DropItem("filtered out by keyword/topic/date range")

        db = SessionLocal()
        try:
            if db.query(Article).filter_by(origin_url=origin_url).first():
                return item  # duplicate, skip

            label, sent_score, keywords = analyse(
                item.get("title", ""),
                item.get("content", ""),
            )

            article = Article(
                source_id=item.get("source_id"),
                title=item.get("title", ""),
                content=item.get("content", ""),
                author=item.get("author", ""),
                origin_url=origin_url,
                platform=item.get("platform", ""),
                sentiment=label,
                sent_score=sent_score,
                keywords=keywords,
                published_at=item.get("published_at") or datetime.now(timezone.utc),
            )
            db.add(article)
            db.commit()
        except DropItem:
            raise
        except Exception as e:
            db.rollback()
            logger.error("DB write failed for %r: %s", origin_url, e)
        finally:
            db.close()

        return item
