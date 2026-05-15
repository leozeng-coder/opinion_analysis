from datetime import datetime

import scrapy

from db import SessionLocal
from items import ArticleItem
from models import DataSource

TIEBA_HOT_URL = "https://tieba.baidu.com/hottopic/browse/topicList"


class TiebaSpider(scrapy.Spider):
    name = "tieba"

    async def start(self):
        yield scrapy.Request(
            TIEBA_HOT_URL,
            callback=self.parse,
            headers={"Referer": "https://tieba.baidu.com/"},
        )

    def parse(self, response):
        source_id = self._get_or_create_source()
        topics = (
            response.json()
            .get("data", {})
            .get("bang_topic", {})
            .get("topic_list", [])
        )
        for topic in topics:
            url = topic.get("topic_url", "")
            if not url:
                continue

            item = ArticleItem()
            item["source_id"] = source_id
            item["title"] = topic.get("topic_name", "")
            item["content"] = topic.get("topic_desc", "")
            item["author"] = ""
            item["origin_url"] = url
            item["platform"] = "forum"
            item["published_at"] = datetime.utcnow()
            yield item

    def _get_or_create_source(self) -> int:
        db = SessionLocal()
        try:
            src = db.query(DataSource).filter_by(name="贴吧热榜").first()
            if not src:
                src = DataSource(
                    name="贴吧热榜",
                    type="forum",
                    url="https://tieba.baidu.com/hot",
                    status=1,
                )
                db.add(src)
                db.commit()
                db.refresh(src)
            return src.id
        finally:
            db.close()
