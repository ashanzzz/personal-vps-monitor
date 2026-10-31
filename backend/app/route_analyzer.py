"""
Route Analyzer & Traffic Valuation Engine for VPS & Cloud Instances.

Provides:
  1. Route & network tier identification (S/A/B/C) based on keywords in name, node, tags, comments.
  2. IP attribute detection (Native IP, Clean vs Blocked/Banned, Dual-stack).
  3. Traffic health & depletion evaluation (Remaining GB, Remaining Ratio, Depleted Flag).
  4. Secondhand intrinsic valuation & Sniping Guard (Real Total Cost with Reset Penalty).
"""
from __future__ import annotations

import math
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

    # IP更换官方规则 (ACCK 年付¥15 / 月付¥35；Akile ¥35 或 1500 AK币；IPv6免费)
    # 被墙绝非死刑：只要线路和流量好，花¥15~¥35换新IP即可满血复活
    ip_repair_fee = 0.0
    if not route_info["ip_clean"]:
        # 估算换IP费用: 15 (年付) 或 35 (月付)
        is_yearly = ("年" in intro or "year" in intro.lower() or "年付" in detail or (renew_price_cny > 0 and renew_price_cny >= 100))
        ip_repair_fee = 15.0 if is_yearly else 35.0

    real_total_cost = round(price_cny + traffic_penalty + ip_repair_fee, 2)

    # Intrinsic value baseline
    tier_multipliers = {"S": 0.85, "A": 0.70, "B": 0.50, "C": 0.35}
    base_multiplier = tier_multipliers.get(route_info["tier"], 0.35)

    native_bonus = 3.0 if route_info["native_ip"] else 0.0
    intrinsic_value = max(3.0, round(renew_price_cny * base_multiplier + native_bonus, 2))

    # Sniping safety guard checks
    snipable = True
    block_reasons: List[str] = []

    if not route_info["ip_clean"]:
        block_reasons.append(f"当前IP阻断(官方支持换新IP，费用约¥{int(ip_repair_fee)}或1500AK币，换新后满血复活)")

    if traffic_info["is_depleted"]:
        if not is_budget_asset:
            if real_total_cost >= renew_price_cny * 0.85 and renew_price_cny > 0:
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
            "ip_repair_fee_cny": ip_repair_fee,
            "real_total_cost_cny": real_total_cost,
            "intrinsic_value_cny": intrinsic_value,
            "is_budget_asset": is_budget_asset,
        },
        "cost_effectiveness": cost_eff,
        "sniping_verdict": {
            "is_snipable": snipable and (final_score >= 60),
            "score": cost_eff["score"],
            "grade_label": cost_eff["grade_label"],
            "reasons": block_reasons if block_reasons else cost_eff["reasons"],
            "verdict_text": cost_eff["grade_label"],
        }
    }


# ---------------------------------------------------------------------------
# General Cost-Effectiveness Evaluation Engine (一手与二手通用性价比算法)
# ---------------------------------------------------------------------------

def match_user_trade_strategy(
    name: str = "",
    node_name: str = "",
    detail: str = "",
    intro: str = "",
    monthly_renew: float = 0.0,
) -> Optional[Dict[str, Any]]:
    """
    山哥专属主力核心机型做市报价与心理底牌字典：
    1. IIJ (Akile JP IIJ 旗舰大口径高配生产力): 买入成本 ¥350，出售底价 ¥500，买入目标 <= ¥200
    2. ISP (ACCK LAX4837-ISP 绝版双ISP传家宝): 目标售价 ¥400，买入目标 ¥200 - ¥250
    3. SB (ACCK JPSB 绝版软银 1.11元传家宝): 目标售价 ¥250 - ¥300，买入目标 ¥100 - ¥170
    """
    combined = f"{name} {node_name} {detail} {intro}".lower()

    # 1. IIJ 旗舰机型 (必须是旗舰高配或特配，绝非普通JPIIJ-Lite等常规路人款)
    if "iij" in combined and ("pro" in combined or "2000" in combined or "bgp" in combined or "旗舰" in combined or ("lite" not in combined and monthly_renew <= 15.0)):
        return {
            "model_code": "IIJ",
            "model_name": "Akile JP IIJ 旗舰生产力",
            "buy_target_max": 200.0,
            "sell_target_min": 500.0,
            "cost_basis": 350.0,
            "desc": "性能极强生产力底座(2.5G口+2G内存+20G盘)，成本¥350，非¥500不卖，¥200内闭眼入",
        }

    # 2. ISP 绝版双ISP传家宝 (月付1.90元)
    if ("isp" in combined or "双isp" in combined) and monthly_renew <= 2.5:
        return {
            "model_code": "ISP",
            "model_name": "ACCK LAX4837-ISP 绝版双ISP传家宝",
            "buy_target_max": 250.0,
            "sell_target_min": 400.0,
            "cost_basis": 1.90,
            "desc": "美西双ISP绝版传家宝(1000M口+1200G)，买入目标¥200-250，目标售价¥400",
        }

    # 3. SB 绝版软银传家宝 (月付1.11元)
    if ("sb" in combined or "softbank" in combined or "软银" in combined) and monthly_renew <= 2.5:
        return {
            "model_code": "SB",
            "model_name": "ACCK JPSB 绝版软银传家宝",
            "buy_target_max": 170.0,
            "sell_target_min": 250.0,
            "cost_basis": 1.11,
            "desc": "日本软银绝版传家宝(1000M口+1000G)，买入目标¥100-170，目标售价¥250-300",
        }

    return None


def evaluate_cost_effectiveness(
    item_type: str = "store",  # "store" (一手) | "secondhand" (二手) | "server" (已有名下主机)
    price_cny: float = 0.0,
    renew_price_cny: float = 0.0,
    cpu: int = 1,
    memory_mb: float = 1024.0,
    disk_gb: float = 10.0,
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
    Values: 1. Route Quality (35%), 2. Traffic Quota (30%), 3. Specs & LTV (20%), 4. Pricing (15%).
    Blocked IPs are treatable via official IP replacement (¥15~¥35 or 1500 AK coins).
    """
    route_info = analyze_route(name=name, node_name=node_name, detail=detail, intro=intro, area=area)
    traffic_info = analyze_traffic(flow_total_gb, flow_used_gb, reset_price_cny)

    # Normalize hardware units safely (ACCK passes disk in MB, e.g. 5120MB -> 5GB)
    disk_gb = float(disk_gb or 10.0)
    if disk_gb >= 100.0:
        disk_gb = round(disk_gb / 1024.0, 1)
    disk_gb = max(1.0, disk_gb)

    memory_mb = float(memory_mb or 1024.0)
    if memory_mb < 64.0:
        memory_mb = memory_mb * 1024.0
    memory_mb = max(128.0, memory_mb)

    price_cny = max(0.1, float(price_cny or 0))
    renew_price_cny = max(0.0, float(renew_price_cny or 0))
    monthly_renew = renew_price_cny if (item_type == "secondhand" and renew_price_cny > 0) else price_cny
    monthly_renew = max(0.1, monthly_renew)
    reasons: List[str] = []

    # -------------------------------------------------------------
    # 山哥逆向推理锚点法则 (Reverse-Inference Calibration Architecture)
    # 锚点 1: 垃圾 = 0 分 (Junk / 致命避坑 / IP阻断且高续费 / 无法使用)
    # 锚点 2: 路人 = 50 分 (Passerby / 无明显折扣的日常在售套餐或公允二手，48~55分中位数)
    # 锚点 3: 我的 IIJ = 80 分 (Flagship / 2.5G大口径+2G大内存+20G盘+直连优化主力生产力底座)
    # 锚点 4: 我的传家宝 = 100 分 (Absolute Heirloom / 月供<=2元零持有成本+好线路/原生双ISP)
    # -------------------------------------------------------------
    ram_gb = max(0.25, memory_mb / 1024.0)
    is_high_spec_powerhouse = (ram_gb >= 2.0 and disk_gb >= 20.0 and bandwidth_mbps >= 1000 and flow_total_gb >= 1000)

    tier = str(route_info.get("tier") or "C")
    combined = f"{name} {node_name} {detail} {intro}".lower()
    is_dual_isp = ("isp" in combined or "双isp" in combined) and bool(route_info.get("native_ip"))
    is_native = bool(route_info.get("native_ip"))
    ip_clean = bool(route_info.get("ip_clean"))

    is_junk_trap = False
    final_score = 50

    # 1. 垃圾判定 (Junk Trap -> 0分)
    if not ip_clean:
        if monthly_renew >= 20.0 or tier == "C":
            is_junk_trap = True
            final_score = 0
            reasons.append("【垃圾避坑】IP阻断且高续费/劣质线路，换IP成本倒挂且无使用价值 (0分)")
    if monthly_renew > 60.0 and tier in ("B", "C") and not is_high_spec_powerhouse:
        is_junk_trap = True
        final_score = 0
        reasons.append(f"【垃圾避坑】劣质线路天价月租¥{monthly_renew:.2f}/月严重失血 (0分)")

    # 2. 传家宝判定 (Heirloom -> 100分)
    if not is_junk_trap and monthly_renew <= 2.0 and ip_clean:
        if tier in ("S", "A") or is_dual_isp:
            final_score = 100
            reasons.append(f"【顶级绝版传家宝】月供仅¥{monthly_renew:.2f}/月 + 稀缺优质属性，无脑顶格公认神机 (100分)")
        else:
            final_score = 90
            reasons.append(f"【绝版传家宝】月供仅¥{monthly_renew:.2f}/月近乎零持有成本 (90分)")

    # 3. 我的 IIJ 旗舰生产力判定 (Flagship Productivity -> 80分)
    elif not is_junk_trap and is_high_spec_powerhouse and tier in ("S", "A") and monthly_renew <= 25.0 and ip_clean:
        final_score = 80
        reasons.append(f"【生产力旗舰底座】大带宽({bandwidth_mbps}M)+大内存({int(ram_gb)}G)+大硬盘({int(disk_gb)}G)+优质直连，主力生产力 (80分)")

    # 4. 常规路人模型 (Passerby Baseline -> 围绕 50 分基准展开)
    elif not is_junk_trap:
        score = 50.0

        # 线路溢折价
        tier_deltas = {"S": 10.0, "A": 4.0, "B": -1.0, "C": -6.0}
        delta_t = tier_deltas.get(tier, 0.0)
        score += delta_t
        if delta_t != 0:
            reasons.append(f"线路等级: {tier}级 ({delta_t:+0.0f}分)")

        # IP 稀缺性
        if is_dual_isp:
            score += 6.0
            reasons.append("原生双ISP住宅属性 (+6分)")
        elif is_native:
            score += 2.0
            reasons.append("原生单IP (+2分)")

        # 月供经济性 (以 15~20 元常规路人平价为 0 基准)
        if monthly_renew <= 5.0:
            score += 15.0
            reasons.append(f"超低月供¥{monthly_renew:.2f}/月 (+15分)")
        elif monthly_renew <= 10.0:
            score += 6.0
            reasons.append(f"超值月供¥{monthly_renew:.2f}/月 (+6分)")
        elif monthly_renew <= 20.0:
            score += 0.0
            reasons.append(f"平价月供¥{monthly_renew:.2f}/月 (常规路人价位 +0分)")
        elif monthly_renew <= 25.0:
            score -= 3.0
            reasons.append(f"月供¥{monthly_renew:.2f}/月略偏高 (-3分)")
        elif monthly_renew <= 35.0:
            score -= 12.0
            reasons.append(f"月供¥{monthly_renew:.2f}/月偏贵 (-12分)")
        elif monthly_renew <= 50.0:
            score -= 22.0
            reasons.append(f"月供¥{monthly_renew:.2f}/月较高持续失血 (-22分)")
        else:
            score -= 35.0
            reasons.append(f"月供¥{monthly_renew:.2f}/月天价严重失血 (-35分)")

        # 二手折价/捡漏调节
        if item_type == "secondhand" and renew_price_cny > 0:
            discount = price_cny / renew_price_cny
            if discount <= 0.3:
                score += 12.0
                reasons.append(f"二手大幅折价捡漏 (售价¥{price_cny:.1f}/月租¥{renew_price_cny:.1f} +12分)")
            elif discount <= 0.6:
                score += 6.0
                reasons.append(f"二手适度折价 (售价¥{price_cny:.1f}/月租¥{renew_price_cny:.1f} +6分)")
            elif discount > 1.2:
                score -= 8.0
                reasons.append(f"二手溢价倒卖 (售价¥{price_cny:.1f}/月租¥{renew_price_cny:.1f} -8分)")

        # IP 阻断但属于超低月供可抢救机型
        if not ip_clean:
            score -= 25.0
            reasons.append("IP被阻断 (官方换IP需¥15~¥35成本 -25分)")

        # 硬件加分
        if bandwidth_mbps >= 2000:
            score += 4.0
            reasons.append(f"{bandwidth_mbps}Mbps超大口径并发 (+4分)")
        elif bandwidth_mbps >= 1000:
            score += 2.0
            reasons.append(f"{bandwidth_mbps}Mbps千兆口径 (+2分)")
        if ram_gb >= 2.0:
            score += 3.0
            reasons.append(f"{int(ram_gb)}G大内存 (+3分)")

        final_score = int(round(max(0, min(100, score))))

    USD_RATE = 7.25

    # 6. 山哥传家宝资产化估值模型与做市商策略 (Annualized Arbitrage Fair Valuation & Market-Making)
    # 遵照山哥指导：
    # 1) 普通国际BGP (C级) 流量根本不值钱！绝不能因为 20000G 就膨胀产能！超过 1000G 边际价值极度递减。
    # 2) 二手每元性价比必须看真实总代价 (转让售价 + 月供续费 + 换IP成本)，绝不能只除以转让挂牌价！
    # 3) 5分【拦截避坑】的失血机，其性价比分/元绝不能高达几十，必须与总评级严格对齐！
    
    route_tier = str(route_info.get("tier") or "C")
    tier_k_map = {"S": 3.0, "A": 2.2, "B": 1.4, "C": 0.5}
    k_route = tier_k_map.get(route_tier, 0.5)

    # 对流量做线路加权：C级普通国际流量按对数折算，只有S/A级优质优化流量才具备高商业价值
    if route_tier == "C":
        # 普通国际BGP：1000G为基准，超出部分极度弱化 (20000G也只算约1.5倍)
        t_effective = 1.0 + (math.log10(max(1.0, flow_total_gb / 1000.0)) * 0.35) if flow_total_gb > 1000 else max(0.1, flow_total_gb / 1000.0)
    else:
        t_effective = max(0.1, min(5.0, flow_total_gb / 1000.0))

    ram_ratio = max(0.25, min(4.0, memory_mb / 1024.0))
    disk_ratio = max(0.5, min(4.0, disk_gb / 10.0))
    bw_ratio = max(0.5, min(10.0, bandwidth_mbps / 200.0))
    
    # 基础物理产能
    phi_capacity = 6.0 * (0.35 * t_effective + 0.25 * ram_ratio + 0.15 * disk_ratio + 0.25 * bw_ratio)

    combined = f"{name} {node_name} {detail} {intro}".lower()
    is_dual_isp = ("isp" in combined or "双isp" in combined) and bool(route_info.get("native_ip"))
    k_ip = 1.25 if is_dual_isp else (1.10 if route_info.get("native_ip") else 1.0)
    
    # 若IP被阻断，可用产能直接减半
    if not route_info["ip_clean"]:
        k_ip *= 0.5

    m_market = round(k_route * k_ip * phi_capacity, 1)

    monthly_saving = max(0.0, m_market - monthly_renew)
    annual_saving = round(12.0 * monthly_saving, 1)

    trade_strategy = match_user_trade_strategy(name, node_name, detail, intro, monthly_renew)
    fair_market_value_cny = 0.0
    valuation_rationale = ""
    target_buy_price = None
    target_sell_price = None

    if trade_strategy:
        target_buy_price = trade_strategy["buy_target_max"]
        target_sell_price = trade_strategy["sell_target_min"]
        fair_market_value_cny = trade_strategy["sell_target_min"]

        if item_type == "secondhand":
            if price_cny <= target_buy_price:
                if trade_strategy.get("model_code") in ("ISP", "SB"):
                    # 动态价格折算系数 k_price：
                    # 标价 0 元时 k=1.00 (满血 100 分)；标价达到买入上限 (250元) 时 k=0.65 (65分及格线)
                    ratio = min(1.5, price_cny / max(1.0, target_buy_price))
                    k_price = 1.0 - (0.35 * (ratio ** 0.85))
                    heirloom_deal_score = int(round(100.0 * max(0.2, k_price)))
                    final_score = heirloom_deal_score

                    if price_cny <= 50.0:
                        reasons.append(f"🚨 命中山哥传家宝超低折价扫单区(标价¥{price_cny:.1f} ≤ ¥50)，无脑秒杀 (折算买入分:{heirloom_deal_score})！")
                    else:
                        reasons.append(f"🚨 命中山哥传家宝买入底牌区间(标价¥{price_cny:.1f} ≤ 上限¥{target_buy_price:.0f})，价格折算系数k={k_price:.2f} (折算买入分:{heirloom_deal_score})")

                    valuation_rationale = f"【绝版传家宝买入评估】标价¥{price_cny:.1f} (折算系数k={k_price:.2f})，公允出售身价¥{target_sell_price:.0f}"
                else:
                    ratio = min(1.5, price_cny / max(1.0, target_buy_price))
                    k_price = 1.0 - (0.25 * (ratio ** 0.85))
                    iij_deal_score = int(round(80.0 * max(0.2, k_price)))
                    final_score = iij_deal_score
                    reasons.append(f"🚨 命中山哥旗舰生产力扫货区间(标价¥{price_cny:.1f} ≤ 上限¥{target_buy_price:.0f})，价格折算系数k={k_price:.2f} (折算买入分:{iij_deal_score})！")
                    valuation_rationale = f"【旗舰生产力扫货】标价¥{price_cny:.1f} (折算系数k={k_price:.2f})，公允出售身价¥{target_sell_price:.0f}"
            else:
                valuation_rationale = f"【山哥关注型号】买入目标¥{target_buy_price:.0f}以内，心理出售底价¥{target_sell_price:.0f} (当前标价¥{price_cny:.1f})"
                if price_cny > target_sell_price:
                    reasons.append(f"标价¥{price_cny:.1f}高于山哥心仪卖出价(¥{target_sell_price:.0f})，溢价过高")
                final_score = max(30, int(round(50.0 * (target_sell_price / max(1.0, price_cny)))))
        else:
            valuation_rationale = f"山哥核心主力资产：买入成本¥{trade_strategy.get('cost_basis', 0):.0f}，心理出售底价¥{target_sell_price:.0f}，买入目标≤¥{target_buy_price:.0f}"
    else:
        if monthly_renew <= 2.5:
            fair_market_value_cny = round(annual_saving, 1)
            valuation_rationale = f"绝版传家宝 (市场等效月租¥{m_market}/月，年化公允身价¥{fair_market_value_cny}，约10倍年供)"
        elif is_high_spec_powerhouse and monthly_renew <= 15.0:
            fair_market_value_cny = 120.0
            valuation_rationale = "高配生产力优质机型 (2.5G口+2G内存，年化公允转让身价¥120)"
        elif monthly_renew <= 10.0:
            fair_market_value_cny = 25.0
            valuation_rationale = "轻量超值月租机 (公允二手转让身价¥20-30)"
        elif monthly_renew <= 20.0:
            fair_market_value_cny = 10.0
            valuation_rationale = "平价月租机 (建议折价转让，公允身价约¥10)"
        else:
            fair_market_value_cny = 5.0
            valuation_rationale = "高月租失血机型 (无长期套利溢价，转让价值趋于0)"

    # 1. 资产价值倍数 (Value-to-Price Multiplier / 投资杠杆倍数)
    # 对于二手机器，买家的真实持有门槛是【买入售价 + 首月续费】，绝不能只按低廉的转让挂牌价算！
    if item_type == "secondhand":
        comp_price = price_cny + monthly_renew * 0.5
    elif item_type == "server":
        comp_price = monthly_renew
    else:
        comp_price = price_cny

    value_ratio = round(fair_market_value_cny / max(1.0, comp_price), 2)

    # 2. 月供产能杠杆倍数 (Monthly Leverage: 实际付的月租享受多少倍商业产出)
    monthly_leverage = round(m_market / max(0.1, monthly_renew), 2)

    # 5. 指数函数调整价格敏感度与真实每元产能得分 (Exponential Price Sensitivity Model)
    # 遵照山哥指导：结合物理产能变量与线路乘数系数，重新定义真实每元产出
    def exp_perceived_cost(cost: float) -> float:
        c = max(0.1, float(cost))
        base = 2.0
        return base + c * (1.0 - math.exp(-c / 5.0)) + (max(0.0, c - 25.0) ** 1.3) / 8.0

    eff_cost_ask = exp_perceived_cost(comp_price)
    eff_cost_renew = exp_perceived_cost(monthly_renew)

    # 结合【物理产能】与【套利健康度】
    health_factor = min(1.0, max(0.05, monthly_saving / max(1.0, m_market * 0.4))) if monthly_renew > 2.5 else 1.0
    eff_capability_points = round(m_market * 10 * health_factor, 1)

    # 铁律守卫：若综合品质评分低于 40 分（属于高月供失血或拦截避坑机型），性价比分/元强行锁死为 0.0，杜绝虚高倒挂！
    if final_score < 40:
        points_per_yuan = 0.0
    else:
        points_per_yuan = round(eff_capability_points / eff_cost_ask, 2)

    points_per_usd = round(points_per_yuan * USD_RATE, 2)
    cost_per_point_cny = round(comp_price / max(1, final_score), 3) if final_score > 0 else 999.0

    monthly_points_per_yuan = round(eff_capability_points / eff_cost_renew, 2) if final_score >= 40 else 0.0
    monthly_points_per_usd = round(monthly_points_per_yuan * USD_RATE, 2)

    if final_score >= 88:
        grade = "SSS"
        grade_label = "【绝版传家宝】"
        badge_color = "purple"
    elif final_score >= 75:
        grade = "A"
        grade_label = "【旗舰生产力】"
        badge_color = "green"
    elif final_score >= 48:
        grade = "B"
        grade_label = "【普通路人款】"
        badge_color = "blue"
    elif final_score >= 25:
        grade = "C"
        grade_label = "【轻度失血/偏贵】"
        badge_color = "orange"
    else:
        grade = "D"
        grade_label = "【垃圾/拦截避坑】"
        badge_color = "red"

    return {
        "score": final_score,
        "grade": grade,
        "grade_label": grade_label,
        "badge_color": badge_color,
        "points_per_yuan": points_per_yuan,
        "points_per_usd": points_per_usd,
        "cost_per_point_cny": cost_per_point_cny,
        "monthly_points_per_yuan": monthly_points_per_yuan,
        "monthly_points_per_usd": monthly_points_per_usd,
        "fair_market_value_cny": fair_market_value_cny,
        "valuation_rationale": valuation_rationale,
        "target_buy_price": target_buy_price,
        "target_sell_price": target_sell_price,
        "trade_strategy": trade_strategy,
        "value_ratio": value_ratio,
        "monthly_leverage": monthly_leverage,
        "m_market": m_market,
        "annual_saving": annual_saving,
        "breakdown": {
            "score": final_score,
            "tier": tier,
            "monthly_renew": monthly_renew,
        },
        "reasons": reasons,
    }
