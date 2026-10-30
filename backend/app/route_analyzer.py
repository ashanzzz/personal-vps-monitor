"""
Route Analyzer & Traffic Valuation Engine for VPS & Cloud Instances.

Provides:
  1. Route & network tier identification (S/A/B/C) based on keywords in name, node, tags, comments.
  2. IP attribute detection (Native IP, Clean vs Blocked/Banned, Dual-stack).
  3. Traffic health & depletion evaluation (Remaining GB, Remaining Ratio, Depleted Flag).
  4. Secondhand intrinsic valuation & Sniping Guard (Real Total Cost with Reset Penalty).
"""
from __future__ import annotations

import re
from typing import Any, Dict, List, Optional


# ---------------------------------------------------------------------------
# Route Knowledge Base & Patterns
# ---------------------------------------------------------------------------

ROUTE_DEFINITIONS = [
    # --- S Tier: Premium Tier-1 Direct CN Routing ---
    {
        "id": "cn2_gia",
        "tier": "S",
        "tier_name": "S级·双程精品",
        "isp": "中国电信",
        "line_name": "CN2 GIA (AS4809)",
        "patterns": [r"cn2[\s_\-]*gia", r"as4809", r"gia"],
        "weight": 100,
    },
    {
        "id": "as9929",
        "tier": "S",
        "tier_name": "S级·联通精品",
        "isp": "中国联通",
        "line_name": "AS9929 / CU Premium",
        "patterns": [r"9929", r"as9929", r"cu9929", r"nus[\s_\-]*9929"],
        "weight": 95,
    },
    {
        "id": "cmin2",
        "tier": "S",
        "tier_name": "S级·移动精品",
        "isp": "中国移动",
        "line_name": "CMIN2 (AS58807)",
        "patterns": [r"cmin2", r"as58807"],
        "weight": 92,
    },

    # --- A Tier: High-Quality Direct & Regional Optimization ---
    {
        "id": "as4837",
        "tier": "A",
        "tier_name": "A级·联通骨干直连",
        "isp": "中国联通",
        "line_name": "AS4837 / CU169",
        "patterns": [r"4837", r"as4837", r"cu169"],
        "weight": 85,
    },
    {
        "id": "softbank",
        "tier": "A",
        "tier_name": "A级·日本软银",
        "isp": "Softbank",
        "line_name": "日本软银 (BBTEC/JPSB)",
        "patterns": [r"softbank", r"软银", r"bbtec", r"jpsb\d*", r"sb\d*"],
        "weight": 82,
    },
    {
        "id": "iij",
        "tier": "A",
        "tier_name": "A级·日本IIJ",
        "isp": "IIJ",
        "line_name": "日本 IIJ 直连优化",
        "patterns": [r"iij", r"jpiij"],
        "weight": 80,
    },
    {
        "id": "cmi",
        "tier": "A",
        "tier_name": "A级·移动直连",
        "isp": "中国移动",
        "line_name": "CMI (AS58453)",
        "patterns": [r"\bcmi\b", r"as58453"],
        "weight": 78,
    },
    {
        "id": "ctgnet",
        "tier": "A",
        "tier_name": "A级·电信CTGNet",
        "isp": "中国电信",
        "line_name": "CTGNet / 混合GIA",
        "patterns": [r"ctgnet", r"ctg\b"],
        "weight": 82,
    },

    # --- B Tier: Asia-Pacific & Regional Direct BGP ---
    {
        "id": "ap_bgp",
        "tier": "B",
        "tier_name": "B级·亚太直连BGP",
        "isp": "三网优化",
        "line_name": "亚太优化 BGP (HK/SG/TW/KR/JP)",
        "patterns": [r"hkbgp", r"sgbgp", r"twbgp", r"krbgp", r"jpbgp", r"优化回国", r"三网优化"],
        "weight": 65,
    },

    # --- C Tier: Standard Global BGP / Non-optimized ---
    {
        "id": "std_bgp",
        "tier": "C",
        "tier_name": "C级·标准国际骨干",
        "isp": "国际骨干",
        "line_name": "普通国际 BGP (Cogent/HE/Telia/Lumen)",
        "patterns": [r"hyper", r"std", r"standard", r"bgp", r"cogent", r"he\.net"],
        "weight": 40,
    },
]


def analyze_route(
    name: str = "",
    node_name: str = "",
    detail: str = "",
    intro: str = "",
    area: str = "",
) -> Dict[str, Any]:
    """
    Extract route classification, ISP tags, and IP cleanliness from listing texts.
    """
    corpus = f"{name} {node_name} {detail} {intro} {area}".lower()

    matched_routes: List[Dict[str, Any]] = []
    highest_tier = "C"
    tier_order = {"S": 4, "A": 3, "B": 2, "C": 1}

    for defn in ROUTE_DEFINITIONS:
        for p in defn["patterns"]:
            if re.search(p, corpus):
                matched_routes.append(defn)
                if tier_order[defn["tier"]] > tier_order[highest_tier]:
                    highest_tier = defn["tier"]
                break

    # If no specific route matched, default to C
    if not matched_routes:
        matched_routes.append(ROUTE_DEFINITIONS[-1])

    # Distinct route names
    route_names = [r["line_name"] for r in matched_routes if r["id"] != "std_bgp"]
    if not route_names:
        route_names = ["标准国际互联"]

    # IP Cleanliness Check
    # Look for negative signals: 被墙, 阻断, blocked, banned, 墙
    is_blocked = bool(re.search(r"被墙|阻断|blocked|banned|ip被墙|墙", corpus))
    is_native = bool(re.search(r"原生|native|住宅|isp", corpus))

    tier_labels = {
        "S": "S级·顶级精品",
        "A": "A级·优质优化",
        "B": "B级·亚太直连",
        "C": "C级·普通国际",
    }

    return {
        "tier": highest_tier,
        "tier_label": tier_labels.get(highest_tier, "标准"),
        "is_optimized": highest_tier in ("S", "A"),
        "routes": route_names,
        "primary_route": route_names[0],
        "native_ip": is_native,
        "ip_clean": not is_blocked,
        "ip_status": "blocked" if is_blocked else ("native" if is_native else "clean"),
        "ip_status_text": "IP被墙⚠️" if is_blocked else ("原生IP✅" if is_native else "IP正常"),
    }


# ---------------------------------------------------------------------------
# Traffic Health & Depletion Analysis
# ---------------------------------------------------------------------------

def analyze_traffic(
    flow_total: float,
    flow_used: float,
    reset_price_cny: float = 0.0,
) -> Dict[str, Any]:
    """
    Compute traffic remaining, usage ratio, and depletion status.
    """
    flow_total = max(0.0, float(flow_total or 0))
    flow_used = max(0.0, float(flow_used or 0))

    remaining = max(0.0, flow_total - flow_used)
    ratio = round((remaining / flow_total) * 100, 1) if flow_total > 0 else 100.0

    # Depletion rule: <= 5GB left or <= 1% remaining
    is_depleted = (remaining <= 5.0) or (flow_total > 0 and ratio <= 1.0)
    is_low = (not is_depleted) and (ratio <= 20.0)

    if is_depleted:
        status = "depleted"
        status_text = "流量已耗尽 🛑"
        badge_tone = "error"
    elif is_low:
        status = "low"
        status_text = f"余量偏低 ({ratio}%) ⚠️"
        badge_tone = "warning"
    else:
        status = "healthy"
        status_text = f"余量充足 ({ratio}%) ✅"
        badge_tone = "success"

    return {
        "total_gb": flow_total,
        "used_gb": round(flow_used, 1),
        "remaining_gb": round(remaining, 1),
        "remaining_ratio_percent": ratio,
        "is_depleted": is_depleted,
        "is_low": is_low,
        "status": status,
        "status_text": status_text,
        "badge_tone": badge_tone,
        "reset_price_cny": round(reset_price_cny, 2),
    }


# ---------------------------------------------------------------------------
# Comprehensive Sniping & Intrinsic Valuation Model
# ---------------------------------------------------------------------------

def evaluate_secondhand_listing(
    price_cny: float,
    renew_price_cny: float,
    flow_total_gb: float,
    flow_used_gb: float,
    reset_price_cny: float = 0.0,
    name: str = "",
    node_name: str = "",
    detail: str = "",
    intro: str = "",
    area: str = "",
    days_left: int = 30,
) -> Dict[str, Any]:
    """
    Full multidimensional evaluation for secondhand arbitrage and sniping safety.
    """
    route_info = analyze_route(name=name, node_name=node_name, detail=detail, intro=intro, area=area)
    traffic_info = analyze_traffic(flow_total_gb, flow_used_gb, reset_price_cny)

    price_cny = max(0.0, float(price_cny or 0))
    renew_price_cny = max(0.0, float(renew_price_cny or 0))

    # Real acquisition cost = asking price + reset traffic fee (if depleted)
    traffic_penalty = traffic_info["reset_price_cny"] if traffic_info["is_depleted"] else 0.0
    real_total_cost = round(price_cny + traffic_penalty, 2)

    # Intrinsic value baseline
    # S-tier: high intrinsic value (0.8x renewal price + native bonus)
    tier_multipliers = {"S": 0.85, "A": 0.70, "B": 0.50, "C": 0.35}
    base_multiplier = tier_multipliers.get(route_info["tier"], 0.35)

    native_bonus = 3.0 if route_info["native_ip"] else 0.0
    intrinsic_value = max(3.0, round(renew_price_cny * base_multiplier + native_bonus, 2))

    # Sniping safety guard checks
    snipable = True
    block_reasons: List[str] = []

    if not route_info["ip_clean"]:
        snipable = False
        block_reasons.append("IP已被阻断/被墙，严禁买入")

    if traffic_info["is_depleted"]:
        # If traffic is 0 and real total cost >= 70% of renew price
        if real_total_cost >= renew_price_cny * 0.7 and renew_price_cny > 0:
            snipable = False
            block_reasons.append(f"流量已用光(0G/重置需¥{traffic_penalty})，加上重置费后总成本¥{real_total_cost}过高，缺乏套利空间")
        else:
            block_reasons.append(f"注意：流量已耗尽(需¥{traffic_penalty}重置)")

    if route_info["tier"] == "C" and price_cny > 10.0:
        snipable = False
        block_reasons.append("普通非优化线路，转售价超标缺乏捡漏价值")

    # Score calculation (0 to 100)
    score = 50.0
    if route_info["tier"] == "S":
        score += 30.0
    elif route_info["tier"] == "A":
        score += 20.0
    elif route_info["tier"] == "B":
        score += 10.0

    if route_info["native_ip"]:
        score += 10.0
    if not route_info["ip_clean"]:
        score -= 40.0

    # Traffic contribution
    if traffic_info["is_depleted"]:
        score -= 25.0
    elif traffic_info["is_low"]:
        score -= 10.0
    else:
        score += 10.0

    # Discount score
    if renew_price_cny > 0 and price_cny > 0:
        discount = price_cny / renew_price_cny
        if discount <= 0.3:
            score += 20.0
        elif discount <= 0.5:
            score += 10.0
        elif discount > 0.9:
            score -= 15.0

    final_score = max(0, min(100, round(score, 1)))

    return {
        "route": route_info,
        "traffic": traffic_info,
        "pricing": {
            "ask_price_cny": price_cny,
            "renew_price_cny": renew_price_cny,
            "reset_traffic_fee_cny": traffic_penalty,
            "real_total_cost_cny": real_total_cost,
            "intrinsic_value_cny": intrinsic_value,
        },
        "sniping_verdict": {
            "is_snipable": snipable and (final_score >= 65),
            "score": final_score,
            "reasons": block_reasons,
            "verdict_text": "【优质捡漏】" if (snipable and final_score >= 75) else ("【可入手】" if (snipable and final_score >= 65) else "【拦截避坑】"),
        }
    }
