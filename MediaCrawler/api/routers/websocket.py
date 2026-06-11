# -*- coding: utf-8 -*-
import asyncio
from typing import Set, Optional

from fastapi import APIRouter, WebSocket, WebSocketDisconnect

from ..services import crawler_registry

router = APIRouter(tags=["websocket"])


class ConnectionManager:
    """WebSocket connection manager"""

    def __init__(self):
        self.active_connections: Set[WebSocket] = set()

    async def connect(self, websocket: WebSocket):
        await websocket.accept()
        self.active_connections.add(websocket)

    def disconnect(self, websocket: WebSocket):
        self.active_connections.discard(websocket)

    async def broadcast(self, message: dict):
        """Broadcast message to all connections"""
        if not self.active_connections:
            return

        disconnected = []
        for connection in list(self.active_connections):
            try:
                await connection.send_json(message)
            except Exception:
                disconnected.append(connection)

        for conn in disconnected:
            self.disconnect(conn)


manager = ConnectionManager()


async def log_broadcaster():
    """Background task: read logs from aggregated queue and broadcast"""
    queue = crawler_registry.get_aggregated_queue()
    while True:
        try:
            entry = await queue.get()
            await manager.broadcast(entry.model_dump())
        except asyncio.CancelledError:
            break
        except Exception as e:
            print(f"Log broadcaster error: {e}")
            await asyncio.sleep(0.1)


_broadcaster_task: Optional[asyncio.Task] = None


def start_broadcaster():
    """Start broadcast task"""
    global _broadcaster_task
    if _broadcaster_task is None or _broadcaster_task.done():
        _broadcaster_task = asyncio.create_task(log_broadcaster())


@router.websocket("/ws/logs")
async def websocket_logs(websocket: WebSocket):
    """WebSocket log stream (aggregated from all platforms)"""
    try:
        start_broadcaster()

        await manager.connect(websocket)

        # Send existing logs from all platforms
        all_logs = crawler_registry.get_all_logs(200)
        for log in all_logs:
            try:
                await websocket.send_json(log.model_dump())
            except Exception:
                break

        while True:
            try:
                data = await asyncio.wait_for(
                    websocket.receive_text(),
                    timeout=30.0
                )
                if data == "ping":
                    await websocket.send_text("pong")
            except asyncio.TimeoutError:
                try:
                    await websocket.send_text("ping")
                except Exception:
                    break

    except WebSocketDisconnect:
        pass
    except Exception:
        pass
    finally:
        manager.disconnect(websocket)


@router.websocket("/ws/status")
async def websocket_status(websocket: WebSocket):
    """WebSocket status stream (all platforms)"""
    await websocket.accept()

    try:
        while True:
            statuses = crawler_registry.get_all_status()
            await websocket.send_json({"platforms": statuses})
            await asyncio.sleep(1)
    except WebSocketDisconnect:
        pass
    except Exception:
        pass
