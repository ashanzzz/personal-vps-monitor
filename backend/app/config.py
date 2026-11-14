"""
Settings CRUD – thin wrapper over the KV Settings table.
All values stored as JSON strings.
"""
import json
from typing import Any

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from .database import Settings
from .scoring import default_score_rules

# LS regions Akile supports (used to enumerate the store)
LS_REGIONS = ["HK", "JP", "TW", "US", "SG", "DE", "KR", "MO"]

DEFAULTS: dict[str, Any] = {
    # --- API & connection ---
    "akile_token": "",
    "acck_token": "",
    "lazycat_token": "",
    "lazycat_enabled": True,
    "refresh_interval": 60,          # seconds
    "auto_refresh_token": True,         # keep token alive automatically
    "refresh_margin_min_seconds": 60,   # min random refresh margin (seconds)
    "refresh_margin_max_seconds": 21600, # max random refresh margin (seconds, default 6h)
    "request_timeout": 15,            # seconds per upstream request
    "request_retries": 2,             # retries on transient upstream failure
    "account_refresh_interval": 300,  # seconds; how often the account page cache (balance/servers) is refreshed

    # --- Safety & Anti-Misbuying Protection ---
    "safe_buy_mode": True,            # 防误购保护模式：默认开启，杜绝自动扣款
    "real_purchase_enabled": False,   # 真实购买许可：默认关闭，绝不自动付款

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
    "filter_min_score": 0,            # backend score threshold for notifications
    "filter_min_star": 0,             # star threshold for notifications

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

    # Per-page hardware scoring rules — a column "评分" on each list page is
    # computed from these. Each is a {dimension: weight} map; score is the
    # weighted sum Σ(dimension_value × weight). Empty {} = use code defaults.
    # See scoring.py (compute_score / DEFAULT_SCORE_RULES). Each page is
    # independent so cloud / ls / secondhand can weight differently.
    "score_rules_cloud": default_score_rules(),
    "score_rules_ls": default_score_rules(),
    "score_rules_secondhand": default_score_rules(),

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
    "auto_checkin_enabled": True,       # daily auto sign-in for AK coins
    "acck_auto_checkin_enabled": True,  # daily auto sign-in for ACCK
    "checkin_mode": "fixed",           # "fixed" (固定几点) | "random" (随机时间窗口)
    "checkin_fixed_time": "08:00",     # 固定打卡时间 (HH:MM)
    "checkin_time_start": "08:00",     # random checkin window start (HH:MM)
    "checkin_time_end": "09:00",       # random checkin window end (HH:MM)
    "random_jitter_enabled": True,      # add random jitter to poll intervals (±20%)
    "store_last_refresh": None,          # last time store data was refreshed (ISO8601)
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


def spec_defaults() -> dict:
    """Per-dimension min/max ranges for a rule. 0 on a side = unbounded.

    memory is in MB (matching the stored field); the others map 1:1 to the
    snapshot fields (cpu cores, disk GB, bandwidth Mbps, traffic GB).
    """
    return {dim: {"min": 0, "max": 0} for dim in
            ("cpu", "memory", "disk", "bandwidth", "traffic")}


def rule_defaults() -> dict:
    """A single buy rule. Empty/0 fields mean 'no constraint'.

    Scoring and buying are decoupled: this rule only filters + counts; the
    page's scoring rules decide which matched machine is bought first.
    """
    return {
        "id": "",                      # client-generated unique id
        "enabled": True,
        "label": "",                   # display label, e.g. jppro (optional)
        "tags_include": [],            # match ANY substring in name/node; [] = any
        "spec": spec_defaults(),       # cpu/memory/disk/bandwidth/traffic min-max
        "price_type": "monthly",       # "monthly" | "yearly" (mutually exclusive)
        "price_max": 0,                # fen; cap on the chosen-cycle renewal price; 0 = no limit
        "sale_price_max": 0,           # fen; cap on the sale price; 0 = no limit
        "max_buy": 1,                  # max buys for this rule; 0 = unlimited
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
            # Normalize each rule against rule_defaults (deep-merge spec).
            norm_rules = []
            for rule in merged.get("rules") or []:
                if not isinstance(rule, dict):
                    continue
                r = rule_defaults()
                saved_spec = rule.get("spec")
                r.update(rule)
                # spec is nested {dim: {min, max}}; merge per-dimension so a
                # partial save never drops dimensions or sides.
                spec = spec_defaults()
                if isinstance(saved_spec, dict):
                    for dim, rng in saved_spec.items():
                        if dim in spec and isinstance(rng, dict):
                            spec[dim] = {
                                "min": rng.get("min", 0) or 0,
                                "max": rng.get("max", 0) or 0,
                            }
                r["spec"] = spec
                norm_rules.append(r)
            merged["rules"] = norm_rules
        result[key] = merged

    # Merge each page's score rules over the defaults so a partial save (or an
    # older DB row missing newer dimensions) always exposes every weight.
    for page in PAGE_KEYS:
        key = f"score_rules_{page}"
        merged_rules = default_score_rules()
        saved = result.get(key) or {}
        if isinstance(saved, dict):
            for dim, weight in saved.items():
                try:
                    merged_rules[dim] = float(weight)
                except (TypeError, ValueError):
                    continue
        result[key] = merged_rules
    return result


async def update_settings(db: AsyncSession, data: dict[str, Any]) -> dict[str, Any]:
    for k, v in data.items():
        if k in DEFAULTS:
            await set_setting(db, k, v)
    return await get_all_settings(db)
