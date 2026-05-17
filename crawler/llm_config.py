#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
LLM 配置统一入口。

所有大模型相关参数（apiKey / baseURL / model 等）以数据库 system_settings 表为准，
不再读取 config.py 或 yaml。这样后端管理后台修改后，对爬虫子进程也立即生效。

读取顺序：
  1. system_settings 表中 tagger.* 行
  2. 环境变量（仅 apiKey，作为 CI/紧急情况下的兜底）
  3. 内置默认（baseURL / model）

注意：数据库连接信息仍走 config.py（基础设施 ≠ 业务配置）。
"""

import os
from dataclasses import dataclass
from typing import Optional

import pymysql
from pymysql.cursors import DictCursor

try:
    import config  # type: ignore
except ImportError as e:
    raise ImportError("无法导入 config.py，请确认运行时 PYTHONPATH 包含 crawler 目录") from e


_SETTING_PREFIX = "tagger."

_DEFAULTS = {
    "llm_api_key": "",
    "llm_base_url": "https://api.deepseek.com",
    "llm_model": "deepseek-chat",
}


@dataclass
class LLMConfig:
    api_key: str
    base_url: str
    model: str

    def is_ready(self) -> bool:
        return bool(self.api_key.strip())


def _open_conn():
    return pymysql.connect(
        host=config.DB_HOST,
        port=config.DB_PORT,
        user=config.DB_USER,
        password=config.DB_PASSWORD,
        database=config.DB_NAME,
        charset=config.DB_CHARSET,
        autocommit=True,
        cursorclass=DictCursor,
    )


def _load_from_db() -> dict:
    """从 system_settings 中读取 tagger.* 行；表不存在或连接失败返回空 dict。"""
    out = {}
    try:
        conn = _open_conn()
    except Exception as e:
        print(f"[llm_config] 读取数据库失败，使用内置默认: {e}", flush=True)
        return out
    try:
        with conn.cursor() as cur:
            cur.execute("SELECT `key`, `value` FROM system_settings WHERE `key` LIKE %s",
                        (_SETTING_PREFIX + "%",))
            for row in cur.fetchall():
                key = row["key"][len(_SETTING_PREFIX):]
                out[key] = row["value"]
    except Exception as e:
        # 表可能尚未建好（后端首次启动前），静默回退
        print(f"[llm_config] 查询 system_settings 失败（可能尚未建表）: {e}", flush=True)
    finally:
        try:
            conn.close()
        except Exception:
            pass
    return out


def load(refresh: bool = False) -> LLMConfig:
    """读取当前 LLM 配置。

    Args:
        refresh: 占位参数，目前每次都重新查询；后续可加进程内缓存。
    """
    _ = refresh  # 留作扩展
    db = _load_from_db()
    api_key = (db.get("llm_api_key") or "").strip()
    if not api_key:
        api_key = (os.environ.get("DEEPSEEK_API_KEY") or "").strip()

    base_url = (db.get("llm_base_url") or "").strip() or _DEFAULTS["llm_base_url"]
    model = (db.get("llm_model") or "").strip() or _DEFAULTS["llm_model"]
    return LLMConfig(api_key=api_key, base_url=base_url, model=model)


def get_api_key() -> Optional[str]:
    """便捷函数：仅取 apiKey（空字符串返回 None，便于 if 判断）。"""
    cfg = load()
    return cfg.api_key or None


if __name__ == "__main__":
    cfg = load()
    print(f"base_url = {cfg.base_url}")
    print(f"model    = {cfg.model}")
    print(f"api_key  = {'***' + cfg.api_key[-4:] if cfg.api_key else '(empty)'}")
    print(f"ready    = {cfg.is_ready()}")
