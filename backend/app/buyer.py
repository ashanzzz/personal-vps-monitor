"""
Auto-buy runner: evaluate one page's buy rules against current snapshots and
place purchases through the Akile API, respecting all quota gates.

Called by the per-page scheduler after each poll cycle.
"""
from __future__ import annotations

import logging

from sqlalchemy import select

from . import akile_client as api
from .buy_engine import buy_state, rank_candidates, rule_matches
from .config import get_all_settings
from .database import AlertEvent, MachineSnapshot, SessionLocal
from .logging_setup import log_event

logger = logging.getLogger(__name__)

# Map page key -> snapshot category, and page key -> per-page score rules key.
_PAGE_CATEGORY = {"cloud": "vps", "ls": "ls", "secondhand": "secondhand"}
_PAGE_SCORE_KEY = {
    "cloud": "score_rules_cloud",
    "ls": "score_rules_ls",
    "secondhand": "score_rules_secondhand",
}


async def run_autobuy(page: str) -> None:
    """Evaluate and execute auto-buy for a single page."""
    category = _PAGE_CATEGORY.get(page)
    if category is None:
        return

    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    cfg = settings.get(f"{page}_buy") or {}
    if not cfg.get("enabled"):
        return

    token = settings.get("akile_token") or ""
    if not token:
        log_event("autobuy.skip", page=page, reason="no_token")
        return

    rules = cfg.get("rules") or []
    enabled_rules = [r for r in rules if r.get("enabled", True)]
    if not enabled_rules:
        return

    # Scoring rules (decoupled from buying) decide candidate ordering: the
    # highest-scoring matched machine is bought first.
    score_rules = settings.get(_PAGE_SCORE_KEY.get(page, ""))
    max_per_run = cfg.get("max_total_per_run", 1)
    max_daily = cfg.get("max_total_daily", 1)
    on_limit = cfg.get("on_limit_reached", "keep_watching")
    cycle = cfg.get("cycle", 1)

    # Load current available machines for this category.
    async with SessionLocal() as db:
        rows = (await db.execute(
            select(MachineSnapshot).where(
                MachineSnapshot.category == category,
                MachineSnapshot.available == True,  # noqa: E712
            )
        )).scalars().all()

    if not rows:
        log_event("autobuy.eval", page=page, available=0)
        return

    bought_this_run = 0
    log_event("autobuy.eval", page=page, available=len(rows), rules=len(enabled_rules))

    for rule in enabled_rules:
        rid = rule.get("id") or rule.get("label") or "rule"
        max_buy = rule.get("max_buy", 1)

        # Quota gate: per-rule cap (0 = unlimited).
        already = buy_state.rule_count(page, rid)
        if max_buy and already >= max_buy:
            if on_limit == "stop":
                log_event("autobuy.rule_done", page=page, rule=rid, bought=already)
            continue

        candidates = [m for m in rows if rule_matches(rule, m)]
        if not candidates:
            continue

        ranked = rank_candidates(candidates, score_rules)
        log_event(
            "autobuy.match", page=page, rule=rid, candidates=len(ranked),
            top_price=ranked[0].price_monthly, top_id=ranked[0].plan_id,
        )

        for m in ranked:
            # Re-check all gates before each purchase.
            if max_buy and buy_state.rule_count(page, rid) >= max_buy:
                break
            if max_per_run and bought_this_run >= max_per_run:
                log_event("autobuy.run_cap", page=page, cap=max_per_run)
                return
            if max_daily and buy_state.daily(page) >= max_daily:
                log_event("autobuy.daily_cap", page=page, cap=max_daily)
                return

            await _purchase(token, page, category, m, rule, rid, cycle, settings)
            buy_state.record(page, rid)
            bought_this_run += 1


async def _purchase(token, page, category, m, rule, rid, cycle, settings) -> None:
    log_event(
        "autobuy.attempt", page=page, rule=rid, plan_id=m.plan_id,
        name=m.name, price=m.price_monthly, cycle=cycle,
    )
    payload = {"id": m.plan_id, "cycle": cycle}
    try:
        result = await api.buy_secondhand(token, payload)
    except Exception as e:  # noqa: BLE001 - log and continue to next candidate
        log_event("autobuy.result", level=logging.WARNING, page=page, rule=rid,
                  plan_id=m.plan_id, ok=False, error=str(e))
        return

    code = result.get("status_code") if isinstance(result, dict) else None
    ok = code == 0
    log_event("autobuy.result", page=page, rule=rid, plan_id=m.plan_id,
              ok=ok, code=code, msg=(result or {}).get("status_msg"))

    # Record a purchase event + notify.
    event = {
        "category": category, "plan_id": m.plan_id, "name": m.name,
        "area": m.area, "event_type": "purchased",
        "old_value": None, "new_value": "购买成功" if ok else f"失败:{code}",
        "notified": False,
    }
    async with SessionLocal() as s:
        s.add(AlertEvent(**event))
        await s.commit()

    if ok and settings.get("tg_notify_purchased") and settings.get("tg_bot_token"):
        from .notify import send_telegram
        await send_telegram(settings["tg_bot_token"], settings.get("tg_chat_id", ""), event)

    from .ws import broadcast
    await broadcast({"type": "purchased", "plan_id": m.plan_id,
                     "result": {"ok": ok, "page": page, "rule": rid}})
