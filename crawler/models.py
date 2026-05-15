from datetime import datetime
from sqlalchemy import Column, Integer, String, Text, Float, DateTime, SmallInteger
from sqlalchemy.orm import DeclarativeBase


class Base(DeclarativeBase):
    pass


class DataSource(Base):
    __tablename__ = "data_sources"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(128), nullable=False)
    type = Column(String(32), nullable=False)   # weibo | weixin | news | forum
    url = Column(String(512))
    config = Column(Text)                        # JSON string
    status = Column(SmallInteger, default=1)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
    deleted_at = Column(DateTime, nullable=True)


class CrawlerSpiderConfig(Base):
    """与 Go 后端 model.CrawlerSpiderConfig / 表 crawler_spider_configs 对齐。"""

    __tablename__ = "crawler_spider_configs"

    id = Column(Integer, primary_key=True, autoincrement=True)
    spider_key = Column(String(32), nullable=False, unique=True)
    display_name = Column(String(64), default="")
    interval_minutes = Column(Integer, nullable=False, default=30)
    enabled = Column(SmallInteger, default=1)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class Article(Base):
    __tablename__ = "articles"

    id = Column(Integer, primary_key=True, autoincrement=True)
    source_id = Column(Integer, index=True)
    title = Column(String(512))
    content = Column(Text)
    author = Column(String(128))
    origin_url = Column(String(1024))
    platform = Column(String(32), index=True)
    sentiment = Column(String(16), index=True)  # positive | neutral | negative
    sent_score = Column(Float)                  # -1 ~ 1
    keywords = Column(Text)                     # JSON array string
    published_at = Column(DateTime, index=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
    deleted_at = Column(DateTime, nullable=True)
