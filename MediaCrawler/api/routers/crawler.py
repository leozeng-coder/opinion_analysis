# -*- coding: utf-8 -*-
from typing import List

from fastapi import APIRouter, HTTPException

from ..schemas import CrawlerStartRequest, CrawlerStatusResponse
from ..services import crawler_manager, crawler_registry

router = APIRouter(prefix="/crawler", tags=["crawler"])


# ==================== Per-platform routes ====================

@router.post("/{platform}/start")
async def start_crawler_platform(platform: str, request: CrawlerStartRequest):
    """Start crawler task for a specific platform"""
    mgr = crawler_registry.get(platform)
    success = await mgr.start(request)
    if not success:
        if mgr.process and mgr.process.poll() is None:
            raise HTTPException(status_code=400, detail=f"Crawler for {platform} is already running")
        raise HTTPException(status_code=500, detail=f"Failed to start crawler for {platform}")
    return {"status": "ok", "message": f"Crawler started for {platform}"}


@router.post("/{platform}/stop")
async def stop_crawler_platform(platform: str):
    """Stop crawler task for a specific platform"""
    mgr = crawler_registry.get(platform)
    success = await mgr.stop()
    if not success:
        if not mgr.process or mgr.process.poll() is not None:
            raise HTTPException(status_code=400, detail=f"No crawler running for {platform}")
        raise HTTPException(status_code=500, detail=f"Failed to stop crawler for {platform}")
    return {"status": "ok", "message": f"Crawler stopped for {platform}"}


@router.get("/{platform}/status", response_model=CrawlerStatusResponse)
async def get_crawler_status_platform(platform: str):
    """Get crawler status for a specific platform"""
    mgr = crawler_registry.get(platform)
    return mgr.get_status()


@router.get("/{platform}/logs")
async def get_logs_platform(platform: str, limit: int = 100):
    """Get recent logs for a specific platform"""
    mgr = crawler_registry.get(platform)
    logs = mgr.logs[-limit:] if limit > 0 else mgr.logs
    return {"logs": [log.model_dump() for log in logs]}


# ==================== Aggregated routes ====================

@router.get("/status/all")
async def get_all_status():
    """Get status of all platform crawlers"""
    return {"platforms": crawler_registry.get_all_status()}


@router.post("/stop/all")
async def stop_all_crawlers():
    """Stop all running crawlers"""
    count = await crawler_registry.stop_all()
    return {"status": "ok", "stopped_count": count}


# ==================== Legacy routes (backward-compatible) ====================

@router.post("/start")
async def start_crawler(request: CrawlerStartRequest):
    """Start crawler task (legacy, uses platform from request body)"""
    platform = request.platform.value
    mgr = crawler_registry.get(platform)
    success = await mgr.start(request)
    if not success:
        if mgr.process and mgr.process.poll() is None:
            raise HTTPException(status_code=400, detail="Crawler is already running")
        raise HTTPException(status_code=500, detail="Failed to start crawler")
    return {"status": "ok", "message": "Crawler started successfully"}


@router.post("/stop")
async def stop_crawler():
    """Stop crawler task (legacy, stops first running)"""
    running = crawler_registry.get_running()
    if not running:
        raise HTTPException(status_code=400, detail="No crawler is running")
    success = await running[0].stop()
    if not success:
        raise HTTPException(status_code=500, detail="Failed to stop crawler")
    return {"status": "ok", "message": "Crawler stopped successfully"}


@router.get("/status", response_model=CrawlerStatusResponse)
async def get_crawler_status():
    """Get crawler status (legacy, returns first running or idle)"""
    running = crawler_registry.get_running()
    if running:
        return running[0].get_status()
    return {"status": "idle", "platform": None, "crawler_type": None, "started_at": None, "error_message": None}


@router.get("/logs")
async def get_logs(limit: int = 100, after_seq: int = 0):
    """Get recent logs (legacy, returns aggregated).

    Args:
        limit: 返回条数上限（兜底）。
        after_seq: 增量拉取，只返回 seq > after_seq 的日志；0 表示全量。
                   响应额外返回 lastSeq（本批最大 seq），供前端下次增量请求使用。
    """
    logs = crawler_registry.get_all_logs(limit, after_seq)
    last_seq = logs[-1].seq if logs else after_seq
    return {"logs": [log.model_dump() for log in logs], "lastSeq": last_seq}


# ==================== Utility routes ====================

@router.get("/platforms")
async def get_platforms():
    """获取支持的平台列表"""
    return {
        "platforms": [
            {"value": "xhs", "label": "小红书", "icon": "book-open"},
            {"value": "dy", "label": "抖音", "icon": "music"},
            {"value": "ks", "label": "快手", "icon": "video"},
            {"value": "bili", "label": "B站", "icon": "tv"},
            {"value": "wb", "label": "微博", "icon": "message-circle"},
            {"value": "tieba", "label": "百度贴吧", "icon": "messages-square"},
            {"value": "zhihu", "label": "知乎", "icon": "help-circle"},
        ]
    }


@router.get("/options")
async def get_config_options():
    """获取所有配置选项"""
    return {
        "login_types": [
            {"value": "qrcode", "label": "二维码登录"},
            {"value": "cookie", "label": "Cookie登录"},
        ],
        "crawler_types": [
            {"value": "search", "label": "关键词搜索"},
            {"value": "detail", "label": "指定ID"},
            {"value": "creator", "label": "创作者主页"},
        ],
        "save_options": [
            {"value": "db", "label": "MySQL数据库"},
            {"value": "json", "label": "JSON文件"},
            {"value": "jsonl", "label": "JSONL文件"},
            {"value": "csv", "label": "CSV文件"},
            {"value": "sqlite", "label": "SQLite数据库"},
            {"value": "mongodb", "label": "MongoDB数据库"},
            {"value": "excel", "label": "Excel文件"},
        ],
    }
