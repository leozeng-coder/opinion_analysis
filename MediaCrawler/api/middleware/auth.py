# -*- coding: utf-8 -*-
"""
认证中间件：验证请求来自 Go 后端代理
"""
import hmac
import hashlib
import time
from fastapi import Request, HTTPException, status
from starlette.middleware.base import BaseHTTPMiddleware


class ProxyAuthMiddleware(BaseHTTPMiddleware):
    """验证请求来自 Go 后端的中间件"""

    def __init__(self, app, secret_key: str, time_tolerance: int = 300):
        """
        Args:
            app: FastAPI 应用
            secret_key: 与 Go 后端共享的密钥
            time_tolerance: 时间戳容差（秒），防止重放攻击
        """
        super().__init__(app)
        self.secret_key = secret_key
        self.time_tolerance = time_tolerance

    async def dispatch(self, request: Request, call_next):
        # 不需要认证的路径（用于测试和文档）
        exempt_paths = [
            "/api/health",
            "/health",
            "/docs",
            "/redoc",
            "/openapi.json",
            "/",
            "/favicon.ico",
        ]

        # 检查是否是豁免路径
        if request.url.path in exempt_paths:
            return await call_next(request)

        # 检查是否是静态资源路径
        if request.url.path.startswith(("/assets/", "/static/")):
            return await call_next(request)

        # 所有 /api/crawler/* 路径都需要代理认证（生产环境）
        # 如果需要直接测试 API，可以临时注释掉下面的认证逻辑

        # 获取签名和时间戳
        signature = request.headers.get("X-Proxy-Signature")
        timestamp_str = request.headers.get("X-Proxy-Timestamp")

        if not signature or not timestamp_str:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Missing proxy authentication headers"
            )

        try:
            timestamp = int(timestamp_str)
        except ValueError:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Invalid timestamp format"
            )

        # 验证时间戳（防止重放攻击）
        current_time = int(time.time())
        if abs(current_time - timestamp) > self.time_tolerance:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Request timestamp expired"
            )

        # 验证签名
        expected_signature = self._generate_signature(timestamp)
        if not hmac.compare_digest(signature, expected_signature):
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Invalid proxy signature"
            )

        # 验证通过，继续处理请求
        response = await call_next(request)
        return response

    def _generate_signature(self, timestamp: int) -> str:
        """生成 HMAC-SHA256 签名"""
        message = str(timestamp).encode('utf-8')
        h = hmac.new(self.secret_key.encode('utf-8'), message, hashlib.sha256)
        return h.hexdigest()
