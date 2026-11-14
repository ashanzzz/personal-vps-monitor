"""
LazyCatCloud (dash.lazycat.wiki) API client.
All methods return raw dicts/lists from the API.
Token is passed per-call so it can be updated dynamically.
"""
from __future__ import annotations

import logging
import os
from typing import Any
import httpx

logger = logging.getLogger(__name__)

BASE = "https://dash.lazycat.wiki/api"
TIMEOUT = 15

# Default to our clean local proxy (192.168.8.11:7890) to ensure zero Cloudflare 403 blocks
PROXY = os.environ.get("HTTPS_PROXY") or os.environ.get("HTTP_PROXY") or "http://192.168.8.11:7890"


def _headers(token: str | None = None) -> dict[str, str]:
    headers = {
        "Content-Type": "application/json",
        "Origin": "https://dash.lazycat.wiki",
        "Referer": "https://dash.lazycat.wiki/",
        "User-Agent": (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
            "AppleWebKit/537.36 (KHTML, like Gecko) "
            "Chrome/126.0.0.0 Safari/537.36"
        ),
        "Accept": "application/json, text/plain, */*",
    }
    if token:
        auth = token.strip()
        if not auth.startswith("Bearer "):
            auth = f"Bearer {auth}"
        headers["Authorization"] = auth
    return headers


def _get_client(timeout: float = TIMEOUT) -> httpx.AsyncClient:
    return httpx.AsyncClient(timeout=timeout, proxy=PROXY)


# --- Authentication & User Profile ---

async def login(username: str, password: str, device_fingerprint: str = "") -> dict[str, Any]:
    """POST /auth/login - Turnstile is currently disabled on dash.lazycat.wiki."""
    payload = {
        "username": username,
        "password": password,
        "totpCode": None,
        "recoveryCode": None,
        "turnstileToken": None,
        "deviceFingerprint": device_fingerprint,
    }
    async with _get_client() as c:
        r = await c.post(f"{BASE}/auth/login", json=payload, headers=_headers())
        r.raise_for_status()
        return r.json()


async def get_user_info(token: str) -> dict[str, Any]:
    """GET /auth/me - Current user identity & VIP level."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/auth/me", headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def get_user_balance(token: str) -> dict[str, Any]:
    """GET /balance/me - Balance, total recharge, total consume & VIP threshold."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/balance/me", headers=_headers(token))
        r.raise_for_status()
        res = r.json()
        return res.get("balance") or res


async def get_vip_card(token: str) -> dict[str, Any]:
    """GET /vip-card/me - VIP card status, perks & rebate history."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/vip-card/me", headers=_headers(token))
        r.raise_for_status()
        return r.json()


# --- Daily Checkin & Points ---

async def get_checkin_status(token: str) -> dict[str, Any]:
    """GET /points-center/checkin/status - Daily check-in status."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/points-center/checkin/status", headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def checkin(token: str) -> dict[str, Any]:
    """POST /points-center/checkin/ - Perform daily check-in."""
    async with _get_client() as c:
        r = await c.post(f"{BASE}/points-center/checkin/", json={}, headers=_headers(token))
        try:
            return r.json()
        except Exception:
            return {"status_code": r.status_code, "text": r.text}


# --- Public Store & Instances ---

async def get_public_packages() -> dict[str, Any]:
    """GET /packages/public - Public store package catalog (39 packages)."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/packages/public", headers=_headers())
        r.raise_for_status()
        return r.json()


async def get_public_regions() -> dict[str, Any]:
    """GET /packages/public/regions - Available regions & package counts."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/packages/public/regions", headers=_headers())
        r.raise_for_status()
        return r.json()


async def get_my_instances(token: str) -> list[dict[str, Any]]:
    """GET /instances - User instances list."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/instances", headers=_headers(token))
        r.raise_for_status()
        res = r.json()
        if isinstance(res, dict):
            return res.get("instances") or []
        return res if isinstance(res, list) else []


# --- Secondary Market (instanceMarket) ---

async def get_market_config(token: str) -> dict[str, Any]:
    """GET /instance-market/config - Market trading rules, fees & waiver."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/instance-market/config", headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def get_market_listings(
    token: str,
    region: str | None = None,
    page: int = 1,
    page_size: int = 20,
) -> dict[str, Any]:
    """GET /instance-market/listings - In-market listings for sale."""
    params: dict[str, Any] = {"page": page, "pageSize": page_size}
    if region:
        params["region"] = region.lower()
    async with _get_client() as c:
        r = await c.get(f"{BASE}/instance-market/listings", params=params, headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def get_eligible_instances(token: str) -> list[dict[str, Any]]:
    """GET /instance-market/eligible-instances - Instances eligible for listing."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/instance-market/eligible-instances", headers=_headers(token))
        r.raise_for_status()
        res = r.json()
        return res.get("instances") or []


async def purchase_instance(token: str, listing_id: int | str) -> dict[str, Any]:
    """POST /instance-market/listings/{id}/purchase - Purchase an instance."""
    async with _get_client() as c:
        r = await c.post(f"{BASE}/instance-market/listings/{listing_id}/purchase", json={}, headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def create_listing(
    token: str,
    instance_id: int,
    title: str,
    remark: str,
    sale_price: float,
    ip_status: str = "normal",
) -> dict[str, Any]:
    """POST /instance-market/listings - Put an instance on sale."""
    payload = {
        "instanceId": instance_id,
        "title": title,
        "remark": remark,
        "salePrice": sale_price,
        "ipStatus": ip_status,
    }
    async with _get_client() as c:
        r = await c.post(f"{BASE}/instance-market/listings", json=payload, headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def cancel_listing(token: str, listing_id: int | str) -> dict[str, Any]:
    """POST /instance-market/listings/{id}/cancel - Delist machine from market."""
    async with _get_client() as c:
        r = await c.post(f"{BASE}/instance-market/listings/{listing_id}/cancel", json={}, headers=_headers(token))
        r.raise_for_status()
        return r.json()


# --- Entertainment / Lottery Engine ---

async def get_lotteries(token: str) -> list[dict[str, Any]]:
    """GET /entertainment/lotteries - Active lucky wheel / lottery campaigns."""
    async with _get_client() as c:
        r = await c.get(f"{BASE}/entertainment/lotteries", headers=_headers(token))
        r.raise_for_status()
        res = r.json()
        return res if isinstance(res, list) else res.get("lotteries") or []


async def draw_lottery(token: str, lottery_id: int | str) -> dict[str, Any]:
    """POST /entertainment/lotteries/{id}/draw - Draw lottery."""
    async with _get_client() as c:
        r = await c.post(f"{BASE}/entertainment/lotteries/{lottery_id}/draw", json={}, headers=_headers(token))
        r.raise_for_status()
        return r.json()
