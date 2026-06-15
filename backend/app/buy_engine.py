"""
Auto-buy engine: per-rule candidate matching, score-based ranking, and
quota-gated purchasing.

Design (confirmed with user):
  - Each page (cloud/ls/secondhand) has independent config + a list of rules,
    stored under settings ``<page>_buy.rules`` as a JSON array.
  - A rule is a set of filter conditions. A machine matches when it satisfies
    ALL non-empty conditions, so brand-new plans/names match too:
      * tags_include: list of substrings; machine matches when ANY of them is
        contained in its name OR node (e.g. jppro / jpiij). Empty = any.
      * spec: {dimension: {min, max}} ranges over cpu / memory / disk /
        bandwidth / traffic. 0 (or missing) on a side = unbounded there.
      * price_type: "monthly" | "yearly" — which renewal price the rule caps.
      * price_max: ceiling (fen) on the chosen renewal price. 0 = no cap.
      * sale_price_max: ceiling (fen) on the sale price. 0 = no cap.
      * max_buy: per-rule purchase cap. Counted on every hit; stop at the cap.
  - Scoring and buying are DECOUPLED: the scoring rules (scoring.compute_score)
    decide ordering; the buy rule only filters and counts. When multiple
    machines match one rule, rank by score DESCENDING and buy the highest.
  - Three quota gates before each buy: per-rule (max_buy), page-per-run
    (max_total_per_run), page-per-day (max_total_daily).
  - on_limit_reached: "keep_watching" (default) keeps trying to backfill on
    later cycles; "stop" simply logs once the per-rule cap is hit. Purchase
    counts are tracked in buy_state.
"""
from __future__ import annotations

import logging
from datetime import date
from typing import Any

from .database import MachineSnapshot
from .scoring import compute_score

logger = logging.getLogger(__name__)

# Spec dimensions a rule can range over, mapped to the machine attribute and
# the unit conversion applied before comparison. Memory ranges are expressed
# in MB (matching the stored field); the others map 1:1.
_SPEC_FIELDS: dict[str, str] = {
    "cpu": "cpu",
    "memory": "memory_mb",
    "disk": "disk_gb",
    "bandwidth": "bandwidth_mbps",
    "traffic": "traffic_gb",
}


def _get(m: Any, key: str, default: Any = None) -> Any:
    if isinstance(m, dict):
        return m.get(key, default)
    return getattr(m, key, default)


def _in_range(value: float, lo: float, hi: float) -> bool:
    """lo/hi of 0 (or falsy) mean unbounded on that side."""
    if lo and value < lo:
        return False
    if hi and value > hi:
        return False
    return True


def _tags_match(tags: list[str], m: MachineSnapshot) -> bool:
    """True when ANY tag is a substring of the machine name or node.

    Empty tag list = no tag constraint (matches everything).
    """
    cleaned = [t.strip() for t in tags if isinstance(t, str) and t.strip()]
    if not cleaned:
        return True
    haystack = f"{_get(m, 'name', '') or ''}\n{_get(m, 'node', '') or ''}"
    return any(tag in haystack for tag in cleaned)


def _rule_price(rule: dict, m: MachineSnapshot) -> int | None:
    """The renewal price (fen) this rule caps, per its price_type.

    Returns None when the machine has no price for the chosen cycle, in which
    case the price ceiling can't be satisfied and the machine is excluded.
    """
    if rule.get("price_type") == "yearly":
        return _get(m, "price_yearly", None)
    return _get(m, "price_monthly", None)


def rule_matches(rule: dict, m: MachineSnapshot) -> bool:
    """True when machine m satisfies all non-empty conditions of rule."""
    if not rule.get("enabled", True):
        return False

    if not _tags_match(rule.get("tags_include") or [], m):
        return False

    # Spec ranges (min/max per dimension). Missing dimension = unbounded.
    spec = rule.get("spec") or {}
    for dim, field in _SPEC_FIELDS.items():
        rng = spec.get(dim) or {}
        lo = rng.get("min", 0) or 0
        hi = rng.get("max", 0) or 0
        if not lo and not hi:
            continue
        value = float(_get(m, field, 0) or 0)
        if not _in_range(value, lo, hi):
            return False

    # Renewal price ceiling on the chosen cycle (monthly|yearly).
    price_max = rule.get("price_max", 0) or 0
    if price_max:
        price = _rule_price(rule, m)
        if price is None or price > price_max:
            return False

    # Sale-price ceiling (independent of renewal cycle). Only some machines
    # carry a distinct sale price; when the cap is set and there's no sale
    # price the machine can't satisfy it.
    sale_max = rule.get("sale_price_max", 0) or 0
    if sale_max:
        sale = _get(m, "sale_price", None)
        if sale in (None, "") or float(sale) > sale_max:
            return False

    return True


def rank_candidates(
    machines: list[MachineSnapshot], score_rules: dict | None
) -> list[MachineSnapshot]:
    """Sort matched machines by hardware score DESCENDING.

    Scoring is decoupled from buying: the page's scoring rules (the same ones
    that drive the 评分 column) decide ordering, so the highest-scoring machine
    is bought first. Ties keep a stable order (cheapest monthly as tiebreak).
    """
    return sorted(
        machines,
        key=lambda m: (
            compute_score(m, score_rules),
            -(_get(m, "price_monthly", 0) or 0),
        ),
        reverse=True,
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
