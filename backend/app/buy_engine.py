"""
Auto-buy engine: condition-range rule matching, candidate ranking, and
quota-gated purchasing.

Design (confirmed with user):
  - Each page (cloud/ls/secondhand) has independent config + rules.
  - A rule is a set of range/equality conditions. A machine matches when it
    satisfies ALL non-empty conditions — so brand-new plans/names match too.
  - When multiple machines match one rule, rank by [price asc, hw_score desc]:
    cheapest first; on a price tie the stronger hardware wins.
  - Three quota gates before each buy: per-rule, page-per-run, page-per-day.
  - on_limit_reached: "stop" removes nothing (caller decides) — we simply stop
    buying for that rule once max_per_rule is hit; "keep_watching" keeps trying
    to backfill on later cycles. Purchase counts are tracked in buy_state.
"""
from __future__ import annotations

import logging
from datetime import date

from .config import get_all_settings
from .database import MachineSnapshot, SessionLocal
from .logging_setup import log_event

logger = logging.getLogger(__name__)

# Bandwidth tier buckets (Mbps) used for hardware scoring.
_BW_TIERS = [50, 100, 200, 500, 1000, 2000, 5000, 10000]


def _bw_tier(mbps: int) -> int:
    """Map a bandwidth value to a tier index (0-based) for scoring."""
    tier = 0
    for i, t in enumerate(_BW_TIERS):
        if mbps >= t:
            tier = i + 1
    return tier


def hardware_score(m: MachineSnapshot, weights: dict) -> float:
    """Weighted hardware strength. Higher = stronger config."""
    return (
        m.cpu * weights.get("score_w_cpu", 10.0)
        + (m.memory_mb / 1024.0) * weights.get("score_w_mem", 5.0)
        + m.disk_gb * weights.get("score_w_disk", 0.5)
        + _bw_tier(m.bandwidth_mbps) * weights.get("score_w_bw", 2.0)
        + (m.traffic_gb or 0) * weights.get("score_w_traffic", 0.05)
    )


def _in_range(value: int, lo: int, hi: int) -> bool:
    """lo/hi of 0 mean unbounded on that side."""
    if lo and value < lo:
        return False
    if hi and value > hi:
        return False
    return True


def rule_matches(rule: dict, m: MachineSnapshot, page_max_price: int) -> bool:
    """True when machine m satisfies all non-empty conditions of rule."""
    if not rule.get("enabled", True):
        return False

    kw = (rule.get("name_keyword") or "").strip()
    if kw and kw not in (m.name or ""):
        return False

    if not _in_range(m.cpu, rule.get("cpu_min", 0), rule.get("cpu_max", 0)):
        return False
    if not _in_range(m.memory_mb, rule.get("memory_min_mb", 0), rule.get("memory_max_mb", 0)):
        return False
    if not _in_range(m.disk_gb, rule.get("disk_min_gb", 0), rule.get("disk_max_gb", 0)):
        return False

    # Bandwidth: any / min / exact
    mode = rule.get("bandwidth_mode", "any")
    bw = rule.get("bandwidth_mbps", 0)
    if mode == "min" and bw and m.bandwidth_mbps < bw:
        return False
    if mode == "exact" and bw and m.bandwidth_mbps != bw:
        return False

    if rule.get("traffic_min_gb", 0) and (m.traffic_gb or 0) < rule["traffic_min_gb"]:
        return False

    # Price ceilings: rule-level and page-level (fen, monthly).
    rule_cap = rule.get("price_max_monthly", 0)
    if rule_cap and m.price_monthly and m.price_monthly > rule_cap:
        return False
    if page_max_price and m.price_monthly and m.price_monthly > page_max_price:
        return False

    areas = rule.get("areas") or []
    if areas and m.area not in areas:
        return False

    return True


def rank_candidates(machines: list[MachineSnapshot], weights: dict) -> list[MachineSnapshot]:
    """Sort by price ascending, then hardware score descending."""
    return sorted(
        machines,
        key=lambda m: (m.price_monthly or 0, -hardware_score(m, weights)),
    )


# ---------------------------------------------------------------------------
# Per-day / per-run purchase tracking (in-memory; reset on date change)
# ---------------------------------------------------------------------------

class BuyState:
    """Tracks purchases per page/rule for quota enforcement."""

    def __init__(self) -> None:
        self._day = date.today()
        self._daily: dict[str, int] = {}      # page -> count today
        self._rule_daily: dict[str, int] = {}  # f"{page}:{rule_id}" -> count today

    def _rollover(self) -> None:
        today = date.today()
        if today != self._day:
            self._day = today
            self._daily.clear()
            self._rule_daily.clear()

    def daily(self, page: str) -> int:
        self._rollover()
        return self._daily.get(page, 0)

    def rule_count(self, page: str, rule_id: str) -> int:
        self._rollover()
        return self._rule_daily.get(f"{page}:{rule_id}", 0)

    def record(self, page: str, rule_id: str) -> None:
        self._rollover()
        self._daily[page] = self._daily.get(page, 0) + 1
        k = f"{page}:{rule_id}"
        self._rule_daily[k] = self._rule_daily.get(k, 0) + 1


buy_state = BuyState()
