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
    Full multidimensional evaluation for secondhand arbitrage, long-term asset value, and sniping safety.
    """
    route_info = analyze_route(name=name, node_name=node_name, detail=detail, intro=intro, area=area)
    traffic_info = analyze_traffic(flow_total_gb, flow_used_gb, reset_price_cny)

    price_cny = max(0.0, float(price_cny or 0))
    renew_price_cny = max(0.0, float(renew_price_cny or 0))
    monthly_renew = renew_price_cny if renew_price_cny > 0 else price_cny

    # Check if budget/perpetual asset (low entry price or low recurring monthly fee)
    # 核心：流量是每月自动重置刷新的资产。如果价格够低，当期流量用尽可忽略，因为我们要的是长远价值！
    is_budget_asset = (monthly_renew <= 10.0) or (price_cny <= 10.0)

    # For budget assets, buyers hold for the perpetual monthly quota (waits for monthly cycle reset),
    # so immediate traffic reset penalty is OPTIONAL and NOT forced into acquisition cost.
    traffic_penalty = 0.0 if is_budget_asset else (traffic_info["reset_price_cny"] if traffic_info["is_depleted"] else 0.0)
    real_total_cost = round(price_cny + traffic_penalty, 2)

    # Intrinsic value baseline
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
        if not is_budget_asset:
            # High cost machine depleted of traffic
            if real_total_cost >= renew_price_cny * 0.75 and renew_price_cny > 0:
                snipable = False
                block_reasons.append(f"中高价位机型流量用尽(重置需¥{traffic_info['reset_price_cny']})，到手成本¥{real_total_cost}偏高")
        else:
            block_reasons.append("当期流量已用尽（低持有成本机型，静待月度重置满血复活）")

    if route_info["tier"] == "C" and price_cny > 15.0:
        snipable = False
        block_reasons.append("普通非优化线路，转售价超标缺乏捡漏价值")

    # Cost-effectiveness evaluation
    cost_eff = evaluate_cost_effectiveness(
        item_type="secondhand",
        price_cny=price_cny,
        renew_price_cny=renew_price_cny,
        cpu=1,
        memory_mb=1024,
        disk_gb=10,
        bandwidth_mbps=200,
        flow_total_gb=flow_total_gb,
        flow_used_gb=flow_used_gb,
        reset_price_cny=reset_price_cny,
        name=name,
        node_name=node_name,
        detail=detail,
        intro=intro,
        area=area,
    )

    final_score = cost_eff["score"]

    return {
        "route": route_info,
        "traffic": traffic_info,
        "pricing": {
            "ask_price_cny": price_cny,
            "renew_price_cny": renew_price_cny,
            "reset_traffic_fee_cny": traffic_penalty,
            "real_total_cost_cny": real_total_cost,
            "intrinsic_value_cny": intrinsic_value,
            "is_budget_asset": is_budget_asset,
        },
        "cost_effectiveness": cost_eff,
        "sniping_verdict": {
            "is_snipable": snipable and (final_score >= 65),
            "score": cost_eff["score"],
            "grade_label": cost_eff["grade_label"],
            "reasons": block_reasons if block_reasons else cost_eff["reasons"],
            "verdict_text": cost_eff["grade_label"],
        }
    }


# ---------------------------------------------------------------------------
# General Cost-Effectiveness Evaluation Engine (一手与二手通用性价比算法)
# ---------------------------------------------------------------------------

def evaluate_cost_effectiveness(
    item_type: str = "store",  # "store" (一手) | "secondhand" (二手) | "server" (已有名下主机)
    price_cny: float = 0.0,
    renew_price_cny: float = 0.0,
    cpu: int = 1,
    memory_mb: int = 1024,
    disk_gb: int = 10,
    bandwidth_mbps: int = 100,
    flow_total_gb: float = 1000.0,
    flow_used_gb: float = 0.0,
    reset_price_cny: float = 20.0,
    name: str = "",
    node_name: str = "",
    detail: str = "",
    intro: str = "",
    area: str = "",
) -> Dict[str, Any]:
    """
    Multidimensional cost-effectiveness scoring engine (0 - 100).
    Values long-term recurring efficiency and cyclical quota replenishing.

    Dimensions:
      1. Network & Routing Tier (0-30 points)
      2. Long-Term Recurring Value / LTV (0-25 points)
      3. Cyclical Traffic Quota & Reset Health (0-25 points)
      4. Pricing & Entry Barrier (0-20 points)
    """
    route_info = analyze_route(name=name, node_name=node_name, detail=detail, intro=intro, area=area)
    traffic_info = analyze_traffic(flow_total_gb, flow_used_gb, reset_price_cny)

    price_cny = max(0.1, float(price_cny or 0))
    renew_price_cny = max(0.0, float(renew_price_cny or 0))
    monthly_renew = renew_price_cny if (item_type == "secondhand" and renew_price_cny > 0) else price_cny
    monthly_renew = max(0.1, monthly_renew)
    reasons: List[str] = []

    # 1. Network & Routing Tier (0 - 30 points)
    network_score = 8.0
    tier = route_info["tier"]
    if tier == "S":
        network_score = 30.0
        reasons.append(f"{route_info['primary_route']} (S级精品线路 +30)")
    elif tier == "A":
        network_score = 22.0
        reasons.append(f"{route_info['primary_route']} (A级优化直连 +22)")
    elif tier == "B":
        network_score = 15.0
        reasons.append("亚太直连BGP (+15)")
    else:
        network_score = 8.0
        reasons.append("普通国际BGP (+8)")

    if route_info["native_ip"]:
        network_score = min(30.0, network_score + 4.0)
        reasons.append("原生IP认证 (+4)")

    if not route_info["ip_clean"]:
        network_score = -30.0
        reasons.append("IP阻断/被墙严重警告 (-50)")

    # 2. Long-Term Recurring Value / LTV (0 - 25 points)
    # 核心：VPS是长期订阅资产，月续费越低，全生命周期成本越低，传家宝价值越高！
    ltv_score = 10.0
    if monthly_renew <= 2.0:
        ltv_score = 25.0
        reasons.append(f"月供仅¥{monthly_renew:.2f}/月 (绝版传家宝，长期持有近乎零成本 +25)")
    elif monthly_renew <= 5.0:
        ltv_score = 22.0
        reasons.append(f"超低月供¥{monthly_renew:.2f}/月 (极高长线续费价值 +22)")
    elif monthly_renew <= 12.0:
        ltv_score = 18.0
        reasons.append(f"亲民低月供¥{monthly_renew:.2f}/月 (+18)")
    elif monthly_renew <= 25.0:
        ltv_score = 14.0
    elif monthly_renew <= 50.0:
        ltv_score = 10.0
    else:
        ltv_score = 5.0
        reasons.append("月供偏高，长期持有成本显著 (-5)")

    # Hardware Output Bonus (+1~2 points)
    ram_gb = max(0.25, memory_mb / 1024)
    if cpu >= 2 or ram_gb >= 2.0 or bandwidth_mbps >= 500:
        ltv_score = min(25.0, ltv_score + 2.0)

    # 3. Cyclical Traffic Quota & Reset Health (0 - 25 points)
    # 核心：流量是每个月自动重置刷新的资产，重点看每月供给额度与单价效率
    traffic_score = 10.0
    remaining_gb = max(0.0, flow_total_gb - flow_used_gb)
    is_depleted = (flow_total_gb > 0 and remaining_gb <= 1.0)
    quota_efficiency = flow_total_gb / monthly_renew if monthly_renew > 0 else 50.0
    is_budget_asset = (monthly_renew <= 10.0) or (price_cny <= 10.0)

    if flow_total_gb <= 0:
        traffic_score = 25.0
        reasons.append("不限流量无限带宽 (+25)")
    elif is_budget_asset:
        # 低持有成本机型：每月自动满血重置，当期用尽不损长远价值！
        if quota_efficiency >= 100:
            traffic_score = 23.0
            reasons.append(f"长效配额极高({int(quota_efficiency)}G/元·月，每月自动满血重置 +23)")
        else:
            traffic_score = 18.0
            reasons.append(f"周期配额{int(flow_total_gb)}G按月自动刷新 (+18)")
        if is_depleted:
            reasons.append("提示：当期流量已用尽，但持有成本极低，静待月度重置即可")
    else:
        # 高持有成本机型：需考虑当期即时可用性
        if is_depleted:
            traffic_score = 8.0
            reasons.append(f"当期流量已耗尽(重置需¥{reset_price_cny}) ⚠️")
        elif remaining_gb / flow_total_gb < 0.2:
            traffic_score = 12.0
            reasons.append("当期剩余流量偏低 ⚠️")
        else:
            traffic_score = 20.0
            reasons.append(f"当期流量充沛({int(remaining_gb)}G可用 +20)")

    # 4. Pricing & Entry Barrier (0 - 20 points)
    price_score = 10.0
    if item_type == "secondhand":
        if is_budget_asset and price_cny <= 10.0:
            price_score = 20.0
            reasons.append(f"入场成本仅¥{price_cny:.2f} (极低试错成本与流动性 +20)")
        elif renew_price_cny > 0:
            discount = price_cny / renew_price_cny
            if discount <= 0.3:
                price_score = 20.0
                reasons.append(f"大额骨折转让({int(discount*100)}%折 +20)")
            elif discount <= 0.6:
                price_score = 15.0
                reasons.append(f"半价实惠转让({int(discount*100)}%折 +15)")
            elif discount > 1.3:
                price_score = 5.0
                reasons.append("溢价过户转让 (-5)")
            else:
                price_score = 10.0
    else:
        if price_cny <= 5.0:
            price_score = 20.0
            reasons.append("低至5元内极限底价 (+20)")
        elif price_cny <= 15.0:
            price_score = 16.0
            reasons.append("百元内亲民低月付 (+16)")
        elif price_cny <= 30.0:
            price_score = 12.0
        else:
            price_score = 8.0

    raw_total = network_score + ltv_score + traffic_score + price_score
    final_score = max(0, min(100, int(round(raw_total))))

    if not route_info["ip_clean"]:
        grade = "D"
        grade_label = "【拦截避坑】"
        badge_color = "red"
    elif final_score >= 88:
        grade = "SSS"
        grade_label = "【神机/必抢】"
        badge_color = "purple"
    elif final_score >= 75:
        grade = "A"
        grade_label = "【超值高性价】"
        badge_color = "green"
    elif final_score >= 60:
        grade = "B"
        grade_label = "【平价可选】"
        badge_color = "blue"
    else:
        grade = "C"
        grade_label = "【性价比偏低】"
        badge_color = "orange"

    return {
        "score": final_score,
        "grade": grade,
        "grade_label": grade_label,
        "badge_color": badge_color,
        "breakdown": {
            "network_score": max(0, int(network_score)),
            "ltv_score": int(ltv_score),
            "traffic_score": int(traffic_score),
            "price_score": int(price_score),
        },
        "reasons": reasons,
    }
