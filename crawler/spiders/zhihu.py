from datetime import datetime

import scrapy

from db import SessionLocal
from items import ArticleItem
from models import DataSource

ZHIHU_HOT_URL = (
    "https://www.zhihu.com/api/v3/feed/topstory/hot-lists/total?limit=50"
)


class ZhihuSpider(scrapy.Spider):
    name = "zhihu"

    async def start(self):
        yield scrapy.Request(
            ZHIHU_HOT_URL,
            callback=self.parse,
            headers={
                "Referer": "https://www.zhihu.com/hot",
                "x-api-version": "3.0.91",
                "x-app-za": "OS=Web",
            },
        )

    def parse(self, response):
        source_id = self._get_or_create_source()
        for entry in response.json().get("data", []):
            target = entry.get("target", {})
            qid = target.get("id", "")
            if not qid:
                continue

            item = ArticleItem()
            item["source_id"] = source_id
            item["title"] = target.get("title", "")
            item["content"] = target.get("excerpt", "")
            item["author"] = target.get("author", {}).get("name", "")
            item["origin_url"] = f"https://www.zhihu.com/question/{qid}"
            item["platform"] = "forum"
            item["published_at"] = datetime.utcnow()
            yield item

    def _get_or_create_source(self) -> int:
        db = SessionLocal()
        try:
            src = db.query(DataSource).filter_by(name="知乎热榜").first()
            if not src:
                src = DataSource(
                    name="知乎热榜",
                    type="forum",
                    url="https://www.zhihu.com/hot",
                    status=1,
                )
                db.add(src)
                db.commit()
                db.refresh(src)
            return src.id
        finally:
            db.close()
