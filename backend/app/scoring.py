"""
Per-page hardware scoring.

Each list page (cloud / ls / secondhand) carries an independent set of scoring
rules (``score_rules_cloud`` / ``score_rules_ls`` / ``score_rules_secondhand``
in settings). A rule is a flat map of ``{dimension: weight}``; the score of a
machine is the weighted sum of its dimension values:

    score = Σ (dimension_value × weight)

Dimensions and the units their weight applies to:

    cpu            per CPU core
    memory         per GB of RAM            (memory_mb / 1024)
    disk           per GB of disk
    bandwidth      per Mbps
    traffic        per GB of monthly traffic
    price_monthly  per ¥/month              (price_monthly fen / 100); usually
                                             a negative weight (cheaper = better)
    sale_price     per ¥                     (sale_price fen / 100); negative
    latency        per ms                    negative; SKIPPED when no data

``latency`` (and ``sale_price``) are only present on some machines. When a
machine has no value for a dimension that dimension is skipped — latency in
particular must never raise just because the field is missing.

The score is computed in the backend and attached to each machine in the
snapshot query response so the frontend can render and sort by it directly.
"""
from __future__ import annotations

from typing import Any

# Default weights, shared by all three pages unless a page overrides them.
# Positive = more is better; negative = less is better (price, latency).
DEFAULT_SCORE_RULES: dict[str, float] = {
    "cpu": 50.0,          # per core
    "memory": 20.0,       # per GB
    "disk": 1.0,          # per GB
    "bandwidth": 0.0,     # per Mbps
    "traffic": 0.0,       # per GB
    "price_monthly": -1.0,  # per ¥/month (negative: cheaper scores higher)
    "sale_price": -1.0,     # per ¥ (negative)
    "latency": -1.0,        # per ms (negative); skipped when no data
}

# Ordered list of weighting dimensions, used to build the settings UI.
SCORE_DIMENSIONS = list(DEFAULT_SCORE_RULES.keys())


def default_score_rules() -> dict[str, float]:
    """A fresh copy of the default weight map."""
    return dict(DEFAULT_SCORE_RULES)


def _get(machine: Any, key: str, default: Any = None) -> Any:
    """Read an attribute or dict key from a machine-like object."""
    if isinstance(machine, dict):
        return machine.get(key, default)
    return getattr(machine, key, default)


def _dimension_value(machine: Any, dim: str) -> float | None:
    """Resolve a machine's value for a scoring dimension.

    Returns ``None`` when the machine has no data for that dimension, in which
    case the caller skips it (this is what makes ``latency`` safe when absent).
    """
    if dim == "cpu":
        return float(_get(machine, "cpu", 0) or 0)
    if dim == "memory":
        return float(_get(machine, "memory_mb", 0) or 0) / 1024.0
    if dim == "disk":
        return float(_get(machine, "disk_gb", 0) or 0)
    if dim == "bandwidth":
        return float(_get(machine, "bandwidth_mbps", 0) or 0)
    if dim == "traffic":
        return float(_get(machine, "traffic_gb", 0) or 0)
    if dim == "price_monthly":
        return float(_get(machine, "price_monthly", 0) or 0) / 100.0
    if dim == "sale_price":
        # Only some (secondhand) machines carry a distinct sale price; treat a
        # missing field as "no data" so its weight doesn't apply spuriously.
        raw = _get(machine, "sale_price", None)
        if raw in (None, ""):
            return None
        return float(raw) / 100.0
    if dim == "latency":
        # Skip entirely when latency hasn't been measured.
        raw = _get(machine, "latency", None)
        if raw in (None, ""):
            return None
        return float(raw)
    # Unknown dimension: try a plain attribute read, else skip.
    raw = _get(machine, dim, None)
    if raw in (None, ""):
        return None
    try:
        return float(raw)
    except (TypeError, ValueError):
        return None


def compute_score(machine: Any, rules: dict[str, float] | None) -> float:
    """Weighted hardware score for a machine under a page's scoring rules.

    ``machine`` may be a MachineSnapshot, a plain dict, or anything exposing the
    standard fields. ``rules`` is a ``{dimension: weight}`` map; missing weights
    fall back to the defaults. Dimensions with no data on the machine (notably
    ``latency``) are skipped without error. Returns the rounded score.
    """
    merged = default_score_rules()
    if isinstance(rules, dict):
        for k, v in rules.items():
            try:
                merged[k] = float(v)
            except (TypeError, ValueError):
                continue

    total = 0.0
    for dim, weight in merged.items():
        if not weight:
            continue
        value = _dimension_value(machine, dim)
        if value is None:
            continue
        total += value * weight

    return round(total, 2)


def compute_star_rating(score: float) -> int:
    """Map a weighted hardware score to a 1–5 star rating.

    The mapping uses percentile-based thresholds calculated from typical store
    score distributions rather than hard-coded absolute ranges, so the stars
    adapt to the data naturally.  Falls back to a conservative linear mapping
    when the score is zero or negative.
    """
    if score <= 0:
        return 1
    # Linear mapping: 0..200 → 1..5 stars (clamped)
    # A score of ~200 corresponds to a strong machine (e.g. 4-core 8GB).
    raw = int((score / 200.0) * 4) + 1
    return min(max(raw, 1), 5)


def price_recommendation(score: float, price_monthly: int, price_yearly: int | None) -> dict | None:
    """Generate a pricing recommendation for a machine.

    Returns a dict with:
        - recommend: str — "monthly", "yearly", or "either"
        - reason: str — human-readable explanation in Chinese
        - savings_pct: float | None — percentage saved by yearly vs. monthly

    Returns None when neither price is available.
    """
    monthly = (price_monthly or 0) / 100.0   # fen → ¥
    yearly = (price_yearly or 0) / 100.0 if price_yearly is not None else 0

    if monthly <= 0 and yearly <= 0:
        return None

    result: dict = {"savings_pct": None}

    if monthly > 0 and yearly > 0:
        # Calculate effective monthly cost when paying yearly
        effective_monthly = yearly / 12.0
        savings = round((1 - effective_monthly / monthly) * 100, 1)

        if savings >= 15:
            result["recommend"] = "yearly"
            result["reason"] = f"年付相当于月付 ¥{effective_monthly:.1f}/月，节省 {savings:.0f}%"
            result["savings_pct"] = savings
        elif savings >= 5:
            result["recommend"] = "yearly"
            result["reason"] = f"年付略省 {savings:.0f}%，建议年付锁定价格"
            result["savings_pct"] = savings
        elif savings <= -5:
            result["recommend"] = "monthly"
            result["reason"] = "月付更灵活，年付无明显折扣"
        else:
            result["recommend"] = "either"
            result["reason"] = "月付与年付价格接近，按需选择"
    elif monthly > 0:
        result["recommend"] = "monthly"
        result["reason"] = f"仅支持月付 ¥{monthly:.2f}/月"
    elif yearly > 0:
        result["recommend"] = "yearly"
        result["reason"] = f"仅支持年付 ¥{yearly:.2f}/年（¥{yearly/12:.2f}/月）"
    else:
        return None

    # Boost recommendation with star context
    if score > 100:
        result["reason"] += "，性价比优秀"
    elif score > 50:
        result["reason"] += "，性价比良好"

    return result
