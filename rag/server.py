# -*- coding: utf-8 -*-
"""RAG Embedding 服务 — 仅负责文本向量化，Milvus 读写和同步已迁入 Go 后端。"""
from __future__ import annotations

import logging
import os
from pathlib import Path
from typing import Any, Dict, List

import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from embedder import Embedder, create_embedder

RAG_HOST = os.environ.get("RAG_HOST", "0.0.0.0")
RAG_PORT = int(os.environ.get("RAG_PORT", "5055"))
MODEL_NAME = os.environ.get("RAG_EMBED_MODEL", "paraphrase-multilingual-MiniLM-L12-v2")
EMBED_PROVIDER = os.environ.get("RAG_EMBED_PROVIDER", "local")
EMBED_API_BASE = os.environ.get("RAG_EMBED_API_BASE", "")
EMBED_API_KEY = os.environ.get("RAG_EMBED_API_KEY", "")
EMBED_DIM_ENV = int(os.environ.get("RAG_EMBED_DIM", "0"))

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
log = logging.getLogger("embed")

app = FastAPI(title="RAG Embedding Service")

# 延迟加载：首次 /v1/embed 请求时初始化，避免阻塞启动。
_embedder: Embedder | None = None


def get_embedder() -> Embedder:
    global _embedder
    if _embedder is not None:
        return _embedder
    _embedder = create_embedder(
        provider=EMBED_PROVIDER,
        model_name=MODEL_NAME,
        api_base=EMBED_API_BASE,
        api_key=EMBED_API_KEY,
    )
    log.info("embedder loaded: %s dim=%d", _embedder.label(), _embedder.dimension())
    return _embedder


class EmbedRequest(BaseModel):
    texts: List[str]
    normalize: bool = True


class EmbedResponse(BaseModel):
    vectors: List[List[float]]
    dim: int


@app.get("/health")
def health() -> Dict[str, Any]:
    ready = _embedder is not None
    dim = _embedder.dimension() if ready else EMBED_DIM_ENV
    return {
        "ok": True,
        "embed_provider": EMBED_PROVIDER,
        "embed_model": MODEL_NAME,
        "embed_dimension": dim,
        "embedder_ready": ready,
    }


@app.post("/v1/embed", response_model=EmbedResponse)
def embed(body: EmbedRequest) -> EmbedResponse:
    texts = [t for t in body.texts if t and t.strip()]
    if not texts:
        raise HTTPException(status_code=400, detail="texts 不能为空")
    try:
        emb = get_embedder()
        vecs = emb.encode(texts, normalize=body.normalize)
    except Exception as e:
        log.exception("embed failed")
        raise HTTPException(status_code=500, detail=str(e)) from e
    return EmbedResponse(vectors=vecs, dim=emb.dimension())


if __name__ == "__main__":
    log.info(
        "Embedding service listening on %s:%s model=%s provider=%s",
        RAG_HOST, RAG_PORT, MODEL_NAME, EMBED_PROVIDER,
    )
    uvicorn.run(app, host=RAG_HOST, port=RAG_PORT, log_level="info")
