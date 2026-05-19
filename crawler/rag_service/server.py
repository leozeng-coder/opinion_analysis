#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
舆情 RAG：Milvus + MySQL 增量同步 + 混合检索（句向量 + 关键词）。
向量化支持本地 Sentence-Transformers 或 OpenAI 兼容 Embedding API（与对话大模型无关）。
"""
from __future__ import annotations

import hashlib
import json
import logging
import os
import threading
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

import pymysql
import uvicorn
from apscheduler.schedulers.background import BackgroundScheduler
from fastapi import BackgroundTasks, FastAPI, HTTPException
from pydantic import AliasChoices, BaseModel, ConfigDict, Field
from pymilvus import DataType, MilvusClient
from embedder import Embedder, create_embedder

RAG_HOST = os.environ.get("RAG_HOST", "0.0.0.0")
RAG_PORT = int(os.environ.get("RAG_PORT", "5055"))

_BASE = Path(__file__).resolve().parent
_DATA = _BASE.parent / "data"
_DATA.mkdir(exist_ok=True)
MILVUS_URI = os.environ.get("MILVUS_LITE_URI", str(_DATA / "milvus_lite.db"))

MODEL_NAME = os.environ.get("RAG_EMBED_MODEL", "paraphrase-multilingual-MiniLM-L12-v2")
EMBED_DIM_ENV = int(os.environ.get("RAG_EMBED_DIM", "0"))
# 新集合：按 chunk 存向量（替换旧 opinion_articles_kb 整篇单向量）
COL_NAME = os.environ.get("RAG_MILVUS_COLLECTION", "opinion_chunks_kb")
SYNC_INTERVAL_SEC = int(os.environ.get("RAG_SYNC_INTERVAL_SEC", "120"))
TEXT_MAX_RUNES = 2500

# 切块：控制单条向量文本长度与重叠（字符级，偏中文场景）
CHUNK_MAX_CHARS = int(os.environ.get("RAG_CHUNK_MAX_CHARS", "420"))
CHUNK_OVERLAP = int(os.environ.get("RAG_CHUNK_OVERLAP", "72"))

BATCH_LIMIT = int(os.environ.get("RAG_SYNC_BATCH", "100"))

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
log = logging.getLogger("rag")

_CRAWLER_CFG_PATH = _BASE.parent / "config.py"


def _load_crawler_config() -> Any:
    """当未设置 CRAWLER_DB_* 环境变量时，从 crawler/config.py 读库配置（与爬虫一致）。"""
    if not _CRAWLER_CFG_PATH.is_file():
        return None
    try:
        import importlib.util

        spec = importlib.util.spec_from_file_location(
            "_opinion_crawler_config", _CRAWLER_CFG_PATH
        )
        if spec is None or spec.loader is None:
            return None
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        return mod
    except Exception as e:
        log.warning("optional load %s failed: %s", _CRAWLER_CFG_PATH, e)
        return None


_CRAWLER = _load_crawler_config()


def _env_first(
    keys: Tuple[str, ...], attr: str, fallback: str
) -> str:
    for k in keys:
        if k in os.environ:
            return os.environ[k]
    if _CRAWLER is not None and hasattr(_CRAWLER, attr):
        return str(getattr(_CRAWLER, attr))
    return fallback


def _env_first_int(keys: Tuple[str, ...], attr: str, fallback: int) -> int:
    for k in keys:
        if k in os.environ:
            return int(os.environ[k])
    if _CRAWLER is not None and hasattr(_CRAWLER, attr):
        return int(getattr(_CRAWLER, attr))
    return fallback


# 优先级：环境变量 > crawler/config.py > 内置默认。直接 python server.py 时也能读到 config 里的密码。
DB_HOST = _env_first(("CRAWLER_DB_HOST", "MYSQL_HOST"), "DB_HOST", "127.0.0.1")
DB_PORT = _env_first_int(("CRAWLER_DB_PORT", "MYSQL_PORT"), "DB_PORT", 3306)
DB_USER = _env_first(("CRAWLER_DB_USER", "MYSQL_USER"), "DB_USER", "root")
DB_PASSWORD = _env_first(("CRAWLER_DB_PASSWORD", "MYSQL_PASSWORD"), "DB_PASSWORD", "")
DB_NAME = _env_first(("CRAWLER_DB_NAME", "MYSQL_DATABASE"), "DB_NAME", "opinion_analysis")

_pw_src = (
    "from env"
    if "CRAWLER_DB_PASSWORD" in os.environ or "MYSQL_PASSWORD" in os.environ
    else (
        "from crawler/config.py"
        if _CRAWLER is not None and hasattr(_CRAWLER, "DB_PASSWORD")
        else "empty — set CRAWLER_DB_PASSWORD or crawler/config.py"
    )
)
log.info(
    "MySQL %s:%s user=%s database=%s (password %s)",
    DB_HOST,
    DB_PORT,
    DB_USER,
    DB_NAME,
    _pw_src,
)

_embedder: Embedder | None = None
_client: MilvusClient | None = None
_embed_dim: int = 0
_sync_lock = threading.Lock()
_config_lock = threading.Lock()

EMBED_PROVIDER = os.environ.get("RAG_EMBED_PROVIDER", "local")
EMBED_API_BASE = os.environ.get("RAG_EMBED_API_BASE", "")
EMBED_API_KEY = os.environ.get("RAG_EMBED_API_KEY", "")

# system_settings key -> (env var, default str)
_RAG_SETTING_SPECS: Dict[str, Tuple[str, str]] = {
    "rag.embed_provider": ("RAG_EMBED_PROVIDER", "local"),
    "rag.embed_model": ("RAG_EMBED_MODEL", "paraphrase-multilingual-MiniLM-L12-v2"),
    "rag.embed_api_base": ("RAG_EMBED_API_BASE", ""),
    "rag.embed_api_key": ("RAG_EMBED_API_KEY", ""),
    "rag.chunk_max_chars": ("RAG_CHUNK_MAX_CHARS", "420"),
    "rag.chunk_overlap": ("RAG_CHUNK_OVERLAP", "72"),
    "rag.sync_interval_sec": ("RAG_SYNC_INTERVAL_SEC", "120"),
    "rag.sync_batch": ("RAG_SYNC_BATCH", "100"),
}


def _read_system_setting(key: str, default: str = "") -> str:
    try:
        conn = mysql_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(
                    "SELECT `value` FROM system_settings WHERE `key` = %s LIMIT 1",
                    (key,),
                )
                row = cur.fetchone()
                if row and row.get("value") is not None:
                    return str(row["value"]).strip()
        finally:
            conn.close()
    except Exception as e:
        log.debug("read setting %s: %s", key, e)
    return default


def _effective_setting(db_key: str) -> str:
    env_key, default = _RAG_SETTING_SPECS[db_key]
    if env_key in os.environ:
        return str(os.environ[env_key]).strip()
    return _read_system_setting(db_key, default)


def refresh_runtime_settings() -> None:
    """从 DB / 环境变量刷新可调运行时参数（启动与后台保存后调用）。"""
    global MODEL_NAME, EMBED_PROVIDER, EMBED_API_BASE, EMBED_API_KEY
    global CHUNK_MAX_CHARS, CHUNK_OVERLAP, SYNC_INTERVAL_SEC, BATCH_LIMIT
    with _config_lock:
        EMBED_PROVIDER = (_effective_setting("rag.embed_provider") or "local").strip().lower()
        MODEL_NAME = _effective_setting("rag.embed_model")
        EMBED_API_BASE = _effective_setting("rag.embed_api_base")
        if "RAG_EMBED_API_KEY" in os.environ:
            EMBED_API_KEY = os.environ["RAG_EMBED_API_KEY"]
        else:
            EMBED_API_KEY = _read_system_setting("rag.embed_api_key", "")
        CHUNK_MAX_CHARS = max(128, int(_effective_setting("rag.chunk_max_chars") or "420"))
        CHUNK_OVERLAP = max(0, min(CHUNK_MAX_CHARS // 2, int(_effective_setting("rag.chunk_overlap") or "72")))
        SYNC_INTERVAL_SEC = max(30, int(_effective_setting("rag.sync_interval_sec") or "120"))
        BATCH_LIMIT = max(1, min(500, int(_effective_setting("rag.sync_batch") or "100")))


def _embedder_fingerprint() -> str:
    return f"{EMBED_PROVIDER}|{MODEL_NAME}|{EMBED_API_BASE}|{bool(EMBED_API_KEY)}"


def _reload_embedder_if_needed() -> List[str]:
    """嵌入配置变更时卸载缓存，下次 encode 时重载。返回 warning 列表。"""
    global _embedder, _embed_dim
    warnings: List[str] = []
    old_dim = _embed_dim if _embed_dim > 0 else None
    fp = _embedder_fingerprint()
    with _config_lock:
        if _embedder is not None and getattr(_embedder, "_rag_fingerprint", None) == fp:
            return warnings
        _embedder = None
        _embed_dim = 0
    try:
        emb = get_embedder()
        emb._rag_fingerprint = fp  # type: ignore[attr-defined]
        new_dim = get_embed_dim()
        if old_dim is not None and new_dim != old_dim:
            warnings.append(
                f"向量维度 {old_dim} → {new_dim}，若检索异常请重建 Milvus 集合并重新同步"
            )
    except Exception as e:
        warnings.append(f"加载嵌入模型失败: {e}")
    return warnings


def get_embedder() -> Embedder:
    global _embedder
    refresh_runtime_settings()
    fp = _embedder_fingerprint()
    if _embedder is None or getattr(_embedder, "_rag_fingerprint", None) != fp:
        log.info(
            "loading embedder provider=%s model=%s base=%s",
            EMBED_PROVIDER,
            MODEL_NAME,
            EMBED_API_BASE or "(local)",
        )
        _embedder = create_embedder(EMBED_PROVIDER, MODEL_NAME, EMBED_API_BASE, EMBED_API_KEY)
        try:
            _embedder._rag_fingerprint = fp  # type: ignore[attr-defined]
        except Exception:
            pass
    return _embedder


def encode_embeddings(texts: List[str]) -> List[List[float]]:
    if not texts:
        return []
    return get_embedder().encode(texts, normalize=True)


def encode_query(text: str) -> List[float]:
    return encode_embeddings([text])[0]


def get_embed_dim() -> int:
    global _embed_dim
    if _embed_dim <= 0:
        d = EMBED_DIM_ENV
        if d <= 0:
            d = int(get_embedder().dimension())
        _embed_dim = d
    return _embed_dim


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def clip_runes(s: str, max_runes: int) -> str:
    r = list(s)
    if len(r) <= max_runes:
        return s
    return "".join(r[:max_runes]) + "…"


def build_full_embed_text(title: str, content: str) -> str:
    t = (title or "").strip()
    c = clip_runes((content or "").strip(), TEXT_MAX_RUNES)
    return (t + "\n" + c).strip() if c else t


def semantic_chunks(text: str, max_chars: int | None = None, overlap: int | None = None) -> List[str]:
    """按字符窗口切块，优先在换行、中文标点处断开；块间带重叠利于跨句检索。"""
    if max_chars is None:
        max_chars = CHUNK_MAX_CHARS
    if overlap is None:
        overlap = CHUNK_OVERLAP
    t = (text or "").strip()
    if not t:
        return []
    if len(t) <= max_chars:
        return [t]
    punct = "\n。！？；，、"
    out: List[str] = []
    n = len(t)
    start = 0
    while start < n:
        end = min(start + max_chars, n)
        if end < n:
            best = end
            scan_lo = start + max(1, max_chars - 120)
            for i in range(end - 1, scan_lo - 1, -1):
                if t[i] in punct:
                    best = i + 1
                    break
            end = best
        chunk = t[start:end].strip()
        if chunk:
            out.append(chunk)
        if end >= n:
            break
        start = max(start + 1, end - overlap)
    return out if out else [t[:max_chars]]


def embed_chunk_text(title: str, chunk: str) -> str:
    tit = (title or "").strip()
    ck = (chunk or "").strip()
    if tit:
        return (tit + "\n" + ck).strip()
    return ck


def mysql_conn():
    return pymysql.connect(
        host=DB_HOST,
        port=DB_PORT,
        user=DB_USER,
        password=DB_PASSWORD,
        database=DB_NAME,
        charset="utf8mb4",
        cursorclass=pymysql.cursors.DictCursor,
    )


def ensure_milvus_client() -> MilvusClient:
    global _client
    if _client is not None:
        return _client
    dim = get_embed_dim()
    client = MilvusClient(uri=MILVUS_URI)
    if not client.has_collection(COL_NAME):
        schema = MilvusClient.create_schema(auto_id=False, enable_dynamic_field=False)
        schema.add_field("chunk_pk", DataType.VARCHAR, is_primary=True, max_length=96)
        schema.add_field("article_id", DataType.INT64)
        schema.add_field("chunk_idx", DataType.INT64)
        schema.add_field("embedding", DataType.FLOAT_VECTOR, dim=dim)
        schema.add_field("title", DataType.VARCHAR, max_length=512)
        schema.add_field("snippet", DataType.VARCHAR, max_length=4096)
        schema.add_field("platform", DataType.VARCHAR, max_length=32)
        index_params = MilvusClient.prepare_index_params()
        index_params.add_index("embedding", index_type="AUTOINDEX", metric_type="COSINE")
        client.create_collection(COL_NAME, schema=schema, index_params=index_params)
        log.info("created collection %s dim=%s @ %s", COL_NAME, dim, MILVUS_URI)
    client.load_collection(COL_NAME)
    _client = client
    return client


def delete_chunks_for_article(client: MilvusClient, article_id: int) -> None:
    try:
        client.delete(COL_NAME, filter=f"article_id == {int(article_id)}")
    except Exception as e:
        log.debug("milvus delete chunks: %s", e)


def upsert_article_chunks(
    client: MilvusClient,
    article_id: int,
    title: str,
    content: str,
    platform: str,
) -> int:
    full = build_full_embed_text(title, content)
    pieces = semantic_chunks(full)
    if not pieces:
        delete_chunks_for_article(client, article_id)
        return 0

    embed_inputs = [embed_chunk_text(title, piece) for piece in pieces]
    vecs = encode_embeddings(embed_inputs)

    rows: List[Dict[str, Any]] = []
    for i, (piece, vec) in enumerate(zip(pieces, vecs)):
        h = sha256_text(piece)[:12]
        pk = f"{int(article_id)}:{i}:{h}"
        if len(pk) > 96:
            pk = pk[:96]
        rows.append({
            "chunk_pk": pk,
            "article_id": int(article_id),
            "chunk_idx": i,
            "embedding": vec,
            "title": clip_runes(title or "", 500),
            "snippet": clip_runes(piece, 4000),
            "platform": clip_runes(platform or "", 30),
        })

    delete_chunks_for_article(client, article_id)
    client.insert(COL_NAME, rows)
    return len(rows)


def purge_deleted_from_milvus(client: MilvusClient) -> int:
    conn = mysql_conn()
    n_purged_articles = 0
    try:
        with conn.cursor() as cur:
            cur.execute(
                "SELECT id FROM articles WHERE deleted_at IS NOT NULL LIMIT 400"
            )
            ids = [int(r["id"]) for r in cur.fetchall()]
        for aid in ids:
            delete_chunks_for_article(client, aid)
            n_purged_articles += 1
    finally:
        conn.close()
    return n_purged_articles


def _update_sync_log(
    conn: pymysql.Connection,
    log_id: int,
    *,
    progress: int,
    detail: str,
    articles_processed: int,
    chunks_upserted: int,
    chunks_deleted: int,
) -> None:
    with conn.cursor() as cur:
        cur.execute(
            """
            UPDATE rag_sync_logs
            SET progress=%s, progress_detail=%s, articles_processed=%s,
                chunks_upserted=%s, chunks_deleted=%s
            WHERE id=%s
            """,
            (progress, detail, articles_processed, chunks_upserted, chunks_deleted, log_id),
        )
    conn.commit()


def _finish_sync_log(
    conn: pymysql.Connection,
    log_id: int,
    *,
    ok: bool,
    message: str,
    articles_processed: int,
    chunks_upserted: int,
    chunks_deleted: int,
) -> None:
    status = "success" if ok else "failed"
    now = datetime.now(timezone.utc)
    with conn.cursor() as cur:
        cur.execute(
            """
            UPDATE rag_sync_logs
            SET status=%s, progress=%s, progress_detail=%s, message=%s,
                articles_processed=%s, chunks_upserted=%s, chunks_deleted=%s,
                finished_at=%s
            WHERE id=%s
            """,
            (
                status,
                100 if ok else 0,
                "完成" if ok else "失败",
                message,
                articles_processed,
                chunks_upserted,
                chunks_deleted,
                now,
                log_id,
            ),
        )
    conn.commit()


def incremental_sync(sync_log_id: Optional[int] = None, mode: Optional[str] = None) -> Dict[str, Any]:
    """
    增量：软删文章从 Milvus 清除 chunk；对有变更的正文重新切块写回。
    sync_log_id 非空时写入 rag_sync_logs（与 Go 管理端共用）。
    """
    with _sync_lock:
        return _incremental_sync_impl(sync_log_id, mode)


def _incremental_sync_impl(sync_log_id: Optional[int], mode: Optional[str]) -> Dict[str, Any]:
    client = ensure_milvus_client()
    conn = mysql_conn()
    processed = 0
    upserted = 0
    chunks_up = 0
    chunks_del = 0
    log_id = sync_log_id

    def tick(msg: str) -> None:
        if not log_id:
            return
        pct = min(99, int(processed * 100 / max(1, min(BATCH_LIMIT, 100))))
        _update_sync_log(
            conn,
            log_id,
            progress=pct,
            detail=msg[:2000],
            articles_processed=processed,
            chunks_upserted=chunks_up,
            chunks_deleted=chunks_del,
        )

    try:
        del_cnt = purge_deleted_from_milvus(client)
        chunks_del += del_cnt

        if log_id:
            tick("清理软删文章对应的向量块")

        with conn.cursor() as cur:
            cur.execute(
                f"""
                SELECT id, title, content, platform,
                       embedding_content_hash, embedding_synced_at, updated_at
                FROM articles
                WHERE deleted_at IS NULL
                ORDER BY embedding_synced_at IS NULL DESC, updated_at DESC
                LIMIT {int(BATCH_LIMIT)}
                """
            )
            rows = cur.fetchall()
        total = len(rows)

        for row in rows:
            processed += 1
            text = build_full_embed_text(row.get("title") or "", row.get("content") or "")
            if not text:
                continue
            h = sha256_text(text)
            if row.get("embedding_content_hash") == h and row.get("embedding_synced_at"):
                continue

            aid = int(row["id"])
            n_chunks = upsert_article_chunks(
                client,
                aid,
                str(row.get("title") or ""),
                str(row.get("content") or ""),
                str(row.get("platform") or ""),
            )
            chunks_up += n_chunks

            with conn.cursor() as cur:
                cur.execute(
                    """
                    UPDATE articles
                    SET embedding_content_hash = %s, embedding_synced_at = %s
                    WHERE id = %s
                    """,
                    (h, datetime.now(timezone.utc), aid),
                )
            conn.commit()
            upserted += 1

            if log_id and (processed % 5 == 0 or processed == total):
                tick(f"已处理 {processed}/{total}，本批已向量化 upsert {upserted} 篇")

        result = {
            "processed": processed,
            "upserted": upserted,
            "chunks_upserted": chunks_up,
            "chunks_deleted": chunks_del,
            "collection": COL_NAME,
            "embed_model": MODEL_NAME,
        }
        if log_id:
            _finish_sync_log(
                conn,
                log_id,
                ok=True,
                message=json.dumps(result, ensure_ascii=False)[:65000],
                articles_processed=processed,
                chunks_upserted=chunks_up,
                chunks_deleted=chunks_del,
            )
        return result
    except Exception as e:
        log.exception("incremental_sync failed")
        if log_id:
            try:
                _finish_sync_log(
                    conn,
                    log_id,
                    ok=False,
                    message=str(e)[:65000],
                    articles_processed=processed,
                    chunks_upserted=chunks_up,
                    chunks_deleted=chunks_del,
                )
            except Exception:
                log.exception("failed to update rag_sync_logs")
        raise
    finally:
        conn.close()


def keyword_candidate_ids(query: str) -> List[int]:
    q = (query or "").strip()
    if len(q) < 2:
        return []
    q = clip_runes(q, 80)
    like = f"%{q}%"
    conn = mysql_conn()
    try:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT id FROM articles
                WHERE deleted_at IS NULL
                  AND (title LIKE %s OR content LIKE %s)
                LIMIT 60
                """,
                (like, like),
            )
            return [int(r["id"]) for r in cur.fetchall()]
    except Exception as e:
        log.warning("keyword search: %s", e)
        return []
    finally:
        conn.close()


def distance_to_sim(d: float) -> float:
    try:
        return max(0.0, min(1.0, 1.0 - float(d)))
    except Exception:
        return 0.5


def hybrid_search(query: str, top_k: int) -> List[Dict[str, Any]]:
    client = ensure_milvus_client()
    kw_ids = set(keyword_candidate_ids(query))
    qvec = encode_query(query.strip())

    lim = min(40, max(top_k * 5, top_k))
    raw = client.search(
        COL_NAME,
        data=[qvec],
        anns_field="embedding",
        search_params={"metric_type": "COSINE"},
        limit=lim,
        output_fields=["article_id", "title", "snippet", "platform", "chunk_idx"],
    )

    by_article: Dict[int, Tuple[float, Dict[str, Any]]] = {}
    hits = raw[0] if raw else []
    for hit in hits:
        aid = int(hit.get("entity", {}).get("article_id") or hit.get("id", 0) or 0)
        if aid == 0:
            continue
        title = str(hit.get("entity", {}).get("title") or "")
        snippet = str(hit.get("entity", {}).get("snippet") or "")
        plat = str(hit.get("entity", {}).get("platform") or "")
        dist = float(hit.get("distance", 0.0))
        sim = distance_to_sim(dist)
        if aid in kw_ids:
            sim = min(1.0, sim + 0.12)
        src = "hybrid" if aid in kw_ids else "vector"
        item = {
            "article_id": aid,
            "title": title,
            "snippet": clip_runes(snippet, 1500),
            "platform": plat,
            "score": round(sim, 4),
            "source": src,
            "chunk_idx": int(hit.get("entity", {}).get("chunk_idx") or 0),
        }
        prev = by_article.get(aid)
        if prev is None or sim > prev[0]:
            by_article[aid] = (sim, item)

    scored = sorted(by_article.values(), key=lambda x: -x[0])
    out: List[Dict[str, Any]] = []
    for sim, item in scored[:top_k]:
        out.append(item)
    if len(out) >= top_k:
        return out

    seen = {x["article_id"] for x in out}
    if len(out) < top_k and kw_ids:
        conn = mysql_conn()
        try:
            with conn.cursor() as cur:
                for aid in kw_ids:
                    if aid in seen:
                        continue
                    cur.execute(
                        """
                        SELECT id, title, content, platform
                        FROM articles WHERE id = %s AND deleted_at IS NULL
                        """,
                        (aid,),
                    )
                    r = cur.fetchone()
                    if not r:
                        continue
                    sn = clip_runes(
                        build_full_embed_text(r["title"], r.get("content") or ""), 1200
                    )
                    out.append(
                        {
                            "article_id": int(r["id"]),
                            "title": r.get("title") or "",
                            "snippet": sn,
                            "platform": str(r.get("platform") or ""),
                            "score": 0.35,
                            "source": "keyword",
                            "chunk_idx": 0,
                        }
                    )
                    seen.add(aid)
                    if len(out) >= top_k:
                        break
        finally:
            conn.close()

    return out[:top_k]


class SearchRequest(BaseModel):
    query: str = Field(..., min_length=1)
    top_k: int = Field(8, ge=1, le=20)


class SyncRequest(BaseModel):
    model_config = ConfigDict(populate_by_name=True)
    sync_log_id: Optional[int] = None
    async_: bool = Field(
        default=False,
        validation_alias=AliasChoices("async", "async_"),
    )


app = FastAPI(title="Opinion RAG", version="1.0.0")


def _reschedule_sync_job() -> None:
    sched = getattr(app.state, "scheduler", None)
    if sched is None:
        return
    try:
        sched.reschedule_job("sync", trigger="interval", seconds=SYNC_INTERVAL_SEC)
        log.info("rescheduled sync job interval=%ss", SYNC_INTERVAL_SEC)
    except Exception as e:
        log.warning("reschedule sync job: %s", e)


def _is_sync_enabled() -> bool:
    """检查 system_settings 表中 rag.sync_enabled 是否为 true（默认 true）。"""
    try:
        conn = mysql_conn()
        try:
            with conn.cursor() as cur:
                cur.execute("SELECT value FROM system_settings WHERE `key` = 'rag.sync_enabled' LIMIT 1")
                row = cur.fetchone()
                if row:
                    val = str(row.get("value", "")).strip().lower()
                    return val in ("true", "1", "yes", "on")
                return True  # 默认启用
        finally:
            conn.close()
    except Exception as e:
        log.warning("check rag.sync_enabled failed: %s; defaulting to enabled", e)
        return True


def _scheduled_sync_job() -> None:
    if not _is_sync_enabled():
        log.debug("scheduled sync skipped: rag.sync_enabled is false")
        return
    conn = mysql_conn()
    log_id: Optional[int] = None
    try:
        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO rag_sync_logs
                  (status, progress, progress_detail, message, articles_processed,
                   chunks_upserted, chunks_deleted, mode, started_at)
                VALUES
                  ('running', 0, '', '', 0, 0, 0, 'scheduled', UTC_TIMESTAMP())
                """
            )
            log_id = int(cur.lastrowid)
        conn.commit()
    except Exception as e:
        log.warning("scheduled sync: could not create rag_sync_logs (%s); sync without log", e)
    try:
        incremental_sync(sync_log_id=log_id, mode="scheduled")
    except Exception as e:
        log.exception("scheduled sync failed: %s", e)


def _run_sync_background(
    sync_log_id: Optional[int],
) -> None:
    try:
        incremental_sync(sync_log_id=sync_log_id, mode="manual")
    except Exception as e:
        log.exception("background sync failed: %s", e)


@app.on_event("startup")
def _startup() -> None:
    refresh_runtime_settings()
    ensure_milvus_client()
    sched = BackgroundScheduler()
    sched.add_job(
        _scheduled_sync_job,
        "interval",
        seconds=SYNC_INTERVAL_SEC,
        id="sync",
        max_instances=1,
        coalesce=True,
    )
    sched.start()
    app.state.scheduler = sched
    log.info(
        "RAG milvus=%s collection=%s sync_interval=%ss embed_model=%s dim=%s",
        MILVUS_URI,
        COL_NAME,
        SYNC_INTERVAL_SEC,
        MODEL_NAME,
        get_embed_dim(),
    )


@app.get("/health")
def health() -> Dict[str, Any]:
    refresh_runtime_settings()
    dim = EMBED_DIM_ENV or 384
    if _embedder is not None:
        try:
            dim = get_embed_dim()
        except Exception:
            pass
    return {
        "status": "ok",
        "embed_provider": EMBED_PROVIDER,
        "embed_model": MODEL_NAME,
        "embed_dimension": float(dim),
        "milvus_uri": MILVUS_URI,
        "collection": COL_NAME,
        "sync_interval_sec": float(SYNC_INTERVAL_SEC),
    }


@app.post("/v1/search")
def search(body: SearchRequest) -> Dict[str, Any]:
    try:
        chunks = hybrid_search(body.query.strip(), body.top_k)
        return {"chunks": chunks}
    except Exception as e:
        log.exception("search failed")
        raise HTTPException(status_code=500, detail=str(e)) from e


@app.post("/v1/sync")
def sync_now(background_tasks: BackgroundTasks, body: SyncRequest = SyncRequest()) -> Dict[str, Any]:
    if body.async_:
        background_tasks.add_task(_run_sync_background, body.sync_log_id)
        return {
            "ok": True,
            "async": True,
            "sync_log_id": body.sync_log_id,
            "message": "submitted",
        }
    return incremental_sync(sync_log_id=body.sync_log_id, mode="manual")


@app.post("/v1/rag-config/reload")
def reload_rag_config() -> Dict[str, Any]:
    """从 system_settings 重新加载配置并热更新（持久化与历史由 Go 后端负责）。"""
    refresh_runtime_settings()
    warnings = _reload_embedder_if_needed()
    _reschedule_sync_job()
    out: Dict[str, Any] = {
        "ok": True,
        "embed_provider": EMBED_PROVIDER,
        "embed_model": MODEL_NAME,
        "sync_interval_sec": SYNC_INTERVAL_SEC,
        "sync_batch": BATCH_LIMIT,
    }
    if warnings:
        out["warnings"] = warnings
    log.info(
        "rag config reloaded provider=%s model=%s interval=%ss",
        EMBED_PROVIDER,
        MODEL_NAME,
        SYNC_INTERVAL_SEC,
    )
    return out


@app.get("/v1/articles")
def list_kb_articles(
    page: int = 1,
    page_size: int = 20,
    keyword: Optional[str] = None,
    platform: Optional[str] = None,
    synced: Optional[str] = None,  # "yes" | "no" | ""
) -> Dict[str, Any]:
    """列出文章的向量同步状态（供管理后台查看知识库内容）。"""
    page = max(1, page)
    page_size = min(100, max(1, page_size))
    offset = (page - 1) * page_size

    where_parts = ["deleted_at IS NULL"]
    args: List[Any] = []
    if keyword:
        where_parts.append("(title LIKE %s OR content LIKE %s)")
        like = f"%{keyword}%"
        args += [like, like]
    if platform:
        where_parts.append("platform = %s")
        args.append(platform)
    if synced == "yes":
        where_parts.append("embedding_content_hash IS NOT NULL")
    elif synced == "no":
        where_parts.append("embedding_content_hash IS NULL")

    where_sql = " AND ".join(where_parts)
    try:
        conn = mysql_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(f"SELECT COUNT(*) AS cnt FROM articles WHERE {where_sql}", args)
                total = int((cur.fetchone() or {}).get("cnt", 0))
                cur.execute(
                    f"""
                    SELECT id, title, platform, published_at,
                           embedding_content_hash, embedding_synced_at
                    FROM articles
                    WHERE {where_sql}
                    ORDER BY id DESC
                    LIMIT %s OFFSET %s
                    """,
                    args + [page_size, offset],
                )
                rows = cur.fetchall()
        finally:
            conn.close()
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e)) from e

    items = []
    for r in rows:
        items.append({
            "id": r["id"],
            "title": r.get("title") or "",
            "platform": r.get("platform") or "",
            "publishedAt": str(r["published_at"]) if r.get("published_at") else None,
            "embeddingHash": r.get("embedding_content_hash"),
            "embeddingSyncedAt": str(r["embedding_synced_at"]) if r.get("embedding_synced_at") else None,
            "synced": r.get("embedding_content_hash") is not None,
        })
    return {"total": total, "list": items}


@app.delete("/v1/articles/{article_id}/embedding")
def delete_article_embedding(article_id: int) -> Dict[str, Any]:
    """从 Milvus 中删除指定文章的向量，并清空 MySQL 中的同步标记（使下次同步重新写入）。"""
    try:
        client = ensure_milvus_client()
        delete_chunks_for_article(client, article_id)
    except Exception as e:
        log.warning("delete embedding from milvus: %s", e)
    try:
        conn = mysql_conn()
        try:
            with conn.cursor() as cur:
                cur.execute(
                    "UPDATE articles SET embedding_content_hash = NULL, embedding_synced_at = NULL WHERE id = %s",
                    (article_id,),
                )
            conn.commit()
        finally:
            conn.close()
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e)) from e
    return {"ok": True, "article_id": article_id}


if __name__ == "__main__":
    uvicorn.run(app, host=RAG_HOST, port=RAG_PORT)
