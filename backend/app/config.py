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
    "account_refresh_interval": 300,  # seconds; how often the account page cache (balance/servers) is refreshed

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

    # --- Per-page auto-buy (cloud / ls / secondhand) ---
    # Each page has an independent refresh interval, page-level quota gates,
    # and a list of condition-range buy rules. See PAGE_BUY_DEFAULTS below.
    "cloud_buy": dict(),              # filled from PAGE_BUY_DEFAULTS at read time
    "ls_buy": dict(),
    "secondhand_buy": dict(),

    # plan_id -> human label map (e.g. "111" -> "jppro"); shown in UI, used for
    # per-rule quotas by label. Empty = show raw plan_id.
    "plan_label_map": {},

    # --- Per-page table column widths (UI only) ---
    # Each maps {column key -> width px}; empty {} means "use the code defaults".
    # Persisted independently per page so each list view remembers its layout.
    "col_widths_cloud": {},
    "col_widths_ls": {},
    "col_widths_secondhand": {},

    # Hardware scoring weights — used to pick between equally-priced candidates.
    # score = cpu*w_cpu + mem_gb*w_mem + disk_gb*w_disk + bw_tier*w_bw + traffic_gb*w_traffic
    "score_w_cpu": 10.0,
    "score_w_mem": 5.0,
    "score_w_disk": 0.5,
    "score_w_bw": 2.0,
    "score_w_traffic": 0.05,

    # --- Logging ---
    "log_level": "INFO",              # INFO | DEBUG

    # --- System ---
    "event_retention_days": 90,       # auto-prune events older than this
    "timezone": "Asia/Shanghai",
}


# Default shape for each page's auto-buy config. Refresh interval is per-page
# (slider 60–6000s in UI, but anything >=5 accepted for power users).
def page_buy_defaults() -> dict:
    return {
        "enabled": False,
        "refresh_interval": 60,        # seconds, independent per page
        "max_total_per_run": 1,        # all rules combined, per cycle
        "max_total_daily": 1,          # all rules combined, per day
        "max_price": 0,                # fen; 0 = no page-level cap
        "cycle": 1,                    # billing cycle months for purchases
        "on_limit_reached": "keep_watching",  # "stop" | "keep_watching"
        "rules": [],                   # list of rule dicts (see rule_defaults)
    }


def rule_defaults() -> dict:
    """A single condition-range buy rule. Empty/0 fields mean 'no constraint'."""
    return {
        "id": "",                      # client-generated unique id
        "enabled": True,
        "label": "",                   # display label, e.g. jppro (optional)
        "name_keyword": "",            # title must contain this (optional)
        "cpu_min": 0, "cpu_max": 0,            # 0 = unbounded on that side
        "memory_min_mb": 0, "memory_max_mb": 0,
        "disk_min_gb": 0, "disk_max_gb": 0,
        "bandwidth_mode": "any",       # "any" | "min" | "exact"
        "bandwidth_mbps": 0,
        "traffic_min_gb": 0,
        "price_max_monthly": 0,        # fen; 0 = no limit
        "areas": [],                   # area allow-list; [] = all
        "max_per_rule": 1,             # max buys for this rule; 0 = unlimited
    }


PAGE_KEYS = ["cloud", "ls", "secondhand"]


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
    # Merge page-buy configs over their defaults so partial saves stay valid.
    for page in PAGE_KEYS:
        key = f"{page}_buy"
        merged = page_buy_defaults()
        saved = result.get(key) or {}
        if isinstance(saved, dict):
            merged.update(saved)
            # Normalize each rule against rule_defaults.
            norm_rules = []
            for rule in merged.get("rules") or []:
                if not isinstance(rule, dict):
                    continue
                r = rule_defaults()
                r.update(rule)
                norm_rules.append(r)
            merged["rules"] = norm_rules
        result[key] = merged
    return result


async def update_settings(db: AsyncSession, data: dict[str, Any]) -> dict[str, Any]:
    for k, v in data.items():
        if k in DEFAULTS:
            await set_setting(db, k, v)
    return await get_all_settings(db)
