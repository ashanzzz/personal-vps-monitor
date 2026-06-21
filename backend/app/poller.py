"""
Polling logic: fetch Akile data, diff against DB, emit events.
"""
from __future__ import annotations

import json
import logging
import re
import time
from decimal import Decimal, InvalidOperation
from datetime import datetime
from typing import TYPE_CHECKING

import httpx
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from . import akile_client as api
from .config import get_all_settings, set_setting
from .database import AccountCache, AlertEvent, MachineSnapshot, PriceHistory, SessionLocal
from .notify import in_dnd_window, send_telegram, send_webhook
from .scoring import compute_score, compute_star_rating
from .ws import broadcast

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)

# Module-level merge state: last notification time per (channel, event_type).
# Used to collapse bursts of the same event type when merge_interval_minutes > 0.
_last_notified: dict[str, float] = {}

STORE_REFRESH_LIMIT = 100


def _coerce_fen(raw: object) -> int:
    if raw in (None, "") or isinstance(raw, bool):
        return 0
    if isinstance(raw, int):
        return max(raw, 0)
    if isinstance(raw, float):
        return max(int(raw), 0)
    s = str(raw).strip()
    if s.isdigit():
        return int(s)
    return 0


def _parse_sale_price(raw: object) -> int | None:
    if raw in (None, "") or isinstance(raw, bool):
        return None
    if isinstance(raw, int):
        return raw if raw >= 0 else None
    if isinstance(raw, float):
        return int(round(raw * 100)) if raw >= 0 else None
    s = str(raw).strip().replace(",", "")
    if not s:
        return None
    if s.isdigit():
        return int(s)
    m = re.search(r"(\d+(?:\.\d+)?)", s)
    if not m:
        return None
    try:
        return int((Decimal(m.group(1)) * 100).quantize(Decimal("1")))
    except (InvalidOperation, ValueError):
        return None


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _price_monthly(price_datas: list[dict]) -> int:
    for p in price_datas:
        if p.get("cycle") == 1:
            return _coerce_fen(p.get("price"))
    return _coerce_fen(price_datas[0].get("price")) if price_datas else 0


def _price_yearly(price_datas: list[dict]) -> int | None:
    for p in price_datas:
        if p.get("cycle") == 12:
            value = _coerce_fen(p.get("price"))
            return value or None
    return None


def _ip_status(item: dict) -> str:
    """Normalize a secondhand listing's seller-declared IP status.

    Akile's listing form stores this as ``ipStatus`` with values gfw / normal,
    but be liberal about the exact key and value shape since the public listing
    payload isn't documented. Returns "gfw", "normal", or "" (unknown).
    """
    raw = (
        item.get("ipStatus")
        or item.get("ip_status")
        or item.get("ipstatus")
        or ""
    )
    # Some APIs use numeric codes (0/1/2) — map conservatively.
    if isinstance(raw, (int, float)):
        return {1: "normal", 2: "gfw"}.get(int(raw), "")
    s = str(raw).strip().lower()
    if not s:
        return ""
    if s in ("gfw", "blocked", "ban", "banned", "2") or "墙" in s:
        return "gfw"
    if s in ("normal", "ok", "good", "1") or "正常" in s:
        return "normal"
    return ""


# ---------------------------------------------------------------------------
# Snapshot upsert + diff
# ---------------------------------------------------------------------------

async def _upsert(db: AsyncSession, category: str, plan_id: str, fields: dict) -> list[dict]:
    """
    Upsert a MachineSnapshot row and return a list of change-event dicts.
    Each event: {category, plan_id, name, area, event_type, old_value, new_value}
    """
    # Wrap the lookup in no_autoflush: a plain query would otherwise trigger
    # autoflush, prematurely flushing pending objects and hitting the SQLite
    # write lock ("raised as a result of Query-invoked autoflush").
    with db.no_autoflush:
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

    if events:
        for ev in events:
            await _dispatch_event(db, ev, settings)

    # Broadcast to WebSocket clients (UI always sees everything, unfiltered)
    if events:
        await broadcast({"type": "events", "data": events})


async def _touch_store_last_refresh(db: AsyncSession) -> None:
    await set_setting(db, "store_last_refresh", datetime.utcnow().isoformat() + "Z")


# ---------------------------------------------------------------------------
# Notification filtering + dispatch
# ---------------------------------------------------------------------------

async def _machine_for(db: AsyncSession, category: str, plan_id: str) -> MachineSnapshot | None:
    return await db.scalar(
        select(MachineSnapshot).where(
            MachineSnapshot.category == category,
            MachineSnapshot.plan_id == plan_id,
        )
    )


def _passes_filters(ev: dict, machine: MachineSnapshot | None, settings: dict) -> bool:
    """Decide whether an event is worth notifying about, based on user filters.

    Filters only gate notifications — data collection and the UI always see
    everything. Returns True when the event should be sent.
    """
    # Scope: only watched plans?
    if settings.get("notify_scope") == "watched":
        watched = settings.get("watched_plans") or []
        if ev.get("plan_id") not in watched:
            return False

    # Area allow-list
    areas = settings.get("filter_areas") or []
    if areas and ev.get("area") not in areas:
        return False

    # Name include / exclude
    name = ev.get("name") or ""
    inc = (settings.get("filter_name_include") or "").strip()
    exc = (settings.get("filter_name_exclude") or "").strip()
    if inc and inc not in name:
        return False
    if exc and exc in name:
        return False

    # Spec / price floors — need the machine snapshot for these
    if machine is not None:
        max_price = settings.get("filter_max_price") or 0
        if max_price and machine.price_monthly and machine.price_monthly > max_price:
            return False
        if (settings.get("filter_min_cpu") or 0) and machine.cpu < settings["filter_min_cpu"]:
            return False
        if (settings.get("filter_min_memory_mb") or 0) and machine.memory_mb < settings["filter_min_memory_mb"]:
            return False
        if (settings.get("filter_min_traffic_gb") or 0) and (machine.traffic_gb or 0) < settings["filter_min_traffic_gb"]:
            return False
        min_score = settings.get("filter_min_score") or 0
        min_star = settings.get("filter_min_star") or 0
        if min_score or min_star:
            score_rules = {
                "vps": settings.get("score_rules_cloud"),
                "ls": settings.get("score_rules_ls"),
                "secondhand": settings.get("score_rules_secondhand"),
            }.get(getattr(machine, "category", ""), settings.get("score_rules_cloud"))
            score = compute_score(machine, score_rules)
            if min_score and score < min_score:
                return False
            if min_star and compute_star_rating(score) < min_star:
                return False

    return True


def _channel_wants(prefix: str, event_type: str, settings: dict) -> bool:
    """Per-channel per-event-type toggle, e.g. tg_notify_restock."""
    key = f"{prefix}_notify_{event_type}"
    # delisted and out_of_stock share the delisted toggle
    if event_type == "out_of_stock":
        key = f"{prefix}_notify_delisted"
    return bool(settings.get(key))


def _merge_ok(channel: str, event_type: str, settings: dict) -> bool:
    """Rate-limit same-type notifications when merge_interval_minutes > 0."""
    minutes = settings.get("merge_interval_minutes") or 0
    if not minutes:
        return True
    key = f"{channel}:{event_type}"
    now = time.time()
    last = _last_notified.get(key, 0)
    if now - last < minutes * 60:
        return False
    _last_notified[key] = now
    return True


async def _dispatch_event(db: AsyncSession, ev: dict, settings: dict) -> None:
    """Route a single event through filters, DND, merge, then channels."""
    et = ev["event_type"]
    machine = await _machine_for(db, ev.get("category", ""), ev.get("plan_id", ""))

    if not _passes_filters(ev, machine, settings):
        return

    # Do-not-disturb window suppresses all push (events are still stored + shown)
    if settings.get("dnd_enabled") and in_dnd_window(
        settings.get("dnd_start", "23:00"),
        settings.get("dnd_end", "08:00"),
        settings.get("timezone", "Asia/Shanghai"),
    ):
        return

    # Telegram
    if (
        settings.get("tg_bot_token")
        and settings.get("tg_chat_id")
        and _channel_wants("tg", et, settings)
        and _merge_ok("tg", et, settings)
    ):
        await send_telegram(
            token=settings["tg_bot_token"],
            chat_id=settings["tg_chat_id"],
            event=ev,
        )

    # Generic webhook
    if (
        settings.get("webhook_enabled")
        and settings.get("webhook_url")
        and _channel_wants("webhook", et, settings)
        and _merge_ok("webhook", et, settings)
    ):
        await send_webhook(settings["webhook_url"], ev)


# ---------------------------------------------------------------------------
# Pollers
# ---------------------------------------------------------------------------

async def poll_vps(token: str, settings: dict, limit: int | None = None) -> None:
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
                    if limit is not None and len(seen) >= limit:
                        break
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

        if limit is None:
            evs = await _mark_delisted(db, "vps", seen)
            all_events.extend(evs)
        await _save_events(db, all_events, settings)
        await _touch_store_last_refresh(db)
        await broadcast({"type": "refresh", "category": "vps"})


async def poll_ls(token: str, settings: dict, limit: int | None = None) -> None:
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
                if limit is not None and len(seen) >= limit:
                    break
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

        if limit is None:
            evs = await _mark_delisted(db, "ls", seen)
            all_events.extend(evs)
        await _save_events(db, all_events, settings)
        await _touch_store_last_refresh(db)
        await broadcast({"type": "refresh", "category": "ls"})


async def poll_secondhand(token: str, settings: dict, limit: int | None = None) -> None:
    # Paginate through the whole market: the API returns ~30 per page by
    # default but the market has ~89 listings. Loop from page=1 accumulating
    # `list` until we've collected `total` rows or a page comes back empty.
    from .logging_setup import log_event
    PAGE_SIZE = 100
    MAX_PAGES = 20  # safety cap against runaway loops

    listings: list[dict] = []
    total = 0
    last_data: dict = {}
    for page in range(1, MAX_PAGES + 1):
        try:
            data = await api.get_secondhand_list(token, page=page,
                                                 page_size=PAGE_SIZE)
        except httpx.HTTPError as e:
            logger.warning("Secondhand poll failed (page %s): %s", page, e)
            log_event("poll.fetch", category="secondhand", ok=False,
                      error=str(e), page=page)
            return

        last_data = data
        total = data.get("total") or total
        page_list = data.get("list") or []
        if not page_list:
            break
        listings.extend(page_list)
        if total and len(listings) >= total:
            break

    log_event("poll.fetch", category="secondhand",
              status_code=last_data.get("status_code"), total=total,
              list_len=len(listings),
              reason=("empty_market" if not listings else "ok"))

    async with SessionLocal() as db:
        seen: set[str] = set()
        all_events: list[dict] = []

        for item in listings:
            if limit is not None and len(seen) >= limit:
                break
            pid = str(item.get("id", item.get("plan_id", "")))
            if not pid:
                continue
            seen.add(pid)

            # Build best fields from whatever the API returns
            price_datas = item.get("price_datas") or item.get("prices") or []
            sale_price = (
                _coerce_fen(item.get("sale_price"))
                or _coerce_fen(item.get("salePrice"))
                or _parse_sale_price(item.get("price"))
            )
            fields = {
                "area": item.get("area_name", item.get("area", "")),
                "node": item.get("node_name", item.get("node", "")),
                "name": item.get("plan_name", item.get("name", f"ID:{pid}")),
                "stock": item.get("stock", 1),
                "price_monthly": (
                    _coerce_fen(item.get("price_monthly"))
                    or _price_monthly(price_datas)
                    or 0
                ),
                "price_yearly": _price_yearly(price_datas),
                "sale_price": sale_price,
                "cpu": item.get("cpu", 0),
                "memory_mb": item.get("memory", 0),
                "disk_gb": item.get("disk", 0),
                "bandwidth_mbps": item.get("bandwidth", 0),
                "traffic_gb": item.get("flow", 0),
                "ipv4": item.get("ipv4_num", 0),
                "available": True,
                "ip_status": _ip_status(item),
                "extra": json.dumps(item),
            }
            try:
                evs = await _upsert(db, "secondhand", pid, fields)
                all_events.extend(evs)
            except Exception as e:
                # One bad row must not abort the whole batch.
                logger.warning("Secondhand upsert failed for pid=%s: %s", pid, e)
                continue

        if limit is None:
            evs = await _mark_delisted(db, "secondhand", seen)
            all_events.extend(evs)
        await _save_events(db, all_events, settings)
        await _touch_store_last_refresh(db)
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
# Store refresh helpers
# ---------------------------------------------------------------------------

async def run_store_refresh(page: str | None = None, limit: int | None = STORE_REFRESH_LIMIT) -> bool:
    """Refresh store data without running auto-buy.

    Manual refreshes are capped by STORE_REFRESH_LIMIT so the UI can trigger a
    fast batch update. Returns True if at least one store poll ran.
    """
    from .token_manager import ensure_fresh

    try:
        await ensure_fresh()
    except Exception:
        logger.exception("ensure_fresh failed during store refresh (continuing)")

    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    token = settings.get("akile_token") or ""
    if not token:
        logger.info("No Akile token configured, skipping store refresh")
        return False

    refreshed = False
    if page is None:
        if settings.get("vps_enabled"):
            await poll_vps(token, settings, limit=limit)
            refreshed = True
        if settings.get("ls_enabled"):
            await poll_ls(token, settings, limit=limit)
            refreshed = True
        if settings.get("secondhand_enabled"):
            await poll_secondhand(token, settings, limit=limit)
            refreshed = True
    else:
        entry = _PAGE_POLLERS.get(page)
        if entry is None:
            return False
        enabled_key, fn = entry
        if not settings.get(enabled_key):
            return False
        await fn(token, settings, limit=limit)
        refreshed = True

    if refreshed:
        await poll_price_history(token)
    return refreshed


# ---------------------------------------------------------------------------
# Account cache (balance + profile + own servers)
# ---------------------------------------------------------------------------

async def poll_account(token: str, settings: dict) -> None:
    """Fetch the logged-in account snapshot and store it in account_cache.

    Aggregates balance/profile from GET /v1/user/info and the account's own
    servers from POST /v1/server/GetServerV2List. Each upstream call degrades
    independently so a single failure does not blank the whole panel. The
    resulting payload (same shape as the /api/account response) is written to
    the single-row account_cache table with a fresh updated_at.
    """
    from .logging_setup import log_event

    errors: list[str] = []
    profile = None
    servers: list[dict] = []

    try:
        info = await api.get_user_info(token)
        profile = {
            "username": info.get("username", ""),
            "email": info.get("email", ""),
            "role": info.get("role", ""),
            "money": info.get("money"),            # balance in fen; None if missing
            "telegram_id": info.get("telegram_id"),
            "is_real_name": bool(info.get("is_real_name")),
            "is_ga": bool(info.get("is_ga")),
            "last_checkin_time": info.get("last_checkin_time"),
            "ak_coin": None,  # populated below
        }
    except httpx.HTTPError as e:
        logger.warning("Account profile poll failed: %s", e)
        errors.append(f"profile:{e}")

    # Fetch AK coin balance from the separate /v1/user/index endpoint.
    if profile is not None:
        try:
            coin_info = await api.get_ak_coin_info(token)
            profile["ak_coin"] = coin_info.get("ak_coin")
        except Exception as e:
            logger.warning("AK coin info poll failed: %s", e)
            errors.append(f"ak_coin:{e}")

    try:
        servers = await api.get_my_servers(token)
    except httpx.HTTPError as e:
        logger.warning("Account servers poll failed: %s", e)
        errors.append(f"servers:{e}")

    log_event("poll.fetch", category="account",
              ok=not errors, errors=errors or None,
              has_profile=profile is not None, server_count=len(servers))

    payload = {
        "configured": True,
        "profile": profile,
        "servers": servers,
        "errors": errors,
    }

    async with SessionLocal() as db:
        row = await db.scalar(select(AccountCache).where(AccountCache.id == 1))
        if row is None:
            db.add(AccountCache(id=1, data=json.dumps(payload)))
        else:
            row.data = json.dumps(payload)
            row.updated_at = datetime.utcnow()
        await db.commit()

    await broadcast({"type": "refresh", "category": "account"})


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


# ---------------------------------------------------------------------------
# Per-page poll + auto-buy (each runs on its own independent interval)
# ---------------------------------------------------------------------------

_PAGE_POLLERS = {
    "cloud": ("vps_enabled", poll_vps),
    "ls": ("ls_enabled", poll_ls),
    "secondhand": ("secondhand_enabled", poll_secondhand),
}


async def run_page_poll(page: str) -> None:
    """Poll a single page, then run its auto-buy rules. Used by per-page jobs."""
    from .logging_setup import log_event
    from .token_manager import ensure_fresh

    try:
        await ensure_fresh()
    except Exception:
        logger.exception("ensure_fresh failed during %s poll (continuing)", page)

    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    token = settings.get("akile_token") or ""
    if not token:
        log_event("poll.skip", page=page, reason="no_token")
        return

    entry = _PAGE_POLLERS.get(page)
    if entry is None:
        return
    enabled_key, fn = entry
    if not settings.get(enabled_key):
        return

    log_event("poll.start", page=page)
    try:
        await fn(token, settings)
        await poll_price_history(token)
    except Exception:
        logger.exception("poll %s failed", page)
        return

    # Run auto-buy for this page after fresh data is stored.
    from .buyer import run_autobuy
    try:
        await run_autobuy(page)
    except Exception:
        logger.exception("autobuy %s failed", page)
