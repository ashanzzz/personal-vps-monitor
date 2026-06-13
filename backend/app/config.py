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
    "akile_token": "",
    "refresh_interval": 60,          # seconds
    "vps_enabled": True,
    "ls_enabled": True,
    "secondhand_enabled": True,
    "watched_plans": [],              # list of plan_id strings to highlight
    "tg_bot_token": "",
    "tg_chat_id": "",
    "tg_notify_new_listing": True,
    "tg_notify_restock": True,
    "tg_notify_delisted": False,
    "tg_notify_price_change": False,
    "tg_notify_purchased": True,
    "auto_buy_enabled": False,
    "auto_buy_plans": [],             # plan_ids eligible for auto-buy
    "auto_buy_max_price": 0,          # fen; 0 = no limit
    "auto_buy_cycle": 1,              # preferred billing cycle in months
    "ls_regions": LS_REGIONS,
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
