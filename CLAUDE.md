# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**MindSpider** is an AI-powered public opinion (舆情) analysis system — a sub-module of the [BettaFish](https://github.com/666ghj/BettaFish) multi-agent system. It has two sequential pipeline stages:

1. **BroadTopicExtraction** — Polls 13 Chinese news/social platforms via a third-party API, then calls DeepSeek LLM to extract trending keywords and generate a daily news summary.
2. **DeepSentimentCrawling** — Uses those keywords to perform Playwright-automated crawling across 7 social platforms (XHS, Douyin, Kuaishou, Bilibili, Weibo, Baidu Tieba, Zhihu), storing posts and comments in MySQL.

All code lives under `MindSpider-main/`.

## Setup

```bash
# Conda environment (Python 3.11 recommended; .python-version pins 3.9)
conda create -n pytorch_python11 python=3.11
conda activate pytorch_python11
pip install -r MindSpider-main/requirements.txt
playwright install

# Alternative: uv (for MediaCrawler sub-package only)
cd MindSpider-main/DeepSentimentCrawling/MediaCrawler
uv sync
```

### Required Configuration

Edit `MindSpider-main/config.py` before running anything:

```python
DB_HOST = "..."
DB_USER = "..."
DB_PASSWORD = "..."
DB_NAME = "mindspider"
DEEPSEEK_API_KEY = "..."
```

### Database Initialization

```bash
cd MindSpider-main
python main.py --setup          # checks config, deps, DB connection, creates tables
# or directly:
python schema/init_database.py
```

## Running the Pipeline

```bash
# System health check
python main.py --status

# Stage 1 only: fetch news + extract keywords via DeepSeek
python main.py --broad-topic

# Stage 2 only: crawl platforms with today's keywords (test mode)
python main.py --deep-sentiment --test

# Full pipeline
python main.py --complete --test

# Production run with specific platforms and limits
python main.py --complete --platforms xhs dy wb --max-keywords 30 --max-notes 50
```

Key CLI flags: `--date YYYY-MM-DD`, `--platforms xhs dy ks bili wb tieba zhihu`, `--test` (limits: 10 keywords, 10 notes), `--keywords-count N`, `--max-keywords N`, `--max-notes N`.

### Running MediaCrawler Directly

```bash
cd MindSpider-main/DeepSentimentCrawling/MediaCrawler
python main.py --platform xhs --lt qrcode --type search --save_data_option db
```

## Type Checking

```bash
cd MindSpider-main/DeepSentimentCrawling/MediaCrawler
mypy .   # config in mypy.ini; ignores missing stubs for cv2 and execjs
```

There is no automated test suite. Use `--test` mode for manual testing.

## Architecture

### Two-Stage Subprocess Orchestration

The root `main.py` invokes each module as a **subprocess** (`subprocess.run()`), keeping the two pipeline stages fully isolated with no shared imports.

### Dynamic Config File Injection (Critical Pattern)

`PlatformCrawler` in `DeepSentimentCrawling/platform_crawler.py` programmatically **overwrites** `MediaCrawler/config/base_config.py` and `MediaCrawler/config/db_config.py` before each crawl run to inject the current platform, keywords, and DB credentials from `config.py`. This is the integration bridge — do not assume these config files are hand-edited.

### Adding a New Platform to MediaCrawler

Implement all four files under `media_platform/{platform}/`:
- `client.py` — API client (request signing)
- `core.py` — Crawl logic (search, paginate, extract)
- `login.py` — Login flow (QR code / phone / cookie)
- `field.py` — Data field definitions

Register the platform shortcode in `CrawlerFactory` (`MediaCrawler/main.py`) and add a config file at `config/{platform}_config.py`.

### Key Patterns

- **Abstract base classes** in `base/base_crawler.py` define the interface every platform must implement (`AbstractCrawler`, `AbstractLogin`, `AbstractStore`, `AbstractApiClient`).
- **Factory pattern**: `CrawlerFactory` maps platform shortcodes (`"xhs"`, `"dy"`, etc.) to concrete crawler classes.
- **Multi-store backend**: selectable at runtime via `SAVE_DATA_OPTION` — `"db"` (MySQL, default), `"sqlite"`, `"csv"`, `"json"`.
- **Cache abstraction**: `cache_factory.py` returns either `RedisCache` or `LocalCache` (in-memory) for content deduplication.
- **CDP mode**: Set `ENABLE_CDP_MODE = True` in `base_config.py` to connect Playwright to an existing real browser instead of launching headless — reduces bot-detection risk.
- **Async split**: BroadTopicExtraction is fully async (`asyncio`). The outer orchestrator and DeepSentimentCrawling are synchronous. MediaCrawler internals are async, driven via `asyncio.get_event_loop().run_until_complete()`.

### AI Keyword Extraction

`TopicExtractor` sends structured prompts to DeepSeek (`deepseek-chat`, OpenAI-compatible client) requesting JSON output with a `keywords` array and a `summary` string. It falls back to regex parsing when the LLM returns malformed JSON.

### Database Schema

All DDL is in `schema/mindspider_tables.sql`. The `DatabaseManager` in `BroadTopicExtraction/database_manager.py` handles `daily_news` and `daily_topics` tables. Platform-specific tables are created by each platform's store module under `MediaCrawler/store/`.
