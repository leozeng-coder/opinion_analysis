from datetime import datetime

import feedparser
import scrapy

from db import SessionLocal
from items import ArticleItem
from models import DataSource

RSS_SOURCES = [
    {
        "name": "新浪新闻",
        "url": "http://rss.sina.com.cn/news/china/focus15.xml",
        "platform": "news",
        "type": "news",
    },
    {
        "name": "网易新闻",
        "url": "http://news.163.com/special/00011K6L/rss_newstop.xml",
        "platform": "news",
        "type": "news",
    },
    {
        "name": "澎湃新闻",
        "url": "https://www.thepaper.cn/rss",
        "platform": "news",
        "type": "news",
    },
]


class RSSSpider(scrapy.Spider):
    name = "rss"

    async def start(self):
        for source in RSS_SOURCES:
            yield scrapy.Request(
                source["url"],
                callback=self.parse,
                cb_kwargs={"source_config": source},
                headers={"Accept": "application/rss+xml, application/xml, */*"},
            )

    def parse(self, response, source_config):
        source_id = self._get_or_create_source(source_config)
        feed = feedparser.parse(response.text)

        for entry in feed.entries:
            item = ArticleItem()
            item["source_id"] = source_id
            item["title"] = entry.get("title", "")
            item["content"] = entry.get("summary", entry.get("description", ""))
            item["author"] = entry.get("author", "")
            item["origin_url"] = entry.get("link", "")
            item["platform"] = source_config["platform"]

            parsed_time = entry.get("published_parsed")
            item["published_at"] = datetime(*parsed_time[:6]) if parsed_time else datetime.utcnow()
            yield item

    def _get_or_create_source(self, source_config: dict) -> int:
        db = SessionLocal()
        try:
            src = db.query(DataSource).filter_by(url=source_config["url"]).first()
            if not src:
                src = DataSource(
                    name=source_config["name"],
                    type=source_config["type"],
                    url=source_config["url"],
                    status=1,
                )
                db.add(src)
                db.commit()
                db.refresh(src)
            return src.id
        finally:
            db.close()
