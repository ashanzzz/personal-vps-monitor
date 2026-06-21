"""
Match secondhand listings against equivalent brand-new official plans.

Given a secondhand MachineSnapshot and the set of currently-available official
plans (vps/ls), find the closest equivalent new machine on the SAME route/area
with the SAME core spec, so the UI can warn "there's an official new one of this
exact config" and show a price comparison.

This runs on the backend; the frontend only renders the precomputed result.
"""
from __future__ import annotations

from dataclasses import dataclass


def _as_fen_int(raw) -> int:
    if raw in (None, "") or isinstance(raw, bool):
        return 0
    try:
        return int(raw)
    except (TypeError, ValueError):
        return 0


# Bandwidth tolerance: treat plans within one "tier" as equivalent. We bucket by
# common Akile tiers (Mbps) so 30 vs 50 don't collapse but 1000 vs 1000 match.
def _bw_tier(mbps: int) -> int:
    if mbps <= 0:
        return 0
    for i, ceiling in enumerate([50, 100, 200, 500, 1000, 2000, 5000, 10000]):
        if mbps <= ceiling:
            return i
    return 99


def _normalize_area(area: str) -> str:
    """Compare routes loosely: strip whitespace, lowercase, drop punctuation."""
    return "".join(ch for ch in (area or "").lower() if ch.isalnum())


@dataclass
class OfficialMatch:
    plan_id: str
    name: str
    area: str
    price_monthly: int
    available: bool
    price_delta: int   # official_monthly - secondhand_monthly (fen). + = official pricier
    cheaper: str       # "official" | "secondhand" | "equal"

    def as_dict(self) -> dict:
        return {
            "plan_id": self.plan_id,
            "name": self.name,
            "area": self.area,
            "price_monthly": self.price_monthly,
            "available": self.available,
            "price_delta": self.price_delta,
            "cheaper": self.cheaper,
            "buy_url": "https://akile.ai/store",
        }


def _specs_match(sh, off) -> bool:
    """Same route + same core spec (cpu/mem/disk exact, bandwidth within a tier)."""
    if _normalize_area(sh.area) != _normalize_area(off.area):
        return False
    if sh.cpu != off.cpu:
        return False
    if sh.memory_mb != off.memory_mb:
        return False
    if sh.disk_gb != off.disk_gb:
        return False
    if _bw_tier(sh.bandwidth_mbps) != _bw_tier(off.bandwidth_mbps):
        return False
    return True


def find_official_equivalent(secondhand, official_machines: list) -> dict | None:
    """Return the best official equivalent for a secondhand machine, or None.

    Prefers an available (in-stock) match; among candidates picks the cheapest.
    `official_machines` should be vps/ls MachineSnapshot rows.
    """
    candidates = [m for m in official_machines if _specs_match(secondhand, m)]
    if not candidates:
        return None

    # Prefer in-stock, then lowest monthly price.
    candidates.sort(key=lambda m: (not m.available, m.price_monthly or 10**9))
    best = candidates[0]

    sh_price = _as_fen_int(secondhand.price_monthly)
    off_price = _as_fen_int(best.price_monthly)
    delta = off_price - sh_price
    if delta > 0:
        cheaper = "secondhand"
    elif delta < 0:
        cheaper = "official"
    else:
        cheaper = "equal"

    return OfficialMatch(
        plan_id=best.plan_id,
        name=best.name,
        area=best.area,
        price_monthly=off_price,
        available=best.available,
        price_delta=delta,
        cheaper=cheaper,
    ).as_dict()
