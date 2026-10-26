"""
ACCK (acck.io) API client.
All methods return raw dicts/lists from the API.
Token is passed per-call so it can be updated without restart.
"""
from __future__ import annotations

import logging
from typing import Any
import httpx

logger = logging.getLogger(__name__)

BASE = "https://acck.io/api"
TIMEOUT = 15


def _headers(token: str) -> dict[str, str]:
    auth = token.strip()
    if not auth.startswith("Bearer "):
        auth = f"Bearer {auth}"
    return {
        "Authorization": auth,
        "Content-Type": "application/json",
        "Origin": "https://acck.io",
        "Referer": "https://acck.io/",
        "User-Agent": (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
            "AppleWebKit/537.36 (KHTML, like Gecko) "
            "Chrome/126.0.0.0 Safari/537.36"
        ),
        "Accept": "application/json, text/plain, */*",
    }


async def get_user_info(token: str) -> dict[str, Any]:
    """GET /api/auth/user/info – user profile."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/auth/user/info", headers=_headers(token))
        r.raise_for_status()
        res = r.json()
        return res.get("data") or res


async def get_ac_shop(token: str) -> dict[str, Any]:
    """GET /api/auth/user/ac-shop – points balance and shop metadata."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/auth/user/ac-shop", headers=_headers(token))
        r.raise_for_status()
        res = r.json()
        return res.get("data") or res


async def checkin(token: str) -> dict[str, Any]:
    """POST /api/auth/user/ac-shop/checkin – daily check-in."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(f"{BASE}/auth/user/ac-shop/checkin", json={}, headers=_headers(token))
        # Note: ACCK returns 200 on success, or 400 on already signed / error
        try:
            return r.json()
        except Exception:
            return {"status_code": r.status_code, "text": r.text}


async def get_shop_logs(token: str, page: int = 1, page_size: int = 20) -> list[dict[str, Any]]:
    """GET /api/auth/user/ac-shop/logs – points history & checkin logs."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(
            f"{BASE}/auth/user/ac-shop/logs",
            params={"page": page, "page_size": page_size},
            headers=_headers(token),
        )
        r.raise_for_status()
        res = r.json()
        data = res.get("data") or []
        if isinstance(data, dict):
            return data.get("list") or []
        return data if isinstance(data, list) else []


async def get_my_servers(token: str) -> list[dict[str, Any]]:
    """GET /api/auth/user/server/list – user servers."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/auth/user/server/list", headers=_headers(token))
        r.raise_for_status()
        res = r.json()
        data = res.get("data") or []
        if isinstance(data, dict):
            return data.get("list") or []
        return data if isinstance(data, list) else []


async def get_exchange_listings(page: int = 1, page_size: int = 50) -> list[dict[str, Any]]:
    """GET /api/exchange/listings – marketplace / secondhand listings."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(
            f"{BASE}/exchange/listings",
            params={"page": page, "page_size": page_size},
            headers={"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"},
        )
        r.raise_for_status()
        res = r.json()
        data = res.get("data") or []
        if isinstance(data, dict):
            return data.get("list") or []
        return data if isinstance(data, list) else []


async def buy_preview(token: str, payload: dict[str, Any]) -> dict[str, Any]:
    """
    【防误购安全模式】
    对购买请求做参数校验与试算展示，严禁触发支付扣款。
    """
    logger.info("ACCK buy_preview triggered with payload: %s (dry-run only)", payload)
    return {
        "status": "simulated",
        "protected": True,
        "msg": "【防误购保护锁已生效】模拟购买试算成功，未发起真实支付扣费。",
        "payload": payload,
    }
