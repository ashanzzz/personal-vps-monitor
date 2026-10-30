"""
Unified API router for AkileCloud & ACCK dual-platform management:
- Unified Servers (My Servers, Auto-Renew, Renew Orders)
- Unified Billing & Payments (Order history, money logs, balance pay)
- Unified Store & New Purchase (Official store, specs, safe buy / dry-run)
- Unified Secondhand Market (Akile PushShop & ACCK Exchange, snipe & buy)
- Dual-Rate Refresh Control (Slow 30s background vs. timed high-speed burst 1~3s)
"""
from __future__ import annotations

import logging
import json
import time
from typing import Any
from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel, Field
from sqlalchemy.ext.asyncio import AsyncSession

from .auth import require_auth
from .config import get_all_settings
from .database import get_db
from . import akile_client, acck_client
from .refresh_manager import refresh_mgr
from .ws import broadcast

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/unified", tags=["unified"])


# ---------------------------------------------------------------------------
# Models
# ---------------------------------------------------------------------------

class BurstRefreshRequest(BaseModel):
    duration: int = Field(default=300, ge=10, le=3600, description="Duration in seconds (default 5 min)")
    interval: int = Field(default=2, ge=1, le=60, description="Polling interval in seconds")


class UpdateAutoRenewRequest(BaseModel):
    platform: str = Field(..., description="'akile' or 'acck'")
    server_id: int = Field(...)
    auto_renew: bool = Field(...)


class RenewServerRequest(BaseModel):
    platform: str = Field(..., description="'akile' or 'acck'")
    server_id: int = Field(...)
    cycle: int = Field(default=1, ge=1)
    auto_pay: bool = Field(default=False, description="Automatically pay with balance if order created")


class PayOrderRequest(BaseModel):
    platform: str = Field(..., description="'akile' or 'acck'")
    order_id: int | str | None = None
    order_no: str | None = None
    method: str = Field(default="balance")


class StoreBuyRequest(BaseModel):
    platform: str = Field(..., description="'akile' or 'acck'")
    plan_id: int | str = Field(...)
    order_data: list[dict[str, Any]] = Field(default_factory=list)
    order_type: str = Field(default="1")
    safe_mode: bool = Field(default=True, description="When true, simulate only without payment")


class SecondhandBuyRequest(BaseModel):
    platform: str = Field(..., description="'akile' or 'acck'")
    listing_id: int = Field(...)
    price_cents: int = Field(default=0)
    captcha: str = Field(default="")
    safe_mode: bool = Field(default=True, description="When true, simulate only without payment")


# ---------------------------------------------------------------------------
# Helper to resolve tokens
# ---------------------------------------------------------------------------

async def _get_tokens(db: AsyncSession) -> tuple[str, str]:
    settings = await get_all_settings(db)
    akile_tok = settings.get("akile_token") or ""
    acck_tok = settings.get("acck_token") or ""
    return akile_tok, acck_tok


# ---------------------------------------------------------------------------
# 1. Dual-Rate Refresh Control Endpoints
# ---------------------------------------------------------------------------

@router.get("/refresh/status")
async def get_refresh_status(_: str = Depends(require_auth)) -> dict[str, Any]:
    """Get current dual-rate refresh mode and countdown."""
    return refresh_mgr.get_state()


@router.post("/refresh/burst")
async def start_burst_refresh(
    req: BurstRefreshRequest,
    _: str = Depends(require_auth),
) -> dict[str, Any]:
    """Activate high-speed burst/sniper polling with countdown."""
    state = refresh_mgr.start_burst(duration=req.duration, interval=req.interval)
    await broadcast({
        "type": "refresh_mode_change",
        "category": "dual_rate_refresh",
        "data": state,
    })
    return state


@router.post("/refresh/slow")
async def stop_burst_refresh(_: str = Depends(require_auth)) -> dict[str, Any]:
    """Revert immediately to standard slow background refresh."""
    state = refresh_mgr.stop_burst()
    await broadcast({
        "type": "refresh_mode_change",
        "category": "dual_rate_refresh",
        "data": state,
    })
    return state


# ---------------------------------------------------------------------------
# 2. Unified Servers (My Servers, Auto-Renew, Renew)
# ---------------------------------------------------------------------------

@router.get("/servers")
async def get_unified_servers(
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Fetch and normalize instances from both AkileCloud and ACCK."""
    akile_tok, acck_tok = await _get_tokens(db)
    servers: list[dict[str, Any]] = []
    errors: dict[str, str] = {}

    # 1. Akile servers
    if akile_tok:
        try:
            ak_list = await akile_client.get_my_servers(akile_tok)
            for s in ak_list:
                sid = s.get("id")
                try:
                    price_cents = int(float(str(s.get("price", 0))))
                except (ValueError, TypeError):
                    price_cents = 0
                cycle = int(s.get("cycle", 1))
                cycle_text = "年付" if cycle == 12 else (f"{cycle}个月" if cycle > 1 else "月付")

                servers.append({
                    "uid": f"akile:{sid}",
                    "platform": "akile",
                    "id": sid,
                    "server_id": s.get("server_id", ""),
                    "name": s.get("server_re_name") or s.get("server_name") or f"Akile-{sid}",
                    "node_name": s.get("node_name", ""),
                    "area_name": s.get("area_name", ""),
                    "ipv4": s.get("ipAddress") or s.get("ipv4") or "",
                    "ipv6": s.get("ipv6", ""),
                    "cpu": s.get("cpu", 1),
                    "memory": s.get("memory", 1024),
                    "disk": s.get("disk", 10),
                    "bandwidth": s.get("bandwidth", 1000),
                    "flow": s.get("flow", 0),
                    "used_flow": s.get("used_flow", 0),
                    "due_time": s.get("due_time", 0),
                    "auto_renew": bool(s.get("auto_renew")),
                    "price_cents": price_cents,
                    "price_cny": round(price_cents / 100, 2),
                    "cycle": cycle,
                    "cycle_text": cycle_text,
                    "status": s.get("status", 1),
                    "system_name": s.get("system_name", ""),
                    "raw": s,
                })
        except Exception as e:
            logger.warning("Failed to fetch Akile servers: %s", e)
            errors["akile"] = str(e)

    # 2. ACCK servers
    if acck_tok:
        try:
            ac_list = await acck_client.get_my_servers(acck_tok)
            for s in ac_list:
                sid = s.get("id")
                try:
                    price_cents = int(float(str(s.get("price", 0))))
                except (ValueError, TypeError):
                    price_cents = 0
                cycle = int(s.get("cycle", 1))
                cycle_text = "年付" if cycle == 12 else (f"{cycle}个月" if cycle > 1 else "月付")

                servers.append({
                    "uid": f"acck:{sid}",
                    "platform": "acck",
                    "id": sid,
                    "server_id": s.get("server_id", ""),
                    "name": s.get("server_re_name") or s.get("server_name") or f"ACCK-{sid}",
                    "node_name": s.get("node_name", ""),
                    "area_name": s.get("area_name", ""),
                    "ipv4": s.get("ipv4", ""),
                    "ipv6": s.get("ipv6", ""),
                    "cpu": s.get("cpu", 1),
                    "memory": s.get("memory", 1024),
                    "disk": s.get("disk", 10),
                    "bandwidth": s.get("bandwidth", 1000),
                    "flow": s.get("flow", 0),
                    "used_flow": 0,
                    "due_time": s.get("due_time", 0),
                    "auto_renew": bool(s.get("auto_renew")),
                    "price_cents": price_cents,
                    "price_cny": round(price_cents / 100, 2),
                    "cycle": cycle,
                    "cycle_text": cycle_text,
                    "status": s.get("status", 1),
                    "system_name": "",
                    "raw": s,
                })
        except Exception as e:
            logger.warning("Failed to fetch ACCK servers: %s", e)
            errors["acck"] = str(e)

    return {
        "total": len(servers),
        "servers": servers,
        "errors": errors,
        "counts": {
            "akile": len([s for s in servers if s["platform"] == "akile"]),
            "acck": len([s for s in servers if s["platform"] == "acck"]),
        }
    }


@router.post("/servers/auto-renew")
async def update_server_auto_renew(
    req: UpdateAutoRenewRequest,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Toggle auto-renew for a server on either Akile or ACCK."""
    akile_tok, acck_tok = await _get_tokens(db)
    if req.platform == "akile":
        if not akile_tok:
            raise HTTPException(400, "Akile token not configured")
        res = await akile_client.update_auto_renew(akile_tok, req.server_id, req.auto_renew)
        return {"platform": "akile", "server_id": req.server_id, "auto_renew": req.auto_renew, "result": res}
    elif req.platform == "acck":
        if not acck_tok:
            raise HTTPException(400, "ACCK token not configured")
        res = await acck_client.update_auto_renew(acck_tok, req.server_id, req.auto_renew)
        return {"platform": "acck", "server_id": req.server_id, "auto_renew": req.auto_renew, "result": res}
    raise HTTPException(400, f"Unsupported platform: {req.platform}")


@router.post("/servers/renew")
async def renew_server(
    req: RenewServerRequest,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Create renewal order and optionally execute balance payment."""
    akile_tok, acck_tok = await _get_tokens(db)
    if req.platform == "acck":
        if not acck_tok:
            raise HTTPException(400, "ACCK token not configured")
        # 1. Create renew order
        order_res = await acck_client.create_renew_order(acck_tok, req.server_id, req.cycle)
        order_info = order_res.get("order") or order_res
        order_id = order_info.get("id")

        pay_res = None
        if req.auto_pay and order_id:
            try:
                pay_res = await acck_client.pay_order(acck_tok, order_id, payment_method="balance")
            except Exception as e:
                pay_res = {"error": f"Auto-pay failed: {e}"}

        return {
            "platform": "acck",
            "server_id": req.server_id,
            "order": order_res,
            "paid": pay_res is not None and "error" not in pay_res,
            "pay_result": pay_res,
        }
    elif req.platform == "akile":
        if not akile_tok:
            raise HTTPException(400, "Akile token not configured")
        # For Akile, auto_renew toggle ensures next billing cycle, or probe billing order
        return {
            "platform": "akile",
            "server_id": req.server_id,
            "msg": "Akile 服务器已配置续费设置。若有待续费账单可在【账单中心】一键支付。",
        }
    raise HTTPException(400, f"Unsupported platform: {req.platform}")


# ---------------------------------------------------------------------------
# 3. Unified Billing & Order History
# ---------------------------------------------------------------------------

@router.get("/billing")
async def get_unified_billing(
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Fetch orders, pending invoices, and money transaction logs from both platforms."""
    akile_tok, acck_tok = await _get_tokens(db)
    ak_orders: list[dict] = []
    ak_money_logs: list[dict] = []
    ak_billing_orders: list[dict] = []
    ac_orders: list[dict] = []
    ac_shop_logs: list[dict] = []

    if akile_tok:
        try:
            ord_res = await akile_client.get_order_list(akile_tok, page, page_size)
            ak_orders = ord_res.get("list") or []
        except Exception as e:
            logger.warning("Akile orders fetch error: %s", e)
        try:
            money_res = await akile_client.get_money_logs(akile_tok, page, page_size)
            raw_money = money_res.get("list") or []
            for m in raw_money:
                amt = m.get("amount") if "amount" in m else m.get("money", 0)
                try:
                    amt_int = int(float(str(amt)))
                except (ValueError, TypeError):
                    amt_int = 0
                ak_money_logs.append({
                    "id": m.get("id"),
                    "money": amt_int,
                    "money_cny": round(abs(amt_int) / 100, 2),
                    "is_positive": amt_int >= 0,
                    "remark": m.get("reason") or m.get("remark") or m.get("name") or "余额变动",
                    "order_no": m.get("order_no", ""),
                    "created_at": m.get("created_at", ""),
                    "raw": m,
                })
        except Exception as e:
            logger.warning("Akile money log error: %s", e)
        try:
            b_ord = await akile_client.get_billing_orders(akile_tok)
            ak_billing_orders = b_ord.get("list") or []
        except Exception as e:
            logger.warning("Akile billing orders error: %s", e)

    if acck_tok:
        try:
            ac_res = await acck_client.get_order_list(acck_tok, page, page_size)
            ac_orders = ac_res.get("orders") or []
        except Exception as e:
            logger.warning("ACCK orders fetch error: %s", e)
        try:
            ac_shop_logs = await acck_client.get_shop_logs(acck_tok, page, page_size)
        except Exception as e:
            logger.warning("ACCK shop logs error: %s", e)

    return {
        "akile": {
            "orders": ak_orders,
            "money_logs": ak_money_logs,
            "pending_invoices": ak_billing_orders,
        },
        "acck": {
            "orders": ac_orders,
            "shop_logs": ac_shop_logs,
        },
    }


@router.post("/billing/pay")
async def pay_unified_order(
    req: PayOrderRequest,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Execute order payment via balance."""
    akile_tok, acck_tok = await _get_tokens(db)
    if req.platform == "akile":
        if not akile_tok:
            raise HTTPException(400, "Akile token not configured")
        if not req.order_no:
            raise HTTPException(400, "order_no required for Akile payment")
        res = await akile_client.pay_order(akile_tok, req.order_no, pay_type=req.method)
        return {"platform": "akile", "order_no": req.order_no, "result": res}
    elif req.platform == "acck":
        if not acck_tok:
            raise HTTPException(400, "ACCK token not configured")
        if not req.order_id:
            raise HTTPException(400, "order_id required for ACCK payment")
        res = await acck_client.pay_order(acck_tok, req.order_id, payment_method=req.method)
        return {"platform": "acck", "order_id": req.order_id, "result": res}
    raise HTTPException(400, f"Unsupported platform: {req.platform}")


# ---------------------------------------------------------------------------
# 4. Unified Store & New Purchase (Official Stores)
# ---------------------------------------------------------------------------

@router.get("/store")
async def get_unified_store(
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Get official store offerings from both Akile and ACCK."""
    akile_tok, acck_tok = await _get_tokens(db)
    ak_vps: dict = {}
    ac_shop: dict = {}

    if akile_tok:
        try:
            ak_vps = await akile_client.get_vps_store(akile_tok)
        except Exception as e:
            logger.warning("Akile VPS store fetch error: %s", e)

    if acck_tok:
        try:
            ac_shop = await acck_client.get_ac_shop(acck_tok)
        except Exception as e:
            logger.warning("ACCK shop fetch error: %s", e)

    return {
        "akile_store": ak_vps.get("data") or ak_vps,
        "acck_store": ac_shop,
    }


@router.post("/store/buy")
async def buy_store_package(
    req: StoreBuyRequest,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Purchase a package from official store (supports safe dry-run mode)."""
    settings = await get_all_settings(db)
    is_safe = req.safe_mode or settings.get("safe_buy_mode", True)

    if is_safe:
        return {
            "status": "simulated",
            "protected": True,
            "platform": req.platform,
            "plan_id": req.plan_id,
            "msg": "【防误购安全模式已生效】新购订单试算成功，未发起扣费下单。",
            "order_data": req.order_data,
        }

    akile_tok, acck_tok = await _get_tokens(db)
    if req.platform == "akile":
        if not akile_tok:
            raise HTTPException(400, "Akile token not configured")
        res = await akile_client.make_order(
            akile_tok,
            order_data=req.order_data,
            order_type=req.order_type,
            plan_id=int(req.plan_id) if str(req.plan_id).isdigit() else 0,
        )
        return {"status": "success", "platform": "akile", "result": res}
    elif req.platform == "acck":
        if not acck_tok:
            raise HTTPException(400, "ACCK token not configured")
        payload = {"plan_id": req.plan_id, "data": req.order_data}
        res = await acck_client.create_order(acck_tok, payload)
        return {"status": "success", "platform": "acck", "result": res}
    raise HTTPException(400, f"Unsupported platform: {req.platform}")


# ---------------------------------------------------------------------------
# 5. Unified Secondhand Market (PushShop & Exchange)
# ---------------------------------------------------------------------------

@router.get("/secondhand")
async def get_unified_secondhand(
    page: int = Query(1, ge=1),
    page_size: int = Query(50, ge=1, le=100),
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Aggregate secondhand / marketplace listings from Akile PushShop and ACCK Exchange."""
    akile_tok, acck_tok = await _get_tokens(db)
    items: list[dict[str, Any]] = []

    # 1. Akile PushShop
    if akile_tok:
        try:
            ak_res = await akile_client.get_secondhand_list(akile_tok, page=page, page_size=page_size)
            raw_list = ak_res.get("list") or []
            for item in raw_list:
                iid = item.get("id")
                # price in fen (can be string or int)
                try:
                    price_cents = int(float(str(item.get("price", 0))))
                except (ValueError, TypeError):
                    price_cents = 0
                try:
                    renew_price_cents = int(float(str(item.get("server_price", 0))))
                except (ValueError, TypeError):
                    renew_price_cents = 0

                items.append({
                    "uid": f"akile:{iid}",
                    "platform": "akile",
                    "id": iid,
                    "title": item.get("name") or item.get("product_name") or f"PushShop-{iid}",
                    "area": item.get("area_name", ""),
                    "node": item.get("node_name", ""),
                    "price_cents": price_cents,
                    "price_cny": round(price_cents / 100, 2),
                    "renew_price_cny": round(renew_price_cents / 100, 2) if renew_price_cents > 0 else 0,
                    "bandwidth": item.get("bandwidth", 0),
                    "flow": item.get("traffic") or item.get("flow", 0),
                    "due_time": item.get("due_time", 0),
                    "seller": item.get("username", "卖家"),
                    "intro": item.get("detail", ""),
                    "raw": item,
                })
        except Exception as e:
            logger.warning("Akile PushShop fetch error: %s", e)

    # 2. ACCK Exchange
    try:
        ac_list = await acck_client.get_exchange_listings(page=page, page_size=page_size)
        for item in ac_list:
            iid = item.get("id")
            try:
                price_cents = int(float(str(item.get("ask_price_cents", 0))))
            except (ValueError, TypeError):
                price_cents = 0

            snapshot = {}
            if item.get("snapshot_json"):
                try:
                    snapshot = json.loads(item["snapshot_json"])
                except Exception:
                    pass

            renew_cents = 0
            if snapshot.get("renew_price_cents"):
                try:
                    renew_cents = int(float(str(snapshot["renew_price_cents"])))
                except (ValueError, TypeError):
                    renew_cents = 0

            items.append({
                "uid": f"acck:{iid}",
                "platform": "acck",
                "id": iid,
                "title": snapshot.get("plan_name") or item.get("intro_text") or f"Exchange-{iid}",
                "area": snapshot.get("area_name", "美国"),
                "node": snapshot.get("node_name", ""),
                "price_cents": price_cents,
                "price_cny": round(price_cents / 100, 2),
                "renew_price_cny": round(renew_cents / 100, 2) if renew_cents > 0 else 0,
                "bandwidth": snapshot.get("bandwidth_mbps", 0),
                "flow": snapshot.get("flow_gb", 0),
                "due_time": snapshot.get("due_time_unix", 0),
                "seller": f"User#{item.get('seller_user_id', '')}",
                "intro": item.get("intro_text", ""),
                "raw": item,
            })
    except Exception as e:
        logger.warning("ACCK Exchange fetch error: %s", e)

    # Sort by price ascending
    items.sort(key=lambda x: int(x.get("price_cents", 0)))

    return {
        "total": len(items),
        "items": items,
        "counts": {
            "akile": len([i for i in items if i["platform"] == "akile"]),
            "acck": len([i for i in items if i["platform"] == "acck"]),
        }
    }


@router.post("/secondhand/buy")
async def buy_secondhand_item(
    req: SecondhandBuyRequest,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Snipe / buy a secondhand server (supports safe dry-run mode)."""
    settings = await get_all_settings(db)
    is_safe = req.safe_mode or settings.get("safe_buy_mode", True)

    if is_safe:
        return {
            "status": "simulated",
            "protected": True,
            "platform": req.platform,
            "listing_id": req.listing_id,
            "price_cny": round(req.price_cents / 100, 2),
            "msg": "【防误购安全模式已生效】二手抢购模拟试算成功，未发起真实支付扣款。",
        }

    akile_tok, acck_tok = await _get_tokens(db)
    if req.platform == "akile":
        if not akile_tok:
            raise HTTPException(400, "Akile token not configured")
        res = await akile_client.buy_secondhand(akile_tok, req.listing_id, auto_pay=True)
        return {"status": "success", "platform": "akile", "result": res}
    elif req.platform == "acck":
        if not acck_tok:
            raise HTTPException(400, "ACCK token not configured")
        res = await acck_client.buy_exchange_listing(acck_tok, req.listing_id)
        return {"status": "success", "platform": "acck", "result": res}
    raise HTTPException(400, f"Unsupported platform: {req.platform}")
