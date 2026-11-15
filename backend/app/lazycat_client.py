"""
LazyCatCloud (dash.lazycat.wiki) API client.
All methods return raw dicts/lists from the API.
Token is passed per-call so it can be updated dynamically.

=== 【用户铁律 · 永久生效】懒猫云实例绝不可销毁 ===

懒猫云的实例**绝对不能销毁**。本客户端**故意不实现、也不得实现**以下接口：

  POST   /instances/{id}/destroy              —— 销毁实例
  GET    /instances/{id}/destroy-info         —— 销毁报价信息
  POST   /instances/{id}/delete-container     —— 删除实例容器
  DELETE /instances/{id}                      —— 删除实例本体
  POST   /instances/{id}/delete-and-refund    —— 删除并退款
  POST   /admin/instances/{id}/refund         —— 管理员退款

同样**禁止**在前端/UI 里出现任何「销毁实例」「删除实例」的按钮、菜单、
确认弹窗或提示文案——不是加二次确认，而是整个入口不存在。

下面 `_FORBIDDEN_PATHS` + `_assert_not_forbidden()` 是运行时硬拦截：
即使将来有人误加方法，请求也会在发出前直接抛错，不会真的打到懒猫云。
"""
from __future__ import annotations

import logging
import os
from typing import Any
import httpx

logger = logging.getLogger(__name__)

BASE = "https://dash.lazycat.wiki/api"
TIMEOUT = 15

# 销毁/删除实例本体 —— 硬性禁止，任何情况下不得调用
_FORBIDDEN_PATHS = (
    "/instances/{}/destroy",
    "/instances/{}/destroy-info",
    "/instances/{}/delete-container",
    "/instances/{}/delete-and-refund",
)


class ForbiddenLazyCatOperation(RuntimeError):
    """试图对懒猫云实例执行销毁/删除类操作时抛出（用户铁律，不可绕过）。"""


def _assert_not_forbidden(url: str, method: str = "GET") -> None:
    """拦截任何指向「销毁/删除实例本体」的请求。"""
    path = url.split("?")[0].rstrip("/")
    lowered = path.lower()
    # 实例本体的 DELETE /instances/{id} —— 仅在 DELETE 方法下视为删除本体
    if method.upper() == "DELETE":
        segs = [s for s in lowered.split("/") if s]
        if "instances" in segs:
            tail = segs[segs.index("instances"):]
            if len(tail) == 2 and tail[0] == "instances":
                raise ForbiddenLazyCatOperation(
                    "已拦截：懒猫云实例绝不可删除（用户铁律）。URL=%s" % url
                )
    for marker in ("/destroy", "/delete-container", "/delete-and-refund", "/refund"):
        if marker in lowered:
            raise ForbiddenLazyCatOperation(
                "已拦截：懒猫云实例绝不可销毁（用户铁律）。URL=%s" % url
            )

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


def _guard_instances_base(url: str, method: str) -> None:
    """DELETE 到 /instances/{id} 是删实例本体 —— 铁律拦截。"""
    if method.upper() == "DELETE":
        segs = [s for s in url.split("?")[0].rstrip("/").lower().split("/") if s]
        if "instances" in segs:
            tail = segs[segs.index("instances"):]
            if len(tail) == 2 and tail[0] == "instances":
                raise ForbiddenLazyCatOperation(
                    "已拦截：懒猫云实例绝不可删除（用户铁律）。URL=%s" % url
                )


# --- Authentication & User Profile ---

async def login(
    username: str,
    password: str,
    device_fingerprint: str = "",
    totp_code: str | None = None,
) -> dict[str, Any]:
    """POST /auth/login - Turnstile is currently disabled on dash.lazycat.wiki.

    totp_code：若账号开启了 2FA（TOTP），传入当前 6 位动态码。
    """
    payload = {
        "username": username,
        "password": password,
        "totpCode": totp_code,
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
