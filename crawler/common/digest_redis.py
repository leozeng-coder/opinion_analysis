# -*- coding: utf-8 -*-
"""每日 AI 摘要 — 写入 Redis，供 Go 后端仪表盘读取。"""

from __future__ import annotations

import json
import os
from datetime import date, datetime
from typing import List, Optional

try:
    import redis
except ImportError:
    redis = None  # type: ignore

KEY_PREFIX = "opinion:daily_digest:"
LATEST_KEY = "opinion:daily_digest:latest"
TTL_SECONDS = 90 * 86400


def _redis_settings():
    try:
        import config as cfg
        host = getattr(cfg, "REDIS_HOST", "127.0.0.1")
        port = int(getattr(cfg, "REDIS_PORT", 6379))
        password = getattr(cfg, "REDIS_PASSWORD", "") or None
        db = int(getattr(cfg, "REDIS_DB", 0))
    except ImportError:
        host = os.getenv("REDIS_HOST", "127.0.0.1")
        port = int(os.getenv("REDIS_PORT", "6379"))
        password = os.getenv("REDIS_PASSWORD") or None
        db = int(os.getenv("REDIS_DB", "0"))
    return host, port, password, db


def _client():
    if redis is None:
        return None
    host, port, password, db = _redis_settings()
    try:
        return redis.Redis(
            host=host,
            port=port,
            password=password,
            db=db,
            decode_responses=True,
            socket_connect_timeout=3,
        )
    except Exception as e:
        print(f"[digest_redis] 连接失败: {e}")
        return None


def save_daily_digest(
    summary: str,
    keywords: List[str],
    digest_date: Optional[date] = None,
) -> bool:
    """保存当日 AI 摘要到 Redis。keywords 仍由 MySQL daily_topics 供 Stage2 使用。"""
    text = (summary or "").strip()
    if not text:
        return False

    d = digest_date or date.today()
    date_str = d.isoformat()
    payload = json.dumps(
        {
            "date": date_str,
            "text": text,
            "keywords": keywords or [],
            "updated_at": datetime.now().isoformat(timespec="seconds"),
        },
        ensure_ascii=False,
    )

    client = _client()
    if client is None:
        print("[digest_redis] Redis 不可用，摘要未写入")
        return False

    try:
        client.setex(f"{KEY_PREFIX}{date_str}", TTL_SECONDS, payload)
        client.setex(LATEST_KEY, TTL_SECONDS, payload)
        print(f"[digest_redis] 已写入 {date_str} 摘要 ({len(text)} 字)")
        return True
    except Exception as e:
        print(f"[digest_redis] 写入失败: {e}")
        return False


def load_daily_digest(digest_date: Optional[date] = None) -> Optional[dict]:
    """从 Redis 读取当日 AI 摘要。"""
    client = _client()
    if client is None:
        return None

    d = digest_date or date.today()
    date_str = d.isoformat()
    try:
        raw = client.get(f"{KEY_PREFIX}{date_str}")
        if not raw:
            raw = client.get(LATEST_KEY)
        if not raw:
            return None
        data = json.loads(raw)
        if not (data.get("text") or "").strip():
            return None
        return data
    except Exception as e:
        print(f"[digest_redis] 读取失败: {e}")
        return None
