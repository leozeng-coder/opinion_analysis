# -*- coding: utf-8 -*-
"""句向量嵌入：本地 Sentence-Transformers 或 OpenAI 兼容 HTTP API。"""
from __future__ import annotations

import json
import logging
import math
import urllib.error
import urllib.request
from typing import List, Optional, Protocol

log = logging.getLogger("rag.embedder")


class Embedder(Protocol):
    def encode(self, texts: List[str], normalize: bool = True) -> List[List[float]]: ...

    def dimension(self) -> int: ...

    def label(self) -> str: ...


class LocalSentenceTransformerEmbedder:
    def __init__(self, model_name: str) -> None:
        from sentence_transformers import SentenceTransformer

        log.info("loading sentence-transformers: %s", model_name)
        self._model_name = model_name
        self._model = SentenceTransformer(model_name)

    def encode(self, texts: List[str], normalize: bool = True) -> List[List[float]]:
        if not texts:
            return []
        vecs = self._model.encode(
            texts, normalize_embeddings=normalize, show_progress_bar=False
        )
        return [v.tolist() for v in vecs]

    def dimension(self) -> int:
        m = self._model
        if hasattr(m, "get_sentence_embedding_dimension"):
            return int(m.get_sentence_embedding_dimension())
        return int(m.get_embedding_dimension())

    def label(self) -> str:
        return f"local:{self._model_name}"


class OpenAICompatibleEmbedder:
    """调用 OpenAI 兼容 POST /v1/embeddings（DeepSeek、OpenAI、Jina、百炼等）。"""

    def __init__(
        self,
        base_url: str,
        api_key: str,
        model: str,
        timeout: float = 90.0,
    ) -> None:
        self.base_url = (base_url or "").strip().rstrip("/")
        self.api_key = (api_key or "").strip()
        self.model = (model or "").strip()
        self.timeout = timeout
        self._dim: Optional[int] = None
        if not self.base_url:
            raise ValueError("embed_api_base required for api provider")
        if not self.api_key:
            raise ValueError("embed_api_key required for api provider")
        if not self.model:
            raise ValueError("embed_model required for api provider")

    def _endpoint(self) -> str:
        base = self.base_url.rstrip("/")
        if base.endswith("/embeddings"):
            return base
        if base.endswith("/v1"):
            return base + "/embeddings"
        return base + "/v1/embeddings"

    def encode(self, texts: List[str], normalize: bool = True) -> List[List[float]]:
        if not texts:
            return []
        payload: dict = {"model": self.model, "input": texts if len(texts) > 1 else texts[0]}
        req = urllib.request.Request(
            self._endpoint(),
            data=json.dumps(payload).encode("utf-8"),
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self.api_key}",
            },
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                body = json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as e:
            detail = e.read().decode("utf-8", errors="replace")[:500]
            raise RuntimeError(f"embedding API HTTP {e.code}: {detail}") from e
        except urllib.error.URLError as e:
            raise RuntimeError(f"embedding API unreachable: {e.reason}") from e

        items = body.get("data") or []
        items.sort(key=lambda x: int(x.get("index", 0)))
        vecs = [item.get("embedding") or [] for item in items]
        if len(vecs) != len(texts):
            raise RuntimeError(f"embedding API returned {len(vecs)} vectors for {len(texts)} inputs")

        if not normalize:
            return vecs
        out: List[List[float]] = []
        for v in vecs:
            norm = math.sqrt(sum(x * x for x in v)) or 1.0
            out.append([x / norm for x in v])
        return out

    def dimension(self) -> int:
        if self._dim is None:
            self._dim = len(self.encode(["probe"], normalize=False)[0])
        return self._dim

    def label(self) -> str:
        return f"api:{self.model}@{self.base_url}"


def create_embedder(provider: str, model_name: str, api_base: str, api_key: str) -> Embedder:
    p = (provider or "local").strip().lower()
    if p in ("api", "openai", "remote", "http"):
        return OpenAICompatibleEmbedder(api_base, api_key, model_name)
    return LocalSentenceTransformerEmbedder(model_name)
