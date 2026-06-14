"""
Settings CRUD – thin wrapper over the KV Settings table.
All values stored as JSON strings.
"""
import json
from typing import Any

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from .database import Settings

# LS regions Akile supports (used to enumerate the store)
LS_REGIONS = ["HK", "JP", "TW", "US", "SG", "DE", "KR", "MO"]

DEFAULTS: dict[str, Any] = {
    # --- API & connection ---
    "akile_token": "",
    "refresh_interval": 60,          # seconds
    "auto_refresh_token": True,       # keep token alive automatically
    "refresh_margin_hours": 6,        # refresh when validity drops below this
    "request_timeout": 15,            # seconds per upstream request
    "request_retries": 2,             # retries on transient upstream failure

    # --- Monitor scope ---
    "vps_enabled": True,
    "ls_enabled": True,
    "secondhand_enabled": True,
    "ls_regions": LS_REGIONS,

    # --- Filters (gate notifications, never the data collection) ---
    "watched_plans": [],              # list of plan_id strings to highlight
    "filter_areas": [],               # only notify for these areas; [] = all
    "filter_name_include": "",        # substring that must be present in name
    "filter_name_exclude": "",        # substring that excludes a machine
    "filter_max_price": 0,            # fen; 0 = no limit
    "filter_min_cpu": 0,
    "filter_min_memory_mb": 0,
    "filter_min_traffic_gb": 0,

    # --- Telegram notifications ---
    "tg_bot_token": "",
    "tg_chat_id": "",
    "tg_notify_new_listing": True,
    "tg_notify_restock": True,
    "tg_notify_delisted": False,
    "tg_notify_price_change": False,
    "tg_notify_purchased": True,
    "notify_scope": "all",            # "all" | "watched"  (watched = only watched_plans)
    "dnd_enabled": False,             # do-not-disturb time window
    "dnd_start": "23:00",
    "dnd_end": "08:00",
    "merge_interval_minutes": 0,      # collapse same-type notifications; 0 = off

    # --- Generic webhook (Server酱 / 企业微信 / Bark / ...) ---
    "webhook_enabled": False,
    "webhook_url": "",
    "webhook_notify_new_listing": True,
    "webhook_notify_restock": True,
    "webhook_notify_delisted": False,
    "webhook_notify_price_change": False,
    "webhook_notify_purchased": True,

    # --- Auto-buy (secondhand) ---
    "auto_buy_enabled": False,
    "auto_buy_plans": [],             # plan_ids eligible for auto-buy
    "auto_buy_max_price": 0,          # fen; 0 = no limit
    "auto_buy_cycle": 1,              # preferred billing cycle in months
    "auto_buy_max_per_run": 1,        # max purchases in a single poll cycle
    "auto_buy_daily_limit": 1,        # max purchases per day

    # --- System ---
    "event_retention_days": 90,       # auto-prune events older than this
    "timezone": "Asia/Shanghai",
}


async def get_setting(db: AsyncSession, key: str) -> Any:
    row = await db.scalar(select(Settings).where(Settings.key == key))
    if row is None:
        return DEFAULTS.get(key)
    return json.loads(row.value)


async def set_setting(db: AsyncSession, key: str, value: Any) -> None:
    row = await db.scalar(select(Settings).where(Settings.key == key))
    encoded = json.dumps(value)
    if row is None:
        db.add(Settings(key=key, value=encoded))
    else:
        row.value = encoded
    await db.commit()


async def get_all_settings(db: AsyncSession) -> dict[str, Any]:
    rows = (await db.execute(select(Settings))).scalars().all()
    result = dict(DEFAULTS)
    for row in rows:
        try:
            result[row.key] = json.loads(row.value)
        except Exception:
            pass
    return result


async def update_settings(db: AsyncSession, data: dict[str, Any]) -> dict[str, Any]:
    for k, v in data.items():
        if k in DEFAULTS:
            await set_setting(db, k, v)
    return await get_all_settings(db)
