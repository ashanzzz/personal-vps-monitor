"""
Targeted high-frequency second-level sniper for high-value secondhand packages.
Runs every 60 seconds (minimum) using Akile's targeted name filter, keeping upstream load low.
"""
from __future__ import annotations

import asyncio
import json
import logging

from . import akile_client as api
from . import acck_client
from .buy_engine import buy_state
from .config import get_all_settings
from .database import AlertEvent, SessionLocal
from .logging_setup import log_event

logger = logging.getLogger("sniper")

_sniper_task: asyncio.Task | None = None
_stop_event = asyncio.Event()


def resolve_search_prefix(rule: dict) -> str:
    """Infer the Akile package name prefix for fast API filtering."""
    if rule.get("search_prefix"):
        return str(rule["search_prefix"]).strip()
    tags = rule.get("tags_include") or []
    for t in tags:
        t_low = str(t).lower()
        if "元旦" in t_low or "hklite" in t_low:
            return "HKLite-One"
        if "圣诞" in t_low or "hksm" in t_low:
            return "HKSM-One"
        if "jpiij" in t_low or "iij" in t_low:
            return "JPIIJ"
        if "jppro" in t_low:
            return "JPPro"
        if "jphyper" in t_low:
            return "JPHyper"
        if "lax" in t_low:
            return "LAXLite"
    return ""


def item_matches_sniper_rule(rule: dict, item: dict) -> bool:
    """Fast in-memory match for targeted listing."""
    rule_plat = rule.get("platform")
    if rule_plat and str(rule_plat).lower() not in ("all", ""):
        item_plat = item.get("platform", "akile")
        if str(item_plat).lower() != str(rule_plat).lower():
            return False

    name = str(item.get("name", "")).strip()
    status = item.get("status", 0)
    if status != 0:
        return False

    tags = rule.get("tags_include") or []
    if tags:
        name_lower = name.lower()
        if not any(str(tag).lower() in name_lower for tag in tags):
            return False

    sale_max = rule.get("sale_price_max", 0) or 0
    if sale_max > 0:
        try:
            price_fen = float(item.get("price", 0))
            if price_fen > sale_max:
                return False
        except (ValueError, TypeError):
            return False

    # Spec filtering (CPU, Memory, Disk, Bandwidth, Traffic)
    spec = rule.get("spec") or {}
    spec_fields = {
        "cpu": ["cpu"],
        "memory": ["memory", "memory_mb"],
        "disk": ["disk", "disk_gb"],
        "bandwidth": ["bandwidth", "bandwidth_mbps"],
        "traffic": ["flow", "flow_gb", "traffic", "traffic_gb"],
    }
    for dim, keys in spec_fields.items():
        rng = spec.get(dim) or {}
        lo = rng.get("min", 0) or 0
        hi = rng.get("max", 0) or 0
        if not lo and not hi:
            continue
        val = 0.0
        for k in keys:
            if k in item and item[k] is not None:
                try:
                    val = float(item[k])
                    break
                except (ValueError, TypeError):
                    pass
        if lo > 0 and val < lo:
            return False
        if hi > 0 and val > hi:
            return False

    return True


async def _run_akile_sniper_pass(rules: list[dict], token: str, settings: dict, max_daily: int) -> None:
    polled_prefixes: set[str] = set()
    for rule in rules:
        prefix = resolve_search_prefix(rule)
        if not prefix or prefix in polled_prefixes:
            continue
        polled_prefixes.add(prefix)

        rid = rule.get("id") or rule.get("label") or "akile_sniper_rule"
        max_buy = rule.get("max_buy", 1)
        if max_buy and buy_state.rule_count("secondhand", rid) >= max_buy:
            continue

        try:
            res = await api.get_secondhand_list(token, page=1, page_size=30, name=prefix)
        except Exception as e:
            logger.debug("Akile sniper poll failed for prefix %s: %s", prefix, e)
            continue

        items = res.get("list") or []
        for item in items:
            item_copy = dict(item)
            item_copy["platform"] = "akile"
            if not item_matches_sniper_rule(rule, item_copy):
                continue

            pid = str(item.get("id", ""))
            price_cny = round(float(item.get("price", 0)) / 100.0, 2)
            name = item.get("name", "")

            if max_buy and buy_state.rule_count("secondhand", rid) >= max_buy:
                break
            if max_daily and buy_state.daily("secondhand") >= max_daily:
                break

            if not settings.get("real_purchase_enabled", False):
                log_event(
                    "sniper.dry_run_match",
                    rule=rid, plan_id=pid, name=name, price=price_cny, platform="akile",
                    msg="【Akile秒杀命中·模拟模式】已锁定目标，真实购买未开启",
                )
                logger.info("[AKILE SNIPER DRY-RUN] Matched target %s (ID:%s) for ¥%.2f", name, pid, price_cny)
                continue

            log_event(
                "sniper.execute",
                rule=rid, plan_id=pid, name=name, price=price_cny, platform="akile",
                msg="【Akile秒杀触发·立即扣款购买】",
            )
            logger.warning("[AKILE SNIPER EXECUTE] Sniping %s (ID:%s) for ¥%.2f...", name, pid, price_cny)

            try:
                buy_res = await api.buy_secondhand(token, pid, auto_pay=True)
                code = buy_res.get("status_code", 0) if isinstance(buy_res, dict) else -1
                ok = code == 0
            except Exception as e:
                logger.error("[AKILE SNIPER ERROR] Purchase failed: %s", e)
                continue

            buy_state.record("secondhand", rid)
            log_event("sniper.result", rule=rid, plan_id=pid, ok=ok, code=code, platform="akile")

            event = {
                "category": "secondhand",
                "plan_id": pid,
                "name": name,
                "area": item.get("area_name", "中国香港"),
                "event_type": "purchased",
                "old_value": None,
                "new_value": f"[Akile] 秒杀成功(¥{price_cny})" if ok else f"[Akile] 秒杀失败:{code}",
                "notified": False,
            }
            async with SessionLocal() as s:
                s.add(AlertEvent(**event))
                await s.commit()

            if ok and settings.get("tg_notify_purchased") and settings.get("tg_bot_token"):
                from .notify import send_telegram
                await send_telegram(settings["tg_bot_token"], settings.get("tg_chat_id", ""), event)

            from .ws import broadcast
            await broadcast({
                "type": "purchased",
                "platform": "akile",
                "plan_id": pid,
                "result": {"ok": ok, "sniper": True, "rule": rid},
            })
            break


async def _run_acck_sniper_pass(rules: list[dict], token: str, settings: dict, max_daily: int) -> None:
    try:
        res = await acck_client.get_exchange_listings()
        items = res if isinstance(res, list) else res.get("items", [])
    except Exception as e:
        logger.debug("ACCK sniper poll failed: %s", e)
        return

    for rule in rules:
        rid = rule.get("id") or rule.get("label") or "acck_sniper_rule"
        max_buy = rule.get("max_buy", 1)
        if max_buy and buy_state.rule_count("secondhand", rid) >= max_buy:
            continue

        for it in items:
            snapshot = {}
            if it.get("snapshot_json"):
                try:
                    snapshot = json.loads(it["snapshot_json"]) if isinstance(it["snapshot_json"], str) else it["snapshot_json"]
                except Exception:
                    pass

            item_norm = {
                "platform": "acck",
                "id": str(it.get("id", "")),
                "name": snapshot.get("plan_name") or it.get("intro_text") or f"Exchange-{it.get('id')}",
                "price": int(float(str(it.get("ask_price_cents", 0)))),
                "status": 0 if not it.get("sold") and not it.get("closed") else 1,
                "cpu": snapshot.get("cpu"),
                "memory": snapshot.get("memory_mb"),
                "disk": snapshot.get("disk_gb") or snapshot.get("disk"),
                "bandwidth": snapshot.get("bandwidth_mbps"),
                "flow": snapshot.get("flow_gb"),
            }

            if not item_matches_sniper_rule(rule, item_norm):
                continue

            pid = str(it.get("id", ""))
            price_cny = round(item_norm["price"] / 100.0, 2)
            name = item_norm["name"]

            if max_buy and buy_state.rule_count("secondhand", rid) >= max_buy:
                break
            if max_daily and buy_state.daily("secondhand") >= max_daily:
                break

            if not settings.get("real_purchase_enabled", False):
                log_event(
                    "sniper.dry_run_match",
                    rule=rid, plan_id=pid, name=name, price=price_cny, platform="acck",
                    msg="【ACCK秒杀命中·模拟模式】已锁定目标，真实购买未开启",
                )
                logger.info("[ACCK SNIPER DRY-RUN] Matched target %s (ID:%s) for ¥%.2f", name, pid, price_cny)
                continue

            log_event(
                "sniper.execute",
                rule=rid, plan_id=pid, name=name, price=price_cny, platform="acck",
                msg="【ACCK秒杀触发·立即扣款购买】",
            )
            logger.warning("[ACCK SNIPER EXECUTE] Sniping %s (ID:%s) for ¥%.2f...", name, pid, price_cny)

            try:
                buy_res = await acck_client.buy_exchange_listing(token, int(pid))
                ok = isinstance(buy_res, dict) and buy_res.get("code", 0) in (0, 200)
                code = buy_res.get("code", 0) if isinstance(buy_res, dict) else -1
            except Exception as e:
                logger.error("[ACCK SNIPER ERROR] Purchase failed: %s", e)
                continue

            buy_state.record("secondhand", rid)
            log_event("sniper.result", rule=rid, plan_id=pid, ok=ok, code=code, platform="acck")

            event = {
                "category": "secondhand",
                "plan_id": pid,
                "name": name,
                "area": snapshot.get("area_name", "美国"),
                "event_type": "purchased",
                "old_value": None,
                "new_value": f"[ACCK] 秒杀成功(¥{price_cny})" if ok else f"[ACCK] 秒杀失败:{code}",
                "notified": False,
            }
            async with SessionLocal() as s:
                s.add(AlertEvent(**event))
                await s.commit()

            if ok and settings.get("tg_notify_purchased") and settings.get("tg_bot_token"):
                from .notify import send_telegram
                await send_telegram(settings["tg_bot_token"], settings.get("tg_chat_id", ""), event)

            from .ws import broadcast
            await broadcast({
                "type": "purchased",
                "platform": "acck",
                "plan_id": pid,
                "result": {"ok": ok, "sniper": True, "rule": rid},
            })
            break


async def run_sniper_cycle() -> None:
    """One single pass of targeted fast sniping with strict platform isolation."""
    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    cfg = settings.get("secondhand_buy") or {}
    if not cfg.get("enabled"):
        return

    rules = cfg.get("rules") or []
    enabled_rules = [r for r in rules if r.get("enabled", True)]
    if not enabled_rules:
        return

    max_daily = cfg.get("max_total_daily", 2)
    if max_daily and buy_state.daily("secondhand") >= max_daily:
        return

    # 抢购隔离机制：严格按平台分流目录，杜绝因抢购 Akile 而无限刷新 ACCK
    akile_rules = [r for r in enabled_rules if r.get("platform", "akile").lower() in ("akile", "")]
    acck_rules = [r for r in enabled_rules if r.get("platform", "").lower() == "acck"]

    # 1. Akile 抢购刷新 (仅在有启用的 Akile 规则时触发)
    akile_token = settings.get("akile_token") or ""
    if akile_rules and akile_token:
        await _run_akile_sniper_pass(akile_rules, akile_token, settings, max_daily)

    # 2. ACCK 抢购刷新 (仅在有启用的 ACCK 规则时触发，无规则零请求零刷新)
    acck_token = settings.get("acck_token") or ""
    if acck_rules and acck_token:
        await _run_acck_sniper_pass(acck_rules, acck_token, settings, max_daily)


async def sniper_worker_loop(interval_seconds: int = 60) -> None:
    """Infinite worker loop polling targeted packages every N seconds."""
    logger.info("Fast targeted sniper worker started (interval=%ds)", interval_seconds)
    while not _stop_event.is_set():
        try:
            await run_sniper_cycle()
        except asyncio.CancelledError:
            break
        except Exception:
            logger.exception("Unexpected error in sniper_worker_loop")
        try:
            await asyncio.wait_for(_stop_event.wait(), timeout=interval_seconds)
        except asyncio.TimeoutError:
            pass
    logger.info("Fast targeted sniper worker stopped")


def start_sniper_worker(interval_seconds: int = 60) -> asyncio.Task:
    global _sniper_task
    _stop_event.clear()
    if _sniper_task is None or _sniper_task.done():
        _sniper_task = asyncio.create_task(sniper_worker_loop(interval_seconds))
    return _sniper_task


def stop_sniper_worker() -> None:
    global _sniper_task
    _stop_event.set()
    if _sniper_task and not _sniper_task.done():
        _sniper_task.cancel()
        _sniper_task = None
