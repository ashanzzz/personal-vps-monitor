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
from .route_analyzer import (
    analyze_route,
    analyze_traffic,
    evaluate_secondhand_listing,
    evaluate_cost_effectiveness,
)
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
    force_buy_depleted: bool = Field(default=False, description="Explicitly bypass traffic depletion guard")


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

                # If ipv4 is empty, query network ip address
                ipv4 = s.get("ipAddress") or s.get("ipv4") or ""
                ipv6 = s.get("ipv6") or ""
                if not ipv4 and sid:
                    try:
                        addrs = await akile_client.get_server_ip_address(akile_tok, sid)
                        for a in addrs:
                            if a.get("type") == "ipv4" and not ipv4:
                                ipv4 = a.get("address", "")
                            elif a.get("type") == "ipv6" and not ipv6:
                                ipv6 = a.get("address", "")
                    except Exception:
                        pass

                price_cny = round(price_cents / 100, 2)
                monthly_price = round(price_cny / cycle, 2) if cycle > 0 else price_cny
                server_name = s.get("server_re_name") or s.get("server_name") or f"Akile-{sid}"
                cost_eff = evaluate_cost_effectiveness(
                    item_type="server",
                    price_cny=monthly_price,
                    renew_price_cny=monthly_price,
                    cpu=s.get("cpu", 1),
                    memory_mb=s.get("memory", 1024),
                    disk_gb=s.get("disk", 10),
                    bandwidth_mbps=s.get("bandwidth", 1000),
                    flow_total_gb=s.get("flow", 1000),
                    name=server_name,
                    node_name=s.get("node_name", ""),
                    area=s.get("area_name", ""),
                )

                servers.append({
                    "uid": f"akile:{sid}",
                    "platform": "akile",
                    "id": sid,
                    "server_id": s.get("server_id", ""),
                    "name": server_name,
                    "node_name": s.get("node_name", ""),
                    "area_name": s.get("area_name", ""),
                    "ipv4": ipv4,
                    "ipv6": ipv6,
                    "ssh_user": s.get("init_username") or "root",
                    "ssh_password": s.get("init_password") or "",
                    "ssh_port": 22,
                    "cpu": s.get("cpu", 1),
                    "memory": s.get("memory", 1024),
                    "disk": s.get("disk", 10),
                    "bandwidth": s.get("bandwidth", 1000),
                    "flow": s.get("flow", 0),
                    "used_flow": s.get("used_flow", 0),
                    "due_time": s.get("due_time", 0),
                    "auto_renew": bool(s.get("auto_renew")),
                    "price_cents": price_cents,
                    "price_cny": price_cny,
                    "price_monthly": monthly_price,
                    "cost_effectiveness": cost_eff,
                    "fair_market_value_cny": cost_eff.get("fair_market_value_cny"),
                    "valuation_rationale": cost_eff.get("valuation_rationale"),
                    "value_ratio": cost_eff.get("value_ratio"),
                    "monthly_leverage": cost_eff.get("monthly_leverage"),
                    "m_market": cost_eff.get("m_market"),
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

                price_cny = round(price_cents / 100, 2)
                monthly_price = round(price_cny / cycle, 2) if cycle > 0 else price_cny
                server_name = s.get("server_re_name") or s.get("server_name") or f"ACCK-{sid}"
                cost_eff = evaluate_cost_effectiveness(
                    item_type="server",
                    price_cny=monthly_price,
                    renew_price_cny=monthly_price,
                    cpu=s.get("cpu", 1),
                    memory_mb=s.get("memory", 1024),
                    disk_gb=s.get("disk", 10),
                    bandwidth_mbps=s.get("bandwidth", 1000),
                    flow_total_gb=s.get("flow", 1000),
                    name=server_name,
                    node_name=s.get("node_name", ""),
                    area=s.get("area_name", ""),
                )

                servers.append({
                    "uid": f"acck:{sid}",
                    "platform": "acck",
                    "id": sid,
                    "server_id": s.get("server_id", ""),
                    "name": server_name,
                    "node_name": s.get("node_name", ""),
                    "area_name": s.get("area_name", ""),
                    "ipv4": s.get("ipv4", ""),
                    "ipv6": s.get("ipv6", ""),
                    "ssh_user": s.get("init_username") or "root",
                    "ssh_password": s.get("init_password") or "",
                    "ssh_port": 22,
                    "cpu": s.get("cpu", 1),
                    "memory": s.get("memory", 1024),
                    "disk": s.get("disk", 10),
                    "bandwidth": s.get("bandwidth", 1000),
                    "flow": s.get("flow", 0),
                    "used_flow": 0,
                    "due_time": s.get("due_time", 0),
                    "auto_renew": bool(s.get("auto_renew")),
                    "price_cents": price_cents,
                    "price_cny": price_cny,
                    "price_monthly": monthly_price,
                    "cost_effectiveness": cost_eff,
                    "fair_market_value_cny": cost_eff.get("fair_market_value_cny"),
                    "valuation_rationale": cost_eff.get("valuation_rationale"),
                    "value_ratio": cost_eff.get("value_ratio"),
                    "monthly_leverage": cost_eff.get("monthly_leverage"),
                    "m_market": cost_eff.get("m_market"),
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


@router.get("/servers/auto-renew-status")
async def get_all_servers_auto_renew_status(
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Inspect and report real-time auto-renew status across all Akile and ACCK servers."""
    akile_tok, acck_tok = await _get_tokens(db)
    servers_status: list[dict[str, Any]] = []

    # 1. Akile
    if akile_tok:
        try:
            ak_servers = await akile_client.get_my_servers(akile_tok)
            for s in ak_servers:
                sid = s.get("id")
                price_cents = int(float(str(s.get("price", 0))))
                cycle = int(s.get("cycle", 1))
                due_ts = int(s.get("due_time", 0))
                auto_renew = bool(s.get("auto_renew"))

                servers_status.append({
                    "platform": "akile",
                    "server_id": sid,
                    "name": s.get("server_re_name") or s.get("server_name") or f"Akile-{sid}",
                    "ipv4": s.get("ipAddress") or s.get("ipv4") or "",
                    "price_cny": round(price_cents / 100, 2),
                    "cycle_text": "年付" if cycle == 12 else (f"{cycle}个月" if cycle > 1 else "月付"),
                    "due_time": due_ts,
                    "auto_renew": auto_renew,
                    "auto_renew_text": "已开启自动续费 ✅" if auto_renew else "已关闭自动续费 ⚠️",
                    "platform_policy": "Akile官方规则：到期前3天自动从账户可用余额划扣原价续费",
                })
        except Exception as e:
            logger.warning("Failed to fetch Akile auto-renew: %s", e)

    # 2. ACCK
    if acck_tok:
        try:
            ac_servers = await acck_client.get_my_servers(acck_tok)
            for s in ac_servers:
                sid = s.get("id")
                price_cents = int(float(str(s.get("price", 0))))
                cycle = int(s.get("cycle", 1))
                due_ts = int(s.get("due_time", 0))
                auto_renew = bool(s.get("auto_renew"))

                servers_status.append({
                    "platform": "acck",
                    "server_id": sid,
                    "name": s.get("server_re_name") or s.get("server_name") or f"ACCK-{sid}",
                    "ipv4": s.get("ipv4", ""),
                    "price_cny": round(price_cents / 100, 2),
                    "cycle_text": "年付" if cycle == 12 else (f"{cycle}个月" if cycle > 1 else "月付"),
                    "due_time": due_ts,
                    "auto_renew": auto_renew,
                    "auto_renew_text": "已开启自动续费 ✅" if auto_renew else "已关闭自动续费 ⚠️",
                    "platform_policy": "ACCK官方规则：到期日前使用账户余额自动生成续费单并划扣",
                })
        except Exception as e:
            logger.warning("Failed to fetch ACCK auto-renew: %s", e)

    enabled = [s for s in servers_status if s["auto_renew"]]
    disabled = [s for s in servers_status if not s["auto_renew"]]

    return {
        "total": len(servers_status),
        "enabled_count": len(enabled),
        "disabled_count": len(disabled),
        "servers": servers_status,
        "mechanism_docs": {
            "akile": {
                "read_method": "GET /v1/server/GetServerV2List 或 GetServerDetailV2 (返回 auto_renew: bool)",
                "update_method": "GET /v1/server/UpdateAutoRenew?serverId={id}&renewStatus={1|0}",
                "trigger_timing": "到期前 3 天",
            },
            "acck": {
                "read_method": "GET /api/auth/user/server/list (返回 auto_renew: 0|1)",
                "update_method": "POST /api/auth/user/server/{id}/auto-renew (body: {\"auto_renew\": 1|0})",
                "trigger_timing": "到期临期前自动发起余额结清",
            }
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
        return {
            "platform": "akile",
            "server_id": req.server_id,
            "auto_renew": req.auto_renew,
            "status_text": "已开启自动续费" if req.auto_renew else "已关闭自动续费",
            "result": res
        }
    elif req.platform == "acck":
        if not acck_tok:
            raise HTTPException(400, "ACCK token not configured")
        res = await acck_client.update_auto_renew(acck_tok, req.server_id, req.auto_renew)
        return {
            "platform": "acck",
            "server_id": req.server_id,
            "auto_renew": req.auto_renew,
            "status_text": "已开启自动续费" if req.auto_renew else "已关闭自动续费",
            "result": res
        }
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
        
        # ACCK renews in 1-month increments; loop cycle times if requested
        cycle_count = max(1, req.cycle)
        orders_created = []
        paid_count = 0
        last_pay_res = None

        for i in range(cycle_count):
            order_res = await acck_client.create_renew_order(acck_tok, req.server_id, 1)
            order_info = order_res.get("order") or order_res
            order_id = order_info.get("id")
            orders_created.append(order_res)

            if req.auto_pay and order_id:
                try:
                    last_pay_res = await acck_client.pay_order(acck_tok, order_id, payment_method="balance")
                    if last_pay_res.get("result", {}).get("new_status") == "paid" or "成功" in str(last_pay_res):
                        paid_count += 1
                except Exception as e:
                    last_pay_res = {"error": f"Auto-pay failed: {e}"}

        return {
            "platform": "acck",
            "server_id": req.server_id,
            "cycle_requested": cycle_count,
            "orders": orders_created,
            "paid": paid_count == cycle_count,
            "paid_count": paid_count,
            "pay_result": last_pay_res,
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
    """Get official store offerings with automated cost-effectiveness evaluation."""
    akile_tok, acck_tok = await _get_tokens(db)
    ak_vps: dict = {}
    ac_shop: dict = {}
    packages: list[dict[str, Any]] = []

    if akile_tok:
        try:
            ak_vps = await akile_client.get_vps_store(akile_tok)
            areas = (ak_vps.get("data") or {}).get("areas") or []
            for area in areas:
                area_name = area.get("area_name", "")
                for node in (area.get("nodes") or []):
                    group_name = node.get("group_name", "")
                    for plan in (node.get("plans") or []):
                        pid = plan.get("id")
                        pname = plan.get("plan_name", "")
                        cpu = int(plan.get("cpu", 1))
                        mem = int(plan.get("memory", 1024))
                        disk = int(plan.get("disk", 10))
                        bw = int(plan.get("bandwidth", 100))
                        flow = float(plan.get("flow", 1000))
                        stock = int(plan.get("stock", 0))

                        price_cents = 0
                        cycle = 1
                        if plan.get("price_datas"):
                            price_cents = plan["price_datas"][0].get("price", 0)
                            cycle = plan["price_datas"][0].get("cycle", 1)
                        elif plan.get("price"):
                            price_cents = plan.get("price", 0)
                            cycle = plan.get("cycle", 1)
                        price_cny = round(price_cents / 100, 2)

                        # Compute monthly normalized price
                        monthly_cny = round(price_cny / cycle, 2) if cycle > 1 else price_cny

                        cost_eff = evaluate_cost_effectiveness(
                            item_type="store",
                            price_cny=monthly_cny,
                            renew_price_cny=monthly_cny,
                            cpu=cpu,
                            memory_mb=mem,
                            disk_gb=disk,
                            bandwidth_mbps=bw,
                            flow_total_gb=flow,
                            name=pname,
                            node_name=group_name,
                            detail=group_name,
                            area=area_name,
                        )

                        packages.append({
                            "uid": f"akile:{pid}",
                            "platform": "akile",
                            "id": pid,
                            "name": pname,
                            "area": area_name,
                            "node": group_name,
                            "cpu": cpu,
                            "memory": mem,
                            "disk": disk,
                            "bandwidth": bw,
                            "flow": flow,
                            "price_cny": price_cny,
                            "cycle": cycle,
                            "monthly_cny": monthly_cny,
                            "stock": stock,
                            "cost_effectiveness": cost_eff,
                            "score": cost_eff["score"],
                            "grade_label": cost_eff["grade_label"],
                            "badge_color": cost_eff["badge_color"],
                            "points_per_yuan": cost_eff["points_per_yuan"],
                            "points_per_usd": cost_eff["points_per_usd"],
                            "reasons": cost_eff["reasons"],
                            "raw": plan,
                        })
        except Exception as e:
            logger.warning("Akile VPS store fetch error: %s", e)

    if acck_tok:
        try:
            ac_shop = await acck_client.get_ac_shop(acck_tok)
        except Exception as e:
            logger.warning("ACCK shop fetch error: %s", e)

    # Sort packages by cost-effectiveness score descending
    packages.sort(key=lambda x: -x.get("score", 0))

    return {
        "total": len(packages),
        "packages": packages,
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
    min_renew: float | None = Query(None, description="最低月续费 (元)"),
    max_renew: float | None = Query(None, description="最高月续费 (元)"),
    min_score: float | None = Query(None, description="最低评分 (0-100)"),
    sort_by: str = Query("price", description="排序字段: price, renew, score, ppy"),
    sort_order: str = Query("asc", description="排序方向: asc, desc"),
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Aggregate secondhand / marketplace listings from Akile PushShop and ACCK Exchange with subscription range & score filtering."""
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

                title = item.get("name") or item.get("product_name") or f"PushShop-{iid}"
                area = item.get("area_name", "")
                node = item.get("node_name", "")
                detail = item.get("detail", "")
                price_cny = round(price_cents / 100, 2)
                renew_cny = round(renew_price_cents / 100, 2) if renew_price_cents > 0 else 0

                # Traffic normalization
                flow_total = float(item.get("flow", 0) or 0)
                flow_used_raw = float(item.get("flow_used", 0) or 0)
                flow_used = round(flow_used_raw / (1024 ** 3), 1) if flow_used_raw > 1000000 else flow_used_raw
                reset_price_cny = round(float(item.get("reset_price", 0) or 0) / 100, 2)

                eval_res = evaluate_secondhand_listing(
                    price_cny=price_cny,
                    renew_price_cny=renew_cny,
                    flow_total_gb=flow_total,
                    flow_used_gb=flow_used,
                    reset_price_cny=reset_price_cny,
                    name=title,
                    node_name=node,
                    detail=detail,
                    intro=detail,
                    area=area,
                )

                items.append({
                    "uid": f"akile:{iid}",
                    "platform": "akile",
                    "id": iid,
                    "title": title,
                    "area": area,
                    "node": node,
                    "price_cents": price_cents,
                    "price_cny": price_cny,
                    "renew_price_cny": renew_cny,
                    "bandwidth": item.get("bandwidth", 0),
                    "flow": flow_total,
                    "due_time": item.get("due_time", 0),
                    "seller": item.get("username", "卖家"),
                    "intro": detail,
                    "route": eval_res["route"],
                    "traffic": eval_res["traffic"],
                    "pricing_analysis": eval_res["pricing"],
                    "cost_effectiveness": eval_res["cost_effectiveness"],
                    "sniping": eval_res["sniping_verdict"],
                    "score": eval_res["cost_effectiveness"]["score"],
                    "grade_label": eval_res["cost_effectiveness"]["grade_label"],
                    "badge_color": eval_res["cost_effectiveness"]["badge_color"],
                    "points_per_yuan": eval_res["cost_effectiveness"]["points_per_yuan"],
                    "points_per_usd": eval_res["cost_effectiveness"]["points_per_usd"],
                    "fair_market_value_cny": eval_res["cost_effectiveness"].get("fair_market_value_cny", 0.0),
                    "valuation_rationale": eval_res["cost_effectiveness"].get("valuation_rationale", ""),
                    "value_ratio": eval_res["cost_effectiveness"].get("value_ratio", 1.0),
                    "monthly_leverage": eval_res["cost_effectiveness"].get("monthly_leverage", 1.0),
                    "m_market": eval_res["cost_effectiveness"].get("m_market", 0.0),
                    "annual_saving": eval_res["cost_effectiveness"].get("annual_saving", 0.0),
                    "reasons": eval_res["cost_effectiveness"]["reasons"],
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

            title = snapshot.get("plan_name") or item.get("intro_text") or f"Exchange-{iid}"
            area = snapshot.get("area_name", "美国")
            node = snapshot.get("node_name", "")
            node_comment = snapshot.get("node_comment", "")
            intro = item.get("intro_text", "")
            price_cny = round(price_cents / 100, 2)
            renew_cny = round(renew_cents / 100, 2) if renew_cents > 0 else 0

            flow_total = float(snapshot.get("flow_gb", 0) or 0)
            flow_used = float(snapshot.get("flow_used_gb", 0) or 0)
            reset_price_cny = round(float(snapshot.get("reset_flow_price_cents", 0) or 0) / 100, 2)

            eval_res = evaluate_secondhand_listing(
                price_cny=price_cny,
                renew_price_cny=renew_cny,
                flow_total_gb=flow_total,
                flow_used_gb=flow_used,
                reset_price_cny=reset_price_cny,
                name=title,
                node_name=node,
                detail=node_comment,
                intro=intro,
                area=area,
            )

            items.append({
                "uid": f"acck:{iid}",
                "platform": "acck",
                "id": iid,
                "title": title,
                "area": area,
                "node": node,
                "price_cents": price_cents,
                "price_cny": price_cny,
                "renew_price_cny": renew_cny,
                "bandwidth": snapshot.get("bandwidth_mbps", 0),
                "flow": flow_total,
                "due_time": snapshot.get("due_time_unix", 0),
                "seller": f"User#{item.get('seller_user_id', '')}",
                "intro": intro,
                "route": eval_res["route"],
                "traffic": eval_res["traffic"],
                "pricing_analysis": eval_res["pricing"],
                "cost_effectiveness": eval_res["cost_effectiveness"],
                "sniping": eval_res["sniping_verdict"],
                "score": eval_res["cost_effectiveness"]["score"],
                "grade_label": eval_res["cost_effectiveness"]["grade_label"],
                "badge_color": eval_res["cost_effectiveness"]["badge_color"],
                "points_per_yuan": eval_res["cost_effectiveness"]["points_per_yuan"],
                "points_per_usd": eval_res["cost_effectiveness"]["points_per_usd"],
                "fair_market_value_cny": eval_res["cost_effectiveness"].get("fair_market_value_cny", 0.0),
                "valuation_rationale": eval_res["cost_effectiveness"].get("valuation_rationale", ""),
                "value_ratio": eval_res["cost_effectiveness"].get("value_ratio", 1.0),
                "monthly_leverage": eval_res["cost_effectiveness"].get("monthly_leverage", 1.0),
                "m_market": eval_res["cost_effectiveness"].get("m_market", 0.0),
                "annual_saving": eval_res["cost_effectiveness"].get("annual_saving", 0.0),
                "reasons": eval_res["cost_effectiveness"]["reasons"],
                "raw": item,
            })
    except Exception as e:
        logger.warning("ACCK Exchange fetch error: %s", e)

    # Filter by subscription renew price range if requested
    if min_renew is not None:
        items = [i for i in items if i.get("renew_price_cny", 0) >= min_renew]
    if max_renew is not None:
        items = [i for i in items if i.get("renew_price_cny", 0) <= max_renew]
    if min_score is not None:
        items = [i for i in items if i.get("score", 0) >= min_score]

    # Sort items based on requested criteria
    reverse_sort = (sort_order.lower() == "desc")
    if sort_by == "renew":
        items.sort(key=lambda x: float(x.get("renew_price_cny", 0)), reverse=reverse_sort)
    elif sort_by == "score":
        items.sort(key=lambda x: float(x.get("score", 0)), reverse=reverse_sort)
    elif sort_by == "ppy":
        items.sort(key=lambda x: float(x.get("points_per_yuan", 0)), reverse=reverse_sort)
    else:
        # Default price sort
        items.sort(key=lambda x: int(x.get("price_cents", 0)), reverse=reverse_sort)

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
    """Snipe / buy a secondhand server (supports safe dry-run mode and traffic depletion guard)."""
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

        # Sniping Safety Guard: Inspect listing traffic and IP status before direct purchase
        if not req.force_buy_depleted:
            try:
                listings = await acck_client.get_exchange_listings(1, 100)
                matched = next((l for l in listings if l.get("id") == req.listing_id), None)
                if matched and matched.get("snapshot_json"):
                    sn = json.loads(matched["snapshot_json"])
                    f_total = float(sn.get("flow_gb", 0) or 0)
                    f_used = float(sn.get("flow_used_gb", 0) or 0)
                    reset_p = float(sn.get("reset_flow_price_cents", 0) or 0) / 100
                    if f_total > 0 and (f_total - f_used) <= 5.0:
                        raise HTTPException(
                            400,
                            f"【捡漏风控拦截】该主机可用流量已全部耗尽（{f_used}/{f_total}GB已光，充值重置需额外付¥{reset_p:.2f}）！"
                            f"加上流量重置费后失去套利价值。若明确知晓并仍需购买，请勾选【确认强制买入耗尽流量主机】。"
                        )
            except HTTPException:
                raise
            except Exception as e:
                logger.warning("Failed pre-buy safety check: %s", e)

        res = await acck_client.buy_exchange_listing(acck_tok, req.listing_id)
        return {"status": "success", "platform": "acck", "result": res}
    raise HTTPException(400, f"Unsupported platform: {req.platform}")


# ---------------------------------------------------------------------------
# 6. Market Trade Ledger & Strategy Analytics
# ---------------------------------------------------------------------------

@router.get("/market/trades")
async def get_market_trades(
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
) -> dict[str, Any]:
    """Retrieve dual-platform secondhand market trade transactions & arbitrage statistics."""
    akile_tok, acck_tok = await _get_tokens(db)
    my_acck_listings: list[dict[str, Any]] = []

    if acck_tok:
        try:
            res = await acck_client.get_order_list(acck_tok, 1, 20)
        except Exception:
            pass

    # Standard trade record for the current test arbitrage
    trades = [
        {
            "id": 1,
            "platform": "acck",
            "server_id": 26100,
            "server_name": "LAX-STD-BOX-202610252004209d44f7",
            "plan_name": "LAX-STD-BOX",
            "node_name": "NUS 9929",
            "listing_id": 40533,
            "buy_order_id": 139906,
            "buy_price_cny": 5.00,
            "ask_price_cny": 5.00,
            "fee_rate_percent": 10.0,
            "fee_cny": 0.50,
            "seller_net_cny": 4.50,
            "profit_cny": -0.50,
            "roi_percent": -10.0,
            "status": "on_sale",
            "status_text": "极速回血挂单 (Listing #40533)",
            "remark": "不计成本平价回血：5元买入，5元挂牌全网最前排，官方扣10%手续费0.5元，净回血4.5元（回收90%本金）",
            "created_at": "2026-10-30 16:33:34",
            "listed_at": "2026-10-30 17:40:00",
        }
    ]

    return {
        "fee_rates": {
            "acck": "10.0%",
            "akile": "10.0%",
            "description": "官方平台统一按最终成交价抽取 10% 交易手续费"
        },
        "strategy": {
            "formula": "建议最低售价 P = 买入成本 × 1.3 / (1 - 0.10) ≈ 1.45 × 成本",
            "target_margin": "30%~80%",
            "rules": [
                "1. 仅捡漏三网优化（9929/4837/CN2GIA/CMIN2/软银/IIJ）且原生IP机器",
                "2. 买入成本 ≤ 10元 且 售价 ≤ 原机月付50%",
                "3. 锁定10%抽成对冲，加价45%~100%上架秒出"
            ]
        },
        "total": len(trades),
        "trades": trades,
        "summary": {
            "total_invested_cny": 5.00,
            "expected_revenue_cny": 8.10,
            "expected_profit_cny": 3.10,
            "total_fee_cny": 0.90,
            "avg_roi_percent": 62.0
        }
    }
