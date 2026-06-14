"""
Polling logic: fetch Akile data, diff against DB, emit events.
"""
from __future__ import annotations

import json
import logging
from datetime import datetime
from typing import TYPE_CHECKING

import httpx
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from . import akile_client as api
from .config import get_all_settings
from .database import AlertEvent, MachineSnapshot, PriceHistory, SessionLocal
from .notify import send_telegram
from .ws import broadcast

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _price_monthly(price_datas: list[dict]) -> int:
    for p in price_datas:
        if p.get("cycle") == 1:
            return p["price"]
    return price_datas[0]["price"] if price_datas else 0


def _price_yearly(price_datas: list[dict]) -> int | None:
    for p in price_datas:
        if p.get("cycle") == 12:
            return p["price"]
    return None


# ---------------------------------------------------------------------------
# Snapshot upsert + diff
# ---------------------------------------------------------------------------

async def _upsert(db: AsyncSession, category: str, plan_id: str, fields: dict) -> list[dict]:
    """
    Upsert a MachineSnapshot row and return a list of change-event dicts.
    Each event: {category, plan_id, name, area, event_type, old_value, new_value}
    """
    row = await db.scalar(
        select(MachineSnapshot).where(
            MachineSnapshot.category == category,
            MachineSnapshot.plan_id == plan_id,
        )
    )
    events: list[dict] = []

    if row is None:
        # Brand-new plan → new_listing event
        row = MachineSnapshot(category=category, plan_id=plan_id, **fields)
        db.add(row)
        events.append({
            "category": category, "plan_id": plan_id,
            "name": fields.get("name", ""), "area": fields.get("area", ""),
            "event_type": "new_listing",
            "old_value": None, "new_value": str(fields.get("available")),
        })
    else:
        # Check availability change
        if row.available != fields.get("available"):
            event_type = "restock" if fields.get("available") else "out_of_stock"
            events.append({
                "category": category, "plan_id": plan_id,
                "name": row.name, "area": row.area,
                "event_type": event_type,
                "old_value": str(row.available), "new_value": str(fields.get("available")),
            })

        # Check price change
        new_price = fields.get("price_monthly", 0)
        if row.price_monthly and new_price and row.price_monthly != new_price:
            events.append({
                "category": category, "plan_id": plan_id,
                "name": row.name, "area": row.area,
                "event_type": "price_change",
                "old_value": str(row.price_monthly), "new_value": str(new_price),
            })

        for k, v in fields.items():
            setattr(row, k, v)

    row.updated_at = datetime.utcnow()
    return events


async def _mark_delisted(db: AsyncSession, category: str, seen_ids: set[str]) -> list[dict]:
    """Plans we knew about but didn't see this cycle → delisted."""
    rows = (
        await db.execute(
            select(MachineSnapshot).where(
                MachineSnapshot.category == category,
                MachineSnapshot.available == True,  # noqa: E712
            )
        )
    ).scalars().all()
    events: list[dict] = []
    for row in rows:
        if row.plan_id not in seen_ids:
            row.available = False
            row.updated_at = datetime.utcnow()
            events.append({
                "category": category, "plan_id": row.plan_id,
                "name": row.name, "area": row.area,
                "event_type": "delisted",
                "old_value": "True", "new_value": "False",
            })
    return events


async def _save_events(db: AsyncSession, events: list[dict], settings: dict) -> None:
    for ev in events:
        db.add(AlertEvent(**ev))
    await db.commit()

    # Notify
    if settings.get("tg_bot_token") and settings.get("tg_chat_id"):
        for ev in events:
            et = ev["event_type"]
            if et == "new_listing" and settings.get("tg_notify_new_listing"):
                pass
            elif et == "restock" and settings.get("tg_notify_restock"):
                pass
            elif et == "delisted" and settings.get("tg_notify_delisted"):
                pass
            elif et == "price_change" and settings.get("tg_notify_price_change"):
                pass
            else:
                continue
            await send_telegram(
                token=settings["tg_bot_token"],
                chat_id=settings["tg_chat_id"],
                event=ev,
            )

    # Broadcast to WebSocket clients
    if events:
        await broadcast({"type": "events", "data": events})


# ---------------------------------------------------------------------------
# Pollers
# ---------------------------------------------------------------------------

async def poll_vps(token: str, settings: dict) -> None:
    try:
        data = await api.get_vps_store(token)
    except httpx.HTTPError as e:
        logger.warning("VPS poll failed: %s", e)
        return

    areas = (data.get("data") or {}).get("areas") or []

    async with SessionLocal() as db:
        seen: set[str] = set()
        all_events: list[dict] = []

        for area in areas:
            area_name = area.get("area_name", "")
            for node in area.get("nodes") or []:
                node_name = node.get("group_name", "")
                for plan in node.get("plans") or []:
                    pid = str(plan["id"])
                    seen.add(pid)
                    price_datas = plan.get("price_datas") or []
                    fields = {
                        "area": area_name,
                        "node": node_name,
                        "name": plan.get("plan_name", ""),
                        "stock": plan.get("stock", 0),
                        "price_monthly": _price_monthly(price_datas),
                        "price_yearly": _price_yearly(price_datas),
                        "cpu": plan.get("cpu", 0),
                        "memory_mb": plan.get("memory", 0),
                        "disk_gb": plan.get("disk", 0),
                        "bandwidth_mbps": plan.get("bandwidth", 0),
                        "traffic_gb": 0,
                        "ipv4": plan.get("ipv4_num", 0),
                        "available": plan.get("stock", 0) > 0,
                        "extra": json.dumps({"tag": plan.get("tag"), "is_pre_sale": plan.get("is_pre_sale")}),
                    }
                    evs = await _upsert(db, "vps", pid, fields)
                    all_events.extend(evs)

        evs = await _mark_delisted(db, "vps", seen)
        all_events.extend(evs)
        await _save_events(db, all_events, settings)
        await broadcast({"type": "refresh", "category": "vps"})


async def poll_ls(token: str, settings: dict) -> None:
    ls_regions: list[str] = settings.get("ls_regions") or []

    async with SessionLocal() as db:
        seen: set[str] = set()
        all_events: list[dict] = []

        for region in ls_regions:
            try:
                data = await api.get_ls_store(token, region)
            except httpx.HTTPError as e:
                logger.warning("LS poll %s failed: %s", region, e)
                continue

            bundles = (data.get("data") or {}).get("Bundles") or []
            for b in bundles:
                pid = f"ls_{region}_{b['BundleId']}"
                seen.add(pid)
                fields = {
                    "area": region,
                    "node": "",
                    "name": b["BundleId"],
                    "stock": -1,  # LS doesn't expose stock
                    "price_monthly": b.get("Price", 0),
                    "price_yearly": None,
                    "cpu": b.get("CPU", 0),
                    "memory_mb": b.get("Memory", 0),
                    "disk_gb": b.get("SysDiskSpace", 0),
                    "bandwidth_mbps": b.get("Bandwidth", 0),
                    "traffic_gb": b.get("TrafficPacket", 0),
                    "ipv4": 1,
                    "available": True,
                    "extra": None,
                }
                evs = await _upsert(db, "ls", pid, fields)
                all_events.extend(evs)

        evs = await _mark_delisted(db, "ls", seen)
        all_events.extend(evs)
        await _save_events(db, all_events, settings)
        await broadcast({"type": "refresh", "category": "ls"})


async def poll_secondhand(token: str, settings: dict) -> None:
    try:
        data = await api.get_secondhand_list(token)
    except httpx.HTTPError as e:
        logger.warning("Secondhand poll failed: %s", e)
        return

    listings = data.get("list") or []

    async with SessionLocal() as db:
        seen: set[str] = set()
        all_events: list[dict] = []

        for item in listings:
            pid = str(item.get("id", item.get("plan_id", "")))
            if not pid:
                continue
            seen.add(pid)

            # Build best fields from whatever the API returns
            price_datas = item.get("price_datas") or item.get("prices") or []
            fields = {
                "area": item.get("area_name", item.get("area", "")),
                "node": item.get("node_name", item.get("node", "")),
                "name": item.get("plan_name", item.get("name", f"ID:{pid}")),
                "stock": item.get("stock", 1),
                "price_monthly": (
                    item.get("price")
                    or _price_monthly(price_datas)
                    or 0
                ),
                "price_yearly": _price_yearly(price_datas),
                "cpu": item.get("cpu", 0),
                "memory_mb": item.get("memory", 0),
                "disk_gb": item.get("disk", 0),
                "bandwidth_mbps": item.get("bandwidth", 0),
                "traffic_gb": item.get("flow", 0),
                "ipv4": item.get("ipv4_num", 0),
                "available": True,
                "extra": json.dumps(item),
            }
            evs = await _upsert(db, "secondhand", pid, fields)
            all_events.extend(evs)

        evs = await _mark_delisted(db, "secondhand", seen)
        all_events.extend(evs)
        await _save_events(db, all_events, settings)
        await broadcast({"type": "refresh", "category": "secondhand"})


async def poll_price_history(token: str) -> None:
    """Sync GetAllRecyclePackage into price_history table."""
    try:
        items = await api.get_recycle_prices(token)
    except httpx.HTTPError as e:
        logger.warning("PriceHistory poll failed: %s", e)
        return

    async with SessionLocal() as db:
        for item in items:
            pid = str(item.get("plan_id", ""))
            if not pid:
                continue
            # Only insert if not already recorded (idempotent by plan_id+price+cycle)
            exists = await db.scalar(
                select(PriceHistory).where(
                    PriceHistory.plan_id == pid,
                    PriceHistory.price == item["price"],
                    PriceHistory.cycle == item["cycle"],
                )
            )
            if not exists:
                db.add(PriceHistory(
                    plan_id=pid,
                    price=item["price"],
                    cycle=item["cycle"],
                ))
        await db.commit()


# ---------------------------------------------------------------------------
# Master poll entry point (called by scheduler)
# ---------------------------------------------------------------------------

async def run_poll() -> None:
    # Keep the token alive before every poll cycle: cheap no-op when healthy,
    # refreshes in-place (and persists to DB) when near expiry. This makes the
    # backend fully self-sufficient — no external cron needed.
    from .token_manager import ensure_fresh
    try:
        await ensure_fresh()
    except Exception:
        logger.exception("ensure_fresh failed during poll (continuing)")

    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    token = settings.get("akile_token") or ""
    if not token:
        logger.info("No Akile token configured, skipping poll")
        return

    if settings.get("vps_enabled"):
        await poll_vps(token, settings)
    if settings.get("ls_enabled"):
        await poll_ls(token, settings)
    if settings.get("secondhand_enabled"):
        await poll_secondhand(token, settings)

    await poll_price_history(token)
