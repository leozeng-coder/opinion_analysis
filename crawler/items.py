import scrapy


class ArticleItem(scrapy.Item):
    source_id = scrapy.Field()
    title = scrapy.Field()
    content = scrapy.Field()
    author = scrapy.Field()
    origin_url = scrapy.Field()
    platform = scrapy.Field()
    published_at = scrapy.Field()  # datetime
