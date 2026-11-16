"""
Akile API client.  All methods return raw dicts/lists from the API.
Token is passed per-call so it can be updated without restart.
"""
from __future__ import annotations

from typing import Any

import httpx

from .token_utils import bare_header

BASE = "https://api.akile.ai/api"
BASE_AI = "https://api.akile.ai/api"
TIMEOUT = 15

# Token refresh uses a different host (the console API gateway).
REFRESH_URL = "https://api.akile.ai/api/v1/user/refreshToken"
REFRESH_TIMEOUT = 25


def _headers(token: str) -> dict:
    # Akile 要裸 JWT：加 Bearer 前缀反而 401
    return {"Authorization": bare_header(token), "Content-Type": "application/json"}


def _ai_headers(token: str) -> dict:
    """Headers for endpoints that require the akile.ai host."""
    return {
        "Authorization": bare_header(token),
        "Origin": "https://akile.ai",
        "Referer": "https://akile.ai/",
        "User-Agent": (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
            "Chrome/126.0 Safari/537.36"
        ),
        "Accept": "application/json, text/plain, */*",
    }


async def refresh_token(token: str) -> str:
    """Exchange a still-valid token for a fresh 24h token.

    Akile's /v1/user/refreshToken accepts the current (non-expired) token and
    returns a brand-new 24h token. Calling it before expiry keeps the token
    alive indefinitely with no login / 2FA. Returns the new token string.
    Raises RuntimeError if the refresh did not succeed.
    """
    headers = {
        "Authorization": bare_header(token),  # 裸 JWT，且必须剥掉 JSON 引号
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


async def get_secondhand_list(
    token: str,
    page: int = 1,
    page_size: int = 30,
    name: str = "",
    area_name: str = "",
    fetch_all: bool = False,
) -> dict:
    """POST /v1/pushshop/GetPushProductList – returns current listings.
    If fetch_all=True, it concurrently traverses all pages until the full catalog is collected.
    """
    import asyncio
    import math

    payload: dict[str, Any] = {"page_num": page, "page_size": min(page_size, 30)}
    if name:
        payload["name"] = name
    if area_name:
        payload["area_name"] = area_name

    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(
            f"{BASE}/v1/pushshop/GetPushProductList",
            json=payload,
            headers=_headers(token),
        )
        r.raise_for_status()
        first_page = r.json()
        if not fetch_all or name:
            return first_page

        total = first_page.get("total", 0) or 0
        items = list(first_page.get("list") or [])
        pages = math.ceil(total / 30)
        if pages > 1:
            tasks = []
            for p in range(2, min(pages + 1, 15)):  # safety cap up to 450 items
                p_payload: dict[str, Any] = {"page_num": p, "page_size": 30}
                if area_name:
                    p_payload["area_name"] = area_name
                tasks.append(
                    c.post(
                        f"{BASE}/v1/pushshop/GetPushProductList",
                        json=p_payload,
                        headers=_headers(token),
                    )
                )
            responses = await asyncio.gather(*tasks, return_exceptions=True)
            for resp in responses:
                if isinstance(resp, httpx.Response) and resp.status_code == 200:
                    try:
                        data = resp.json()
                        items.extend(data.get("list") or [])
                    except Exception:
                        pass
        first_page["list"] = items
        return first_page


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


async def buy_secondhand(token: str, product_id: int | str, auto_pay: bool = True) -> dict:
    """Purchase a secondhand product from PushShop via official order flow.

    1. POST /v1/store/makeOrder with order_type="6" and productId
    2. If auto_pay=True, POST /v1/pay to immediately settle with account balance
    """
    payload = {
        "order_type": "6",
        "order_data": [
            {"key": "productId", "name": "productId", "value": str(product_id)},
        ],
    }
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(f"{BASE}/v1/store/makeOrder", json=payload, headers=_headers(token))
        r.raise_for_status()
        res = r.json()

    order_no = ""
    if isinstance(res, dict):
        order_no = res.get("order_no") or (res.get("data") or {}).get("order_no", "")

    pay_res = None
    if auto_pay and order_no:
        try:
            pay_res = await pay_order(token, order_no, pay_type="balance")
        except Exception as e:
            pay_res = {"error": f"Auto-pay failed: {e}"}

    return {
        "order_result": res,
        "order_no": order_no,
        "paid": pay_res is not None and "error" not in pay_res,
        "pay_result": pay_res,
    }


async def get_user_info(token: str) -> dict:
    """GET /v1/user/info – account profile + balance.

    Returns data dict with: username, email, role, money (balance in fen),
    telegram_id, is_real_name, is_ga, last_checkin_time, etc.
    Verified live: both api.akilecloud.com and api.akile.ai serve this; we use
    the store host for consistency with the rest of the client.
    """
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/v1/user/info", headers=_headers(token))
        r.raise_for_status()
        return r.json().get("data") or {}


async def get_my_servers(token: str) -> list[dict]:
    """POST /v1/server/GetServerV2List – the account's own instances."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(f"{BASE}/v1/server/GetServerV2List",
                         json={}, headers=_headers(token))
        r.raise_for_status()
        res = r.json()
        if isinstance(res, dict):
            if "list" in res and isinstance(res["list"], list):
                return res["list"]
            data = res.get("data") or {}
            if isinstance(data, dict):
                return data.get("list") or []
        return []


async def checkin(token: str) -> dict:
    """GET /v1/user/Checkin – daily sign-in for AK coins.

    Returns the raw JSON response dict.  Successful: status_code=0,
    already signed: status_code=1.
    """
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE_AI}/v1/user/Checkin", headers=_ai_headers(token))
        r.raise_for_status()
        return r.json()


async def get_ak_coin_info(token: str) -> dict:
    """GET /v1/user/index – account dashboard including AK coin balance.

    Returns the data dict (contains ak_coin, money, vps_num, order_num, etc.).
    """
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE_AI}/v1/user/index", headers=_ai_headers(token))
        r.raise_for_status()
        return r.json().get("data") or {}


async def get_order_list(token: str, page: int = 1, page_size: int = 20) -> dict:
    """POST /v1/order/GetOrderListByUserId – user's order history."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(
            f"{BASE}/v1/order/GetOrderListByUserId",
            json={"page": page, "page_size": page_size},
            headers=_headers(token),
        )
        r.raise_for_status()
        return r.json()


async def get_money_logs(token: str, page: int = 1, page_size: int = 20) -> dict:
    """POST /v1/user/GetMoneyLog – balance and transaction history."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(
            f"{BASE}/v1/user/GetMoneyLog",
            json={"page": page, "page_size": page_size},
            headers=_headers(token),
        )
        r.raise_for_status()
        return r.json()


async def get_billing_services(token: str) -> dict:
    """GET /v1/billing/service – active services and renewal billing."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/v1/billing/service", headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def get_billing_orders(token: str) -> dict:
    """GET /v1/billing/order – pending billing invoices."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/v1/billing/order", headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def get_server_detail(token: str, server_id: int) -> dict:
    """GET /v1/server/GetServerDetailV2?id=X – single server full specs."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/v1/server/GetServerDetailV2", params={"id": server_id}, headers=_headers(token))
        r.raise_for_status()
        return r.json().get("data") or {}


async def get_server_ip_address(token: str, server_id: int) -> list[dict]:
    """GET /v1/server/GetIpAddress?id=X – fetch IPv4 and IPv6 network assignments."""
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(f"{BASE}/v1/server/GetIpAddress", params={"id": server_id}, headers=_headers(token))
        r.raise_for_status()
        res = r.json()
        raw_list = res.get("list") or (res.get("data") or {}).get("list") or []
        addrs = []
        for item in raw_list:
            for addr in item.get("address", []):
                addrs.append(addr)
        return addrs


async def update_auto_renew(token: str, server_id: int, auto_renew: bool) -> dict:
    """GET /v1/server/UpdateAutoRenew?serverId=X&renewStatus=1|0."""
    status_str = "1" if auto_renew else "0"
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.get(
            f"{BASE}/v1/server/UpdateAutoRenew",
            params={"serverId": server_id, "renewStatus": status_str},
            headers=_headers(token),
        )
        r.raise_for_status()
        return r.json()


async def make_order(token: str, order_data: list[dict], order_type: str = "1", plan_id: int = 0) -> dict:
    """POST /v1/store/makeOrder – create a store purchase order."""
    payload = {
        "order_data": order_data,
        "order_type": order_type,
    }
    if plan_id:
        payload["order_plan_id"] = plan_id
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(f"{BASE}/v1/store/makeOrder", json=payload, headers=_headers(token))
        r.raise_for_status()
        return r.json()


async def pay_order(token: str, order_no: str, pay_type: str = "balance") -> dict:
    """POST /v1/pay – pay an unpaid order with balance or other method."""
    payload = {"order_no": order_no, "pay_type": pay_type}
    async with httpx.AsyncClient(timeout=TIMEOUT) as c:
        r = await c.post(f"{BASE}/v1/pay", json=payload, headers=_headers(token))
        r.raise_for_status()
        return r.json()
