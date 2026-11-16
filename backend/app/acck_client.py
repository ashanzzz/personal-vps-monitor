"""
ACCK (acck.io) API client.
All methods return raw dicts/lists from the API.
Token is passed per-call so it can be updated without restart.
"""
from __future__ import annotations

import logging
from typing import Any

import httpx

from .token_utils import bearer_header

logger = logging.getLogger(__name__)

BASE = "https://acck.io/api"
TIMEOUT = 15


def _headers(token: str) -> dict[str, str]:
    # 先剥引号/Bearer 再统一加前缀，否则 `Bearer "eyJ..."` 必然 401
    auth = bearer_header(token)
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
        if isinstance(res, dict):
            if "servers" in res and isinstance(res["servers"], list):
                return res["servers"]
            data = res.get("data") or []
            if isinstance(data, dict):
                return data.get("list") or data.get("servers") or []
            if isinstance(data, list):
                return data
        return []


async def get_exchange_listings(
    page: int = 1,
    page_size: int = 50,
    offset: int = 0,
    limit: int = 50,
    fetch_all: bool = False,
) -> list[dict[str, Any]]:
    """GET /api/exchange/listings – marketplace / secondhand listings.
    ACCK uses cursor-based pagination (offset/limit/next_offset).
    If fetch_all=True, it traverses all pages until has_more is False.
    """
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        if not fetch_all:
            # Map page to offset if offset is 0 but page > 1
            real_offset = offset if offset > 0 else (page - 1) * page_size
            real_limit = min(limit or page_size or 50, 50)
            r = await c.get(
                f"{BASE}/exchange/listings",
                params={"offset": real_offset, "limit": real_limit},
                headers={"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"},
            )
            r.raise_for_status()
            res = r.json()
            if isinstance(res, dict) and "listings" in res:
                return res.get("listings") or []
            return res if isinstance(res, list) else []

        all_listings: list[dict[str, Any]] = []
        cur_offset = offset
        for _ in range(20):  # safety cap up to 1000 items
            r = await c.get(
                f"{BASE}/exchange/listings",
                params={"offset": cur_offset, "limit": 50},
                headers={"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"},
            )
            r.raise_for_status()
            res = r.json()
            batch = res.get("listings") if isinstance(res, dict) else (res if isinstance(res, list) else [])
            if not batch:
                break
            all_listings.extend(batch)
            if not isinstance(res, dict) or not res.get("has_more"):
                break
            next_off = res.get("next_offset")
            if next_off is None or next_off == cur_offset:
                break
            cur_offset = next_off
        return all_listings


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


async def get_order_list(token: str, page: int = 1, page_size: int = 20) -> dict[str, Any]:
    """GET /api/auth/user/order/list – user orders and payment history."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(
            f"{BASE}/auth/user/order/list",
            params={"page": page, "page_size": page_size},
            headers=_headers(token),
        )
        r.raise_for_status()
        return r.json()


async def create_renew_order(token: str, server_id: int, cycle: int = 1) -> dict[str, Any]:
    """POST /api/auth/user/server/renew/create – create renewal order."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(
            f"{BASE}/auth/user/server/renew/create",
            json={"server_id": server_id, "cycle": cycle},
            headers=_headers(token),
        )
        r.raise_for_status()
        return r.json()


async def update_auto_renew(token: str, server_id: int, auto_renew: bool) -> dict[str, Any]:
    """POST /api/auth/user/server/{id}/auto-renew."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(
            f"{BASE}/auth/user/server/{server_id}/auto-renew",
            json={"auto_renew": 1 if auto_renew else 0},
            headers=_headers(token),
        )
        r.raise_for_status()
        return r.json()


async def pay_order(token: str, order_id: int | str, payment_method: str = "balance") -> dict[str, Any]:
    """POST /api/auth/user/order/pay – pay an order via balance or alipay."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(
            f"{BASE}/auth/user/order/pay",
            json={"order_id": order_id, "payment_method": payment_method},
            headers=_headers(token),
        )
        r.raise_for_status()
        return r.json()


async def create_order(token: str, payload: dict[str, Any]) -> dict[str, Any]:
    """POST /api/auth/user/order/create – create new server/service order."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(
            f"{BASE}/auth/user/order/create",
            json=payload,
            headers=_headers(token),
        )
        r.raise_for_status()
        return r.json()


async def buy_exchange_listing(token: str, listing_id: int) -> dict[str, Any]:
    """POST /api/auth/user/exchange/listing/{id}/buy – purchase secondhand listing."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(
            f"{BASE}/auth/user/exchange/listing/{listing_id}/buy",
            json={},
            headers=_headers(token),
        )
        r.raise_for_status()
        return r.json()
