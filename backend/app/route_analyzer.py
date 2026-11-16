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
    {
        "id": "jppro",
        "tier": "A",
        "tier_name": "A级·日本三网优化直连",
        "isp": "三网优化",
        "line_name": "日本 Pro 直连优化 (JPPro)",
        "patterns": [r"jppro", r"jp[\s_\-]*pro"],
        "weight": 84,
    },
    {
        "id": "laxpro",
        "tier": "A",
        "tier_name": "A级·美西三网优化直连",
        "isp": "三网优化",
        "line_name": "洛杉矶 Pro 直连优化 (LAXPro)",
        "patterns": [r"laxpro", r"lax[\s_\-]*pro"],
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
    bandwidth_mbps: int = 1000,
) -> Dict[str, Any]:
    """
    Full multidimensional evaluation for secondhand arbitrage, long-term asset value, and sniping safety.
    """
    route_info = analyze_route(name=name, node_name=node_name, detail=detail, intro=intro, area=area)
    traffic_info = analyze_traffic(flow_total_gb, flow_used_gb, reset_price_cny)

    price_cny = max(0.0, float(price_cny or 0))
    # 明确支持 0元续费 (终身免续费/永久传家宝机型)
    monthly_renew = max(0.0, float(renew_price_cny if renew_price_cny is not None else price_cny))

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
        bandwidth_mbps=bandwidth_mbps,
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
    bandwidth_mbps: int = 0,
    ram_gb: float = 0.0,
    flow_total_gb: float = 0.0,
) -> Optional[Dict[str, Any]]:
    """
    做市策略字典：
    仅用于用户特定做市/监控意向辅助提示，绝不直接硬编码公允月租 fair_monthly_value！
    公允月租全部由真实物理硬件 (内存、硬盘、带宽、流量) 动态纯数学公式计算产生！
    """
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
    # 明确支持 0元续费 (终身免供/永久传家宝机型)
    if item_type == "secondhand":
        monthly_renew = max(0.0, float(renew_price_cny if renew_price_cny is not None else price_cny))
    else:
        monthly_renew = max(0.0, float(renew_price_cny if (renew_price_cny is not None and renew_price_cny > 0) else price_cny))
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

    # 对流量做线路加权：C级普通国际流量按对数折算，只有S/A级优质优化流量才具备高商业价值
    if route_tier == "C":
        # 普通国际BGP：1000G为基准，超出部分极度弱化 (20000G也只算约1.5倍)
        t_effective = 1.0 + (math.log10(max(1.0, flow_total_gb / 1000.0)) * 0.35) if flow_total_gb > 1000 else max(0.1, flow_total_gb / 1000.0)
    else:
        t_effective = max(0.1, min(5.0, flow_total_gb / 1000.0))

    ram_ratio = max(0.25, min(4.0, memory_mb / 1024.0))
    disk_ratio = max(0.5, min(4.0, disk_gb / 10.0))
    bw_ratio = max(0.5, min(10.0, bandwidth_mbps / 200.0))  # noqa: F841  预留：接入带宽加权评分后使用

    # 物理配置动态综合加权 (以 1000G / 1C1G / 10G / 500M~1000M 标准物理机型为 1.0 基准)
    # 严格对齐山哥市场动态：硬件、流量多少直接决定公允产出比，绝不能机械一刀切！
    flow_ratio = (flow_total_gb / 1000.0) if route_tier != "C" else t_effective
    phi_norm = 0.40 * flow_ratio + 0.20 * ram_ratio + 0.15 * disk_ratio + 0.25 * min(2.0, bandwidth_mbps / 500.0)  # noqa: F841  预留：物理配置归一化系数

    # -------------------------------------------------------------
    # 6. 山哥四大物理维度模型 (网络40分 / 配置20分 / 带宽10分 / 流量20分 + 附加IP 10分)
    # 遵照山哥指导："提高一下流量的价值，这个如果一个月5T流量，我觉得还值一点点，给个60分，
    # 这么高性能，才1T流量，用处不大，我跑业务的话，1T都不够用，对吧，如果10T我会给100分以上"
    # -------------------------------------------------------------
    combined = f"{name} {node_name} {detail} {intro}".lower()

    if any(k in combined for k in ["cn2 gia", "cn2-gia", "cn2_gia", "as4809", "cmin2", "as58807", "lax premium", "三网优化直连"]):
        network_tier_level = 1
        network_tier_name = "1级网络·三网顶级优化"
        tier_base_m = 26.0
        route_score = 40  # 满分40分
    elif any(k in combined for k in ["9929", "as9929", "cu9929", "nus9929", "nus 9929"]):
        network_tier_level = 2
        network_tier_name = "2级网络·9929精品优化"
        tier_base_m = 18.0
        route_score = 36
    elif any(k in combined for k in ["4837", "as4837", "cu169", "cmi", "as58453", "hkcm"]):
        network_tier_level = 3
        network_tier_name = "3级网络·4837/CMI骨干优化"
        tier_base_m = 13.5
        route_score = 31
    elif any(k in combined for k in ["softbank", "软银", "bbtec", "jpsb", "sb-", "iij", "jpiij"]):
        network_tier_level = 4
        network_tier_name = "4级网络·软银/IIJ特色直连"
        tier_base_m = 10.0  # 用户指定锚点: JPSB 1000M 1000G 1C1G = 10元/月
        route_score = 26
    elif any(k in combined for k in ["tw-lite", "hkplus", "sgbgp", "twbgp", "jpbgp", "亚太"]) or ("hks" in combined and "nhk" not in combined):
        network_tier_level = 5
        network_tier_name = "5级网络·亚太普通直连BGP"
        tier_base_m = 11.5
        route_score = 20
    else:
        network_tier_level = 6
        network_tier_name = "6级网络·普通国际无优化BGP"
        tier_base_m = 10.0  # 用户指定基准: 公允价值 10元/月左右
        route_score = 14

    # 1. 性能配置评分与硬件规格乘数 (Specs Score & Continuous Multiplier)
    # 遵照山哥指导："规则最好动态地来通过不一样的内存、硬盘大小、带宽还有每月流量动态地来计算，以实际的数据为准"
    score_cpu = min(8.0, cpu * 3.0)
    score_ram = min(8.0, ram_gb * 4.0)
    score_disk = min(4.0, disk_gb * 0.2)
    specs_score = int(round(score_cpu + score_ram + score_disk))
    specs_score = min(20, max(4, specs_score))

    # 硬件连续动态乘数 spec_mult (完全基于真实物理硬件，不依赖任何机型名称)
    # CPU: 1核=1.0; 2核=1.25; 4核=1.55
    m_cpu = 1.0 + 0.25 * max(0, cpu - 1)
    # RAM: 512M=0.70; 1G=1.00; 2G=1.35; 4G=1.70
    m_ram = 0.50 + 0.50 * (max(0.25, ram_gb) ** 0.55)
    # DISK: 5G极小盘=0.78 (惩罚小盘); 10G基准=1.00; 20G=1.27; 40G=1.62
    m_disk = (max(2.0, disk_gb) / 10.0) ** 0.35
    spec_mult = round(m_cpu * m_ram * m_disk, 3)

    # 2. 带宽口径评分与动态乘数 (Bandwidth Score: 满分 10分)
    # 动态连续幂律响应: 100M->0.73, 200M->0.88, 1000M->1.00, 2000M->1.10, 5000M->1.25, 10000M->1.38
    if bandwidth_mbps <= 50:
        bandwidth_score = 2
    elif bandwidth_mbps <= 100:
        bandwidth_score = 4
    elif bandwidth_mbps <= 200:
        bandwidth_score = 6
    elif bandwidth_mbps <= 500:
        bandwidth_score = 7
    elif bandwidth_mbps <= 1000:
        bandwidth_score = 9
    elif bandwidth_mbps <= 2500:
        bandwidth_score = 9.5
    elif bandwidth_mbps <= 5000:
        bandwidth_score = 9.8
    else:
        bandwidth_score = 10
    bw_mult = round((max(50, bandwidth_mbps) / 1000.0) ** 0.14, 3)

    # 3. 流量配额评分与动态生产力乘数 (以额定月度总配额 flow_total_gb 为真实物理资产)
    # 动态连续响应: 300G->0.55, 600G->0.77, 1000G->1.00, 2000G->1.41, 5000G->2.24, 10000G->3.16
    if flow_total_gb <= 300:
        traffic_score = 6
    elif flow_total_gb <= 500:
        traffic_score = 8
    elif flow_total_gb <= 1000:
        traffic_score = 14
    elif flow_total_gb <= 2000:
        traffic_score = 16
    elif flow_total_gb <= 5000:
        traffic_score = 19
    else:
        traffic_score = 20
    flow_mult = round((max(100.0, flow_total_gb) / 1000.0) ** 0.50, 3)

    # 4. 附加 IP 属性与纯净度 (IP Score: 满分 10分)
    is_dual_isp = ("isp" in combined or "双isp" in combined) and bool(route_info.get("native_ip"))
    if is_dual_isp:
        ip_score = 10
        k_ip = 1.25
    elif route_info.get("native_ip"):
        ip_score = 8
        k_ip = 1.08
    else:
        ip_score = 5
        k_ip = 1.00

    if not route_info["ip_clean"]:
        ip_score = 1
        k_ip *= 0.15

    # 物理全景总得分 (满分 100 分)
    hardware_perf_score = int(round(route_score + specs_score + bandwidth_score + traffic_score + (ip_score - 5)))
    hardware_perf_score = min(100, max(10, hardware_perf_score))
    performance_score = hardware_perf_score

    # 纯动态公允月租计算 (Pure Physics Dynamic Fair Monthly Rent)
    m_market = round(tier_base_m * bw_mult * flow_mult * spec_mult * k_ip, 1)

    # -------------------------------------------------------------
    # 核心铁律：6级普通国际无优化网络 (C级) 物理天花板
    # 普通国际BGP网络素质低下，绝非大流量商业机，不能因为流量大就虚标至150元月租！
    # 遵照山哥指导：
    # 1. 6级普通国际流量商业价值严重衰减，公允月租上限严格封顶在 12.0 元/月 (无论流量多大)
    # 2. 6级网络机器公允二手转让身价封顶在 ¥40~¥50，坚决杜绝百元身价虚标！
    # -------------------------------------------------------------
    if network_tier_level == 6:
        # 6级无优化国际BGP：流量再大也是廉价国际流量，公允月租封顶12元/月
        flow_mult = min(1.20, flow_mult)
        m_market = min(12.0, m_market)
    elif network_tier_level == 5:
        flow_mult = min(2.0, flow_mult)
        m_market = min(22.0, m_market)

    # 彻底杜绝文字硬编码策略对公允月租的干预，以实际物理硬件为绝对权威准绳
    trade_strategy = None

    monthly_saving = max(0.0, m_market - monthly_renew)
    annual_saving = round(12.0 * monthly_saving, 1)

    fair_market_value_cny = 0.0
    valuation_rationale = ""
    target_buy_price = None
    target_sell_price = None

    if trade_strategy:
        target_buy_price = trade_strategy["buy_target_max"]
        target_sell_price = trade_strategy["sell_target_min"]
        fair_market_value_cny = trade_strategy["sell_target_min"]
        valuation_rationale = f"山哥专属做市策略机型 ({trade_strategy['model_name']}，买入目标<=¥{target_buy_price:g}，出售底价¥{target_sell_price:g})"
    else:
        # 遵照山哥做市与广义指数衰减二手身价方程:
        # P_fair(R) = P_max * max(0, 1 - (R / 20)^1.25)
        # 1. 峰值零月供身价 P_max:
        tier_pmax_map = {
            1: 350.0,  # 1级: 顶级三网GIA/CMIN2
            2: 280.0,  # 2级: 9929精品优化 (用户标尺: 正常区间 ¥150~¥250，中枢¥200)
            3: 240.0,  # 3级: 4837/CMI骨干优化 (双ISP加成后达 ¥300，正常区间 ¥250~¥300)
            4: 180.0,  # 4级: 软银JPSB/IIJ特色直连 (用户标尺: 正常区间 ¥130~¥200)
            5: 120.0,  # 5级: 亚太普通直连BGP
            6: 80.0,   # 6级: 普通国际无优化
        }
        base_pmax = tier_pmax_map.get(network_tier_level, 80.0)

        # 核心金融铁律（山哥指导）：
        # 0.18 ~ 1.5 元极低月供具备“零持有成本囤积期权”（年供仅 ¥2~¥18，卖家无去库存现金流压力）
        # 优化线路即使是小盘或窄口径，转让底价受期权价值保护，避免三重物理连乘过度削平
        if monthly_renew <= 1.5 and network_tier_level <= 4:
            effective_mult = max(0.72, bw_mult * flow_mult * spec_mult)
        else:
            effective_mult = bw_mult * flow_mult * spec_mult

        p_max = round(base_pmax * effective_mult * k_ip, 1)

        # 2. 月供衰减因子 K_renew (20元为绝对天花板):
        if monthly_renew >= 20.0:
            k_renew = 0.0
        else:
            k_renew = max(0.0, 1.0 - (monthly_renew / 20.0) ** 1.25)

        fair_market_value_cny = round(p_max * k_renew, 1)
        if monthly_renew >= 20.0:
            fair_market_value_cny = 0.0
            target_buy_price = 0.0
            target_sell_price = 5.0
            valuation_rationale = "月供超¥20硬天花板，持续失血负资产 (二手身价归零，坚决不买)"
        elif fair_market_value_cny <= 15.0:
            fair_market_value_cny = max(0.0, fair_market_value_cny)
            target_buy_price = round(fair_market_value_cny * 0.5, 1)
            target_sell_price = fair_market_value_cny
            valuation_rationale = f"常规月租机 (二手转让身价约¥{fair_market_value_cny})"
        else:
            target_buy_price = round(fair_market_value_cny * 0.70, 1)
            target_sell_price = fair_market_value_cny
            valuation_rationale = f"优质低月租资产 (公允转让身价约¥{fair_market_value_cny}，买入目标<=¥{target_buy_price})"

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

    # -------------------------------------------------------------
    # 7. 山哥四维量化打分体系全量重构 (4-Dimensional Quant & Arbitrage Engine)
    # 维度 1: 机器性能评分 (performance_score: 0 - 100, 对应物理规格与公允月租)
    # 维度 2: 续费价格评分 (renew_price_score: 0 - 100, 越低越神)
    # 维度 3: 销售价格评分 (sale_price_score: 0 - 200, 指数函数敏感度模型)
    # 维度 4: 套利回本评分 (arbitrage_score: 0 - 100, 核心回本现金流)
    # -------------------------------------------------------------

    # 1. 机器性能评分 (performance_score: 0 - 100)
    p_base = {"S": 88, "A": 78, "B": 62, "C": 42}.get(route_tier, 60)
    if is_dual_isp:
        p_base += 6
    elif is_native:
        p_base += 2
    if bandwidth_mbps >= 10000:
        p_base += 8
    elif bandwidth_mbps >= 1000:
        p_base += 4
    elif bandwidth_mbps <= 50:
        p_base -= 5
    if ram_ratio >= 2.0:
        p_base += 3
    if disk_ratio >= 2.0:
        p_base += 2
    if cpu >= 2:
        p_base += 3
    if flow_total_gb >= 2000:
        p_base += 4
    if not route_info["ip_clean"]:
        p_base -= 25
    performance_score = min(100, max(10, p_base))

    # 2. 续费价格评分 (renew_price_score: 0 - 100)
    # 遵照山哥指导："越便宜价格敏感度越低，0-2无感，2-5略微有感觉，6-10稍微敏感，10-20需要高质量硬件条件补充"
    if monthly_renew <= 2.0:
        # 0 ~ 2 元: 绝对无感区 (0元, 0.18元, 1元, 2元 敏感度几乎为0，打98~100分顶格神仙免持有成本)
        renew_price_score = int(round(100 - monthly_renew * 1.0))
    elif monthly_renew <= 5.0:
        # 2 ~ 5 元: 略微有感区 (90 ~ 98分)
        renew_price_score = int(round(98 - (monthly_renew - 2.0) * (8.0 / 3.0)))
    elif monthly_renew <= 10.0:
        # 6 ~ 10 元: 稍微敏感区 (75 ~ 90分)
        renew_price_score = int(round(90 - (monthly_renew - 5.0) * (15.0 / 5.0)))
    elif monthly_renew <= 20.0:
        # 10 ~ 20 元: 高敏感区 (50 ~ 75分) - 必须依赖高质量硬件强力补充
        renew_price_score = int(round(75 - (monthly_renew - 10.0) * (25.0 / 10.0)))
    elif monthly_renew <= 35.0:
        renew_price_score = int(round(50 - (monthly_renew - 20.0) * (25.0 / 15.0)))
    else:
        renew_price_score = max(0, int(round(25 - (monthly_renew - 35.0) * 1.0)))

    # -------------------------------------------------------------
    # 7. 山哥回本套利与综合性价比模型全量重构 (Universal Payback & Profit Quant Engine)
    # 遵照山哥指导反推建立：
    # 核心原则：
    # 1. 真实价值取决于 "净利润产生的套利回本能力" 与 "月供健康度(无失血)"
    # 2. 流量是业务生命线：1T不够跑业务属于鸡肋；5T值一点点(给60分)；10T产生巨大商业产能(给100分以上)！
    # 3. 每月利润 = 公允续费价格 - 当前续费价格
    # 4. 回本周期 = 销售价格 / 每月利润
    # -------------------------------------------------------------
    profit = round(m_market - monthly_renew, 2)
    margin = profit / max(1.0, m_market)
    real_cost = price_cny

    is_traffic_depleted = traffic_info.get("is_depleted", False) or (flow_total_gb > 0 and (flow_total_gb - flow_used_gb) <= 5.0)

    if not route_info["ip_clean"]:
        # IP 被墙属于致命缺陷（核心功能缺失无法连接，换新IP需花¥15-¥35）
        # 绝不允许打出高分，一票否决封顶压制在 15~28 分避坑残疾机！
        final_score = min(28, max(5, int(round(25.0 - (price_cny / max(1.0, monthly_renew)) * 3.0))))
        payback_months = 999.0
        annual_roi_percent = 0.0
        arbitrage_score = 10
        monthly_cash_savings = 0.0
        payback_tier = "IP被墙⚠️ (需自费¥35换IP)"
        payback_badge_color = "red"
        reasons.append("🛑 致命缺陷：IP处于被墙阻断状态！无法直接访问，需自费¥15~¥35换新IP，属于二手残疾机")
    elif profit <= 0:
        # 月供倒挂 (轻度小鸡路人价 vs 重度失血包袱)
        loss = abs(profit)
        p_to_r = price_cny / max(1.0, monthly_renew)
        arbitrage_score = 25
        monthly_cash_savings = 0.0

        if monthly_renew <= 15.0 and loss <= 6.0:
            # 遵照山哥指导："IIJ不是顶级网络，应该比4837低，而且流量少，给个路人分40-65就好"
            # 月供处于低位 (<=15元) 且轻度差额的小鸡，属于标准路人流转车，根据转让溢价给出 40~65 分路人分！
            if p_to_r <= 2.0:
                final_score = int(round(65.0 - p_to_r * 2.5))
            elif p_to_r <= 7.0:
                final_score = int(round(60.0 - (p_to_r - 2.0) * (20.0 / 5.0)))
            else:
                final_score = max(10, int(round(40.0 - (p_to_r - 7.0) * 3.0)))
            payback_months = round(p_to_r * 4.0, 1)
            annual_roi_percent = 0.0
            payback_tier = f"常规路人机 (月供¥{monthly_renew:g}·转让¥{price_cny:g})"
            payback_badge_color = "blue" if final_score >= 48 else "orange"
            reasons.append(f"📦 常规路人小鸡 (月供¥{monthly_renew:g}·微幅溢价)，二手转让费¥{price_cny:g}，属于正常流转")
        else:
            loss_ratio = loss / max(1.0, monthly_renew)
            final_score = int(round(max(5.0, 35.0 - loss_ratio * 15.0 - p_to_r * 4.0)))
            payback_months = 999.0
            annual_roi_percent = 0.0
            payback_tier = f"高月供失血 (月供¥{monthly_renew:g} > 产出¥{m_market:g})"
            payback_badge_color = "red"
            reasons.append(f"⚠️ 高月供失血包袱：每月续费¥{monthly_renew:g}远高于公允产出¥{m_market:g}(净亏¥{loss:g}/月)")
    else:
        # 正向有利润机型
        payback_months = round(real_cost / profit, 1) if real_cost > 0 else 0.0
        annual_roi_percent = round((profit * 12.0 / max(1.0, real_cost)) * 100.0, 1)
        monthly_cash_savings = profit

        # 1. 回本套利得分 (arbitrage_score)
        if payback_months <= 1.0 or real_cost <= 0:
            raw_arb_score = 10000.0
            arbitrage_score = 10000
        elif payback_months <= 6.0:
            if payback_months <= 2.0:
                raw_arb_score = 10000.0 * math.exp(-(payback_months - 1.0) * 1.5)
            else:
                raw_arb_score = 100.0 + (6.0 - payback_months) * 10.0
            arbitrage_score = int(round(200.0 + 9800.0 * math.exp(-(payback_months - 1.0) * 0.95)))
        elif payback_months <= 12.0:
            raw_arb_score = 80.0 + (12.0 - payback_months) * (20.0 / 6.0)
            arbitrage_score = int(round(200.0 - (payback_months - 6.0) * (80.0 / 6.0)))
        elif payback_months <= 18.0:
            raw_arb_score = 65.0 + (18.0 - payback_months) * (15.0 / 6.0)
            arbitrage_score = int(round(120.0 - (payback_months - 12.0) * (50.0 / 6.0)))
        elif payback_months <= 24.0:
            raw_arb_score = 50.0 + (24.0 - payback_months) * (15.0 / 6.0)
            arbitrage_score = int(round(70.0 - (payback_months - 18.0) * (20.0 / 6.0)))
        elif payback_months <= 36.0:
            raw_arb_score = 35.0 + (36.0 - payback_months) * (15.0 / 12.0)
            arbitrage_score = int(round(50.0 - (payback_months - 24.0) * (15.0 / 12.0)))
        else:
            raw_arb_score = max(5.0, 35.0 * math.exp(-(payback_months - 36.0) / 20.0))
            arbitrage_score = max(5, int(round(35.0 * math.exp(-(payback_months - 36.0) / 20.0))))

        # 2. 综合性价比评分结合利润丰厚度 (Profit Margin Factor)
        # 传家宝 (0.19元/月) margin = 0.98 -> 满额享受无敌性价比 100-N
        # 大流量商业机 (10T) margin = 73/126 = 0.58 -> 满额享受无敌性价比 100-N
        # 微利高月供机 (5T, renew 53, profit 7.2) margin = 0.12 -> 利润微薄，买家每月仍要扛53元高月供
        # 遵照山哥指导：
        # 1. 6级普通国际无优化机型 (C级)：
        #    - 溢价转让 (如 115元/168元): 属于 50~65 分左右（有一定投资价值/可观察/但不能给到80+神机高分）
        #    - 100元左右: 评定在 58~63 分左右（有投资价值/还不错可观察）
        #    - 115元左右: 评定在 52~55 分左右（正常市场价/可观察）
        #    - 168元左右 (2年回本): 评定在 48~50 分左右（公允偏贵/可观察投资价值，不跌入严重失血）
        #    - 6级网络最高综合得分严格封顶在 68 分以内，杜绝虚高评为80+超级性价比！
        if network_tier_level == 6:
            # 6级无优化：基准 50 分左右，根据售价和回本速度在 48 ~ 65 分区间浮动
            tier6_score = int(round(50.0 + (24.0 - payback_months) * 0.9 + (5.0 - monthly_renew) * 1.5))
            if price_cny <= 100.0 and monthly_renew <= 5.0:
                tier6_score = max(tier6_score, 60)
            elif price_cny <= 120.0 and monthly_renew <= 5.0:
                tier6_score = max(tier6_score, 54)
            final_score = min(68, max(42, tier6_score))
        elif margin >= 0.40 or raw_arb_score >= 1000:
            final_score = int(round(0.15 * hardware_perf_score + 0.85 * raw_arb_score))
        else:
            margin_boost = margin * 70.0
            p_to_r = price_cny / max(1.0, monthly_renew)
            final_score = int(round(50.0 + margin_boost + (0.5 - p_to_r) * 5.0))
            final_score = min(75, max(45, final_score))

        if payback_months <= 6.0:
            payback_tier = f"半年内回本 ({payback_months:.1f}月·超级性价比)"
            payback_badge_color = "purple"
            reasons.append(f"⚡ 半年内极速回本({payback_months:.1f}个月·首年年化ROI {annual_roi_percent:.1f}%)，优秀回本能力！")
        elif payback_months <= 12.0:
            payback_tier = f"1年回本 ({payback_months:.1f}月·性价比)"
            payback_badge_color = "green"
            reasons.append(f"🔥 1年内超速回本({payback_months:.1f}个月·首年年化ROI {annual_roi_percent:.1f}%)，稳健回本！")
        elif payback_months <= 18.0:
            payback_tier = f"1.5年回本 ({payback_months:.1f}月·市场价)"
            payback_badge_color = "blue"
            reasons.append(f"📈 1.5年左右回本({payback_months:.1f}个月·年化ROI {annual_roi_percent:.1f}%)，公允市场价")
        elif payback_months <= 24.0:
            payback_tier = f"2年回本 ({payback_months:.1f}月·高价格偏贵)"
            payback_badge_color = "orange"
            reasons.append(f"⚠️ 2年左右回本({payback_months:.1f}个月)，转让价格偏高")
        else:
            payback_tier = f"回本漫长 ({payback_months:.1f}月·低性价比)"
            payback_badge_color = "red"
            reasons.append(f"⏳ 回本周期漫长({payback_months:.1f}个月)，严重溢价无性价比")

    # 当期流量微调 (遵照山哥指导："如果他流量用尽了也没关系，如果足够便宜，我下个月续费一样用")
    # 二手买的是机器的长期资产所有权与续费资格，额定流量配额是下月续费后永久享有的持续生产力！
    # 仅作当期等待刷新的轻微贴现扣减 (最多仅扣2~3分)，绝不伤筋动骨误杀便宜好机
    if is_traffic_depleted:
        depleted_penalty = 3
        final_score = max(5, final_score - depleted_penalty)
        reasons.append(f"🔋 资产提示：当期流量已用尽（不影响长期资产价值：下月续费自动恢复{flow_total_gb:g}G满血，价格足够便宜下月照样用）")

    # 4. 评级标签与徽章定义 (完全对齐山哥最新指示)
    if final_score >= 1000:
        grade = "SSS"
        grade_label = "【10000分·当月秒回本神迹】"
        badge_color = "purple"
    elif final_score >= 100:
        grade = "SS"
        grade_label = "【100-N·无敌性价比】"
        badge_color = "magenta"
    elif final_score >= 80:
        grade = "S"
        grade_label = "【80-100·超级性价比】"
        badge_color = "green"
    elif final_score >= 65:
        grade = "A"
        grade_label = "【性价比款】"
        badge_color = "cyan"
    elif final_score >= 48:
        grade = "B"
        grade_label = "【还凑合·正常市场价】"
        badge_color = "blue"
    elif final_score >= 35:
        grade = "C"
        grade_label = "【高月供/偏贵溢价】"
        badge_color = "orange"
    else:
        grade = "D"
        grade_label = "【严重失血/拦截避坑】"
        badge_color = "red"

    # 销售价格敏感度评分 (sale_price_score: 0 - 200)
    base_anchor_price = max(20.0, fair_market_value_cny if fair_market_value_cny > 0 else 80.0)
    p_ratio = price_cny / base_anchor_price
    if price_cny <= 0:
        sale_price_score = 200
    elif p_ratio <= 0.70:
        sale_price_score = 200
    elif p_ratio <= 1.00:
        sale_price_score = int(round(200.0 - (p_ratio - 0.70) * (20.0 / 0.30)))
    elif p_ratio <= 1.25:
        sale_price_score = int(round(180.0 - (p_ratio - 1.00) * (40.0 / 0.25)))
    else:
        diff = p_ratio - 1.25
        decay = math.exp(-diff / 0.45)
        sale_price_score = int(round(50.0 + 90.0 * decay))
        sale_price_score = max(5, min(200, sale_price_score))

    # 山哥心智意愿与月供承受阶梯标签
    if monthly_renew <= 3.0:
        renew_intent_tip = f"月供¥{monthly_renew:g} (0-3元无感区·肯定会买)"
    elif monthly_renew <= 5.0:
        renew_intent_tip = f"月供¥{monthly_renew:g} (3-5元微感区·基本会买)"
    elif monthly_renew <= 8.0:
        renew_intent_tip = f"月供¥{monthly_renew:g} (5-8元敏感区·准备充足会买)"
    elif monthly_renew <= 15.0:
        renew_intent_tip = f"月供¥{monthly_renew:g} (8-15元高敏感区·急用应急买)"
    else:
        renew_intent_tip = f"月供¥{monthly_renew:g} (超20元天花板·坚决不会买)"

    # ACCK 0.18 元续费小鸡的持有门槛规则
    if monthly_renew <= 0.35 and ("acck" in combined or "exchange" in combined):
        reasons.append("⚠️ ACCK 规则约束：0.18元续费机新手账号无法购买，必须同时持有一台 ≥¥1.0/月的主机才能保持续费特权")

    if network_tier_level <= 4 and monthly_renew <= 1.5:
        if price_cny < 120.0 and price_cny > 0.0:
            reasons.append("⚠️ 价格异常跌破 ¥120 地板：需警惕平台经营风险、拔线或恶性阴跌，切忌盲目接盘")

    if final_score >= 100:
        intent_label = "肯定买 (无敌性价比)"
    elif final_score >= 80:
        intent_label = "基本会买 (超级性价比)"
    elif final_score >= 65:
        intent_label = "准备充足会买 (性价比款)"
    elif final_score >= 48:
        intent_label = "可能会买/急用买 (公允市场价)"
    else:
        intent_label = "不会买 (偏贵避坑)"

    return {
        "score": final_score,
        "grade": grade,
        "grade_label": grade_label,
        "intent_label": intent_label,
        "renew_intent_tip": renew_intent_tip,
        "badge_color": badge_color,
        "network_tier_level": network_tier_level,
        "network_tier_name": network_tier_name,
        "route_score": route_score,
        "bandwidth_score": bandwidth_score,
        "traffic_score": traffic_score,
        "specs_score": specs_score,
        "hardware_perf_score": hardware_perf_score,
        "performance_score": performance_score,
        "renew_price_score": renew_price_score,
        "sale_price_score": sale_price_score,
        "arbitrage_score": arbitrage_score,
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
        "monthly_cash_savings": monthly_cash_savings,
        "payback_months": payback_months,
        "annual_roi_percent": annual_roi_percent,
        "payback_tier": payback_tier,
        "payback_badge_color": payback_badge_color,
        "breakdown": {
            "score": final_score,
            "network_tier_level": network_tier_level,
            "network_tier_name": network_tier_name,
            "route_score": route_score,
            "bandwidth_score": bandwidth_score,
            "traffic_score": traffic_score,
            "specs_score": specs_score,
            "hardware_perf_score": hardware_perf_score,
            "performance_score": performance_score,
            "renew_price_score": renew_price_score,
            "sale_price_score": sale_price_score,
            "arbitrage_score": arbitrage_score,
            "tier": tier,
            "monthly_renew": monthly_renew,
        },
        "reasons": reasons,
    }
