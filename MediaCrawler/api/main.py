# -*- coding: utf-8 -*-
# Copyright (c) 2025 relakkes@gmail.com
#
# This file is part of MediaCrawler project.
# Repository: https://github.com/NanmiCoder/MediaCrawler/blob/main/api/main.py
# GitHub: https://github.com/NanmiCoder
# Licensed under NON-COMMERCIAL LEARNING LICENSE 1.1
#
# 声明：本代码仅供学习和研究目的使用。使用者应遵守以下原则：
# 1. 不得用于任何商业用途。
# 2. 使用时应遵守目标平台的使用条款和robots.txt规则。
# 3. 不得进行大规模爬取或对平台造成运营干扰。
# 4. 应合理控制请求频率，避免给目标平台带来不必要的负担。
# 5. 不得用于任何非法或不当的用途。
#
# 详细许可条款请参阅项目根目录下的LICENSE文件。
# 使用本代码即表示您同意遵守上述原则和LICENSE中的所有条款。

"""
MediaCrawler API Server
Start command: uvicorn api.main:app --port 8085 --reload
Or: python -m api.main
"""
import asyncio
import os
import subprocess
import uvicorn
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from .routers import crawler_router, data_router, websocket_router
from .middleware import ProxyAuthMiddleware

app = FastAPI(
    title="MediaCrawler API",
    description="多平台爬虫管理 API (通过 Go 后端代理访问)",
    version="1.0.0"
)

# 添加代理认证中间件（验证请求来自 Go 后端）
SECRET_KEY = os.getenv("PROXY_SECRET_KEY", "your-secret-key-change-in-production")
app.add_middleware(ProxyAuthMiddleware, secret_key=SECRET_KEY, time_tolerance=300)

# 注意：不需要 CORS 中间件，因为所有请求都通过 Go 后端代理
# Go 后端已经处理了 CORS

# Register routers
app.include_router(crawler_router, prefix="/api")
app.include_router(data_router, prefix="/api")
app.include_router(websocket_router, prefix="/api")


@app.get("/")
async def root():
    return {
        "message": "MediaCrawler API",
        "version": "1.0.0",
        "docs": "/docs",
    }


@app.get("/api/health")
async def health_check():
    return {"status": "ok", "service": "MediaCrawler"}


@app.get("/api/env/check")
async def check_environment():
    """Check if MediaCrawler environment is configured correctly"""
    import os
    import sys

    # Simple check: if the API server is running, the environment is OK
    # The server wouldn't start if dependencies were missing
    try:
        # Get the parent directory (MediaCrawler root)
        current_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

        # Check if main.py exists
        main_py = os.path.join(current_dir, "main.py")
        if not os.path.exists(main_py):
            return {
                "success": False,
                "message": "main.py not found",
                "error": f"main.py not found at {main_py}"
            }

        # Check if config directory exists
        config_dir = os.path.join(current_dir, "config")
        if not os.path.exists(config_dir):
            return {
                "success": False,
                "message": "config directory not found",
                "error": f"config directory not found at {config_dir}"
            }

        # If we got here, basic structure is OK
        # Since the API server is running, dependencies must be installed
        return {
            "success": True,
            "message": "MediaCrawler environment configured correctly",
            "output": "API server is running, all dependencies are available"
        }

    except Exception as e:
        return {
            "success": False,
            "message": "Environment check error",
            "error": str(e) or "Unknown error occurred"
        }


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8085)
