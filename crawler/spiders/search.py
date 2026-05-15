"""按关键词 / 话题搜索的爬虫。

通过百度新闻搜索 RSS 拉结果：
  https://news.baidu.com/ns?word=KEYWORD&tn=newsrss&cl=2&rn=20&ct=1

由 backend `/api/crawler/run` 在 advanced 模式下触发：每个关键词 / 话题
都会发起一次搜索；返回的文章再走 pipeline 过滤（关键词命中 + 时间范围）。
"""

from __future__ import annotations

from datetime import datetime
from urllib.parse import quote

import feedparser
import scrapy

from db import SessionLocal
from items import ArticleItem
from models import DataSource


_BAIDU_SEARCH_RSS = "https://news.baidu.com/ns?word={q}&tn=newsrss&cl=2&rn=20&ct=1"


class SearchSpider(scrapy.Spider):
    name = "search"

    async def start(self):
        terms: list[str] = []
        for src in (getattr(self, "search_keywords", None) or [],
                    getattr(self, "topic_keywords", None) or []):
            for t in src:
                t = (t or "").strip()
                if t and t not in terms:
                    terms.append(t)
        if not terms:
            self.logger.warning("SearchSpider has no keywords/topics, skipping")
            return
        for q in terms:
            yield scrapy.Request(
                _BAIDU_SEARCH_RSS.format(q=quote(q)),
                callback=self.parse,
                cb_kwargs={"query": q},
                headers={"Accept": "application/rss+xml, application/xml, */*"},
            )

    def parse(self, response, query: str):
        # Baidu sometimes returns an HTML page (captcha / bot-block) instead of RSS.
        # feedparser.parse on large HTML blocks the Twisted reactor thread, preventing
        # CLOSESPIDER_TIMEOUT from firing and causing the spider to hang indefinitely.
        body = (response.text or "").lstrip()
        if body.lower().startswith(("<!doctype", "<html")):
            self.logger.warning(
                "SearchSpider: HTML response for query %r (likely bot-blocked), skipping",
                query,
            )
            return
        source_id = self._get_or_create_source()
        feed = feedparser.parse(response.text)
        for entry in feed.entries:
            item = ArticleItem()
            item["source_id"] = source_id
            item["title"] = entry.get("title", "")
            item["content"] = entry.get("summary", entry.get("description", ""))
            item["author"] = entry.get("author", "")
            item["origin_url"] = entry.get("link", "")
            item["platform"] = "search"
            parsed_time = entry.get("published_parsed")
            item["published_at"] = (
                datetime(*parsed_time[:6]) if parsed_time else datetime.utcnow()
            )
            yield item

    def _get_or_create_source(self) -> int:
        db = SessionLocal()
        try:
            src = db.query(DataSource).filter_by(name="搜索-百度新闻").first()
            if not src:
                src = DataSource(
                    name="搜索-百度新闻",
                    type="search",
                    url="https://news.baidu.com/ns",
                    status=1,
                )
                db.add(src)
                db.commit()
                db.refresh(src)
            return src.id
        finally:
            db.close()
