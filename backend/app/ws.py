"""
WebSocket connection manager – broadcast JSON messages to all connected clients.
"""
from __future__ import annotations

import json
import logging
from typing import Any

from fastapi import WebSocket

logger = logging.getLogger(__name__)

_clients: set[WebSocket] = set()


async def connect(ws: WebSocket) -> None:
    await ws.accept()
    _clients.add(ws)


def disconnect(ws: WebSocket) -> None:
    _clients.discard(ws)


async def broadcast(payload: Any) -> None:
    if not _clients:
        return
    text = json.dumps(payload, ensure_ascii=False)
    dead: list[WebSocket] = []
    for ws in list(_clients):
        try:
            await ws.send_text(text)
        except Exception:
            dead.append(ws)
    for ws in dead:
        _clients.discard(ws)
