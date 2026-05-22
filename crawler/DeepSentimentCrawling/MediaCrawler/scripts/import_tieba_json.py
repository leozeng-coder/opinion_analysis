#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Import tieba search JSON exports into opinion_analysis MySQL tables."""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
import time
from pathlib import Path
from typing import Any, Dict, List, Set

import aiomysql

MEDIA_CRAWLER_ROOT = Path(__file__).resolve().parents[1]
if str(MEDIA_CRAWLER_ROOT) not in sys.path:
    sys.path.insert(0, str(MEDIA_CRAWLER_ROOT))

from config.db_config import (  # noqa: E402
    MYSQL_DB_HOST,
    MYSQL_DB_NAME,
    MYSQL_DB_PORT,
    MYSQL_DB_PWD,
    MYSQL_DB_USER,
)
from async_db import AsyncMysqlDB  # noqa: E402


def load_json(path: Path) -> List[Dict[str, Any]]:
    with path.open("r", encoding="utf-8") as f:
        data = json.load(f)
    if not isinstance(data, list):
        raise ValueError(f"{path} must contain a JSON array")
    return data

def normalize_note(item: Dict[str, Any], allowed_columns: Set[str]) -> Dict[str, Any]:
    now_ts = int(time.time() * 1000)
    row = dict(item)
    row.setdefault("tieba_id", "")
    row.setdefault("user_link", "")
    row.setdefault("user_avatar", "")
    row.setdefault("ip_location", "")
    row.setdefault("source_keyword", "")
    row.setdefault("total_replay_num", 0)
    row.setdefault("total_replay_page", 0)
    row.setdefault("add_ts", row.get("last_modify_ts") or now_ts)
    row.setdefault("last_modify_ts", now_ts)
    return {k: v for k, v in row.items() if k in allowed_columns}


def normalize_comment(item: Dict[str, Any], allowed_columns: Set[str]) -> Dict[str, Any]:
    now_ts = int(time.time() * 1000)
    row = dict(item)
    row.setdefault("parent_comment_id", "")
    row.setdefault("user_link", "")
    row.setdefault("user_avatar", "")
    row.setdefault("tieba_id", "")
    row.setdefault("ip_location", "")
    row.setdefault("sub_comment_count", 0)
    row.setdefault("add_ts", row.get("last_modify_ts") or now_ts)
    row.setdefault("last_modify_ts", now_ts)
    return {k: v for k, v in row.items() if k in allowed_columns}


async def fetch_table_columns(db: AsyncMysqlDB, table_name: str) -> Set[str]:
    rows = await db.query(f"SHOW COLUMNS FROM `{table_name}`")
    return {row["Field"] for row in rows}


async def upsert_by_key(
    db: AsyncMysqlDB,
    table_name: str,
    key_field: str,
    item: Dict[str, Any],
) -> str:
    key_value = item[key_field]
    existing = await db.query(
        f"SELECT `{key_field}` FROM `{table_name}` WHERE `{key_field}` = %s LIMIT 1",
        key_value,
    )
    if existing:
        await db.update_table(table_name, item, key_field, key_value)
        return "updated"
    await db.item_to_table(table_name, item)
    return "inserted"


async def import_files(contents_path: Path, comments_path: Path) -> None:
    contents = load_json(contents_path)
    comments = load_json(comments_path)

    pool = await aiomysql.create_pool(
        host=MYSQL_DB_HOST,
        port=MYSQL_DB_PORT,
        user=MYSQL_DB_USER,
        password=MYSQL_DB_PWD,
        db=MYSQL_DB_NAME,
        autocommit=True,
    )
    db = AsyncMysqlDB(pool)

    note_columns = await fetch_table_columns(db, "tieba_note")
    comment_columns = await fetch_table_columns(db, "tieba_comment")

    note_stats = {"inserted": 0, "updated": 0}
    comment_stats = {"inserted": 0, "updated": 0}

    for item in contents:
        row = normalize_note(item, note_columns)
        action = await upsert_by_key(db, "tieba_note", "note_id", row)
        note_stats[action] += 1

    for item in comments:
        row = normalize_comment(item, comment_columns)
        action = await upsert_by_key(db, "tieba_comment", "comment_id", row)
        comment_stats[action] += 1

    note_count = await db.query("SELECT COUNT(*) AS cnt FROM tieba_note")
    comment_count = await db.query("SELECT COUNT(*) AS cnt FROM tieba_comment")

    pool.close()
    await pool.wait_closed()

    print(f"Loaded {len(contents)} notes from {contents_path.name}")
    print(f"Loaded {len(comments)} comments from {comments_path.name}")
    print(f"tieba_note: inserted={note_stats['inserted']}, updated={note_stats['updated']}")
    print(
        f"tieba_comment: inserted={comment_stats['inserted']}, "
        f"updated={comment_stats['updated']}"
    )
    print(f"Database totals: tieba_note={note_count[0]['cnt']}, tieba_comment={comment_count[0]['cnt']}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Import tieba JSON into MySQL")
    parser.add_argument(
        "--contents",
        default=str(Path(__file__).resolve().parents[4] / "search_contents_2026-05-22.json"),
        help="Path to search_contents JSON file",
    )
    parser.add_argument(
        "--comments",
        default=str(Path(__file__).resolve().parents[4] / "search_comments_2026-05-22.json"),
        help="Path to search_comments JSON file",
    )
    args = parser.parse_args()

    contents_path = Path(args.contents)
    comments_path = Path(args.comments)
    if not contents_path.exists():
        raise SystemExit(f"Contents file not found: {contents_path}")
    if not comments_path.exists():
        raise SystemExit(f"Comments file not found: {comments_path}")

    asyncio.run(import_files(contents_path, comments_path))


if __name__ == "__main__":
    main()
