"""
Akile API client.  All methods return raw dicts/lists from the API.
Token is passed per-call so it can be updated without restart.
"""
from __future__ import annotations

import httpx

BASE = "https://api.akilecloud.com/api"
TIMEOUT = 15

# Token refresh uses a different host (the console API gateway).
REFRESH_URL = "https://api.akile.ai/api/v1/user/refreshToken"
REFRESH_TIMEOUT = 25


def _headers(token: str) -> dict:
    return {"Authorization": token, "Content-Type": "application/json"}


async def refresh_token(token: str) -> str:
    """Exchange a still-valid token for a fresh 24h token.

    Akile's /v1/user/refreshToken accepts the current (non-expired) token and
    returns a brand-new 24h token. Calling it before expiry keeps the token
    alive indefinitely with no login / 2FA. Returns the new token string.
    Raises RuntimeError if the refresh did not succeed.
    """
    headers = {
        "Authorization": token,  # interceptor expects raw token, no Bearer prefix
        "Origin": "https://akile.ai",
        "Referer": "https://akile.ai/",
        "User-Agent": (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
            "Chrome/126.0 Safari/537.36"
        ),
        "Accept": "application/json, text/plain, */*",
    }
    async with httpx.AsyncClient(timeout=REFRESH_TIMEOUT) as c:
        r = await c.get(REFRESH_URL, headers=headers)
        r.raise_for_status()
        data = r.json()

    new = (data.get("data") or {}).get("token", "")
    status_msg = data.get("status_msg", "")
    if "成功" in status_msg and new and new.count(".") == 2:
        return new
    raise RuntimeError(f"Token refresh failed: {data.get('status_msg') or data}")


async def get_vps_store(token: str) -> dict:
    """GET /v1/store/GetVpsStoreV3?areaId=1 – returns all areas."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/v1/store/GetVpsStoreV3", params={"areaId": 1},
                        headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def get_ls_store(token: str, region: str) -> dict:
    """GET /v1/store/LSStore?region=X – returns Bundles for one region."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/v1/store/LSStore", params={"region": region},
                        headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def get_secondhand_list(token: str, page: int = 1, page_size: int = 100) -> dict:
    """POST /v1/pushshop/GetPushProductList – returns current listings."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(f"{BASE}/v1/pushshop/GetPushProductList",
                         json={"page": page, "pageSize": page_size},
                         headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def get_secondhand_areas(token: str) -> list[str]:
    """GET /v1/pushshop/GetPushProductArea – returns area name list."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/v1/pushshop/GetPushProductArea",
                        headers=_headers(token))
        r.raise_for_status()
        return r.json().get("data") or []


async def get_recycle_prices(token: str) -> list[dict]:
    """GET /v1/pushshop/GetAllRecyclePackage – historical pricing."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/v1/pushshop/GetAllRecyclePackage",
                        headers=_headers(token))
        r.raise_for_status()
        return r.json().get("data") or []


async def buy_secondhand(token: str, payload: dict) -> dict:
    """POST /v1/pushshop/PushShopUserTx – place purchase order."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(f"{BASE}/v1/pushshop/PushShopUserTx",
                         json=payload, headers=_headers(token))
        r.raise_for_status()
        return r.json()
