BOT_NAME = "crawler"
SPIDER_MODULES = ["spiders"]
NEWSPIDER_MODULE = "spiders"

USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/124.0.0.0 Safari/537.36"
)

ROBOTSTXT_OBEY = False
DOWNLOAD_DELAY = 1
RANDOMIZE_DOWNLOAD_DELAY = True
CONCURRENT_REQUESTS = 4

ITEM_PIPELINES = {
    "pipelines.SentimentAndDBPipeline": 300,
}

LOG_LEVEL = "INFO"

# Async reactor (see scheduler.py: install_reactor before importing reactor)
TWISTED_REACTOR = "twisted.internet.asyncioreactor.AsyncioSelectorReactor"

# Scrapy 2.11+: avoid deprecated default fingerprint implementation
REQUEST_FINGERPRINTER_IMPLEMENTATION = "2.7"
