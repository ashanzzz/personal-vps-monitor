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

    # 1. IIJ 旗舰机型
    if "iij" in combined:
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

    # 1. 核心第一权重：线路与网络品质 (0 - 35 分) —— 用户最核心看重
    network_score = 8.0
    tier = route_info["tier"]
    if tier == "S":
        network_score = 35.0
        reasons.append(f"{route_info['primary_route']} (S级顶级精品网 +35)")
    elif tier == "A":
        network_score = 26.0
        reasons.append(f"{route_info['primary_route']} (A级优化直连 +26)")
    elif tier == "B":
        network_score = 16.0
        reasons.append("亚太直连BGP (+16)")
    else:
        network_score = 8.0
        reasons.append("普通国际BGP (+8)")

    if route_info["native_ip"]:
        network_score = min(35.0, network_score + 4.0)
        reasons.append("原生IP认证 (+4)")

    # 官方更换IP机制：被墙不再一票否决，扣除适度修复摩擦成本(-10分)
    if not route_info["ip_clean"]:
        network_score = max(5.0, network_score - 10.0)
        reasons.append("当前IP阻断(官方支持换新IP，成本约¥15~¥35/或1500AK币 -10)")

    # 2. 核心第二权重：周期流量配额与带宽资产 (0 - 30 分) —— 用户最核心看重
    traffic_score = 10.0
    remaining_gb = max(0.0, flow_total_gb - flow_used_gb)
    is_depleted = (flow_total_gb > 0 and remaining_gb <= 1.0)
    quota_efficiency = flow_total_gb / monthly_renew if monthly_renew > 0 else 50.0
    is_budget_asset = (monthly_renew <= 10.0) or (price_cny <= 10.0)

    if flow_total_gb <= 0:
        traffic_score = 30.0
        reasons.append("不限流量无限带宽 (+30)")
    elif flow_total_gb >= 2000.0:
        traffic_score = 28.0
        reasons.append(f"超大流量配额({int(flow_total_gb)}G/月 +28)")
    elif flow_total_gb >= 1000.0:
        traffic_score = 25.0
        reasons.append(f"充沛流量配额({int(flow_total_gb)}G/月 +25)")
    elif is_budget_asset:
        if quota_efficiency >= 100:
            traffic_score = 24.0
            reasons.append(f"高能效配额({int(quota_efficiency)}G/元·月，每月自动满血重置 +24)")
        else:
            traffic_score = 20.0
            reasons.append(f"周期配额{int(flow_total_gb)}G按月自动刷新 (+20)")
    else:
        traffic_score = 16.0
        reasons.append(f"月配额{int(flow_total_gb)}G (+16)")

    if is_depleted:
        reasons.append("提示：当期流量已用尽，静待月度重置自动满血")

    # 3. 硬件规格与长线持有月供 (0 - 20 分) —— 严格对齐山哥铁律基准
    # 铁律：<=2元超级超值；好线路<=10元才是真正超值；20元左右成交价一般；
    # 高配生产力：2000M(2.5G)超大口径、大内存(>=2G)、大硬盘(>=20G)、多流量(>=1000G)
    ram_gb = max(0.25, memory_mb / 1024)
    is_high_spec_powerhouse = (ram_gb >= 2.0 and disk_gb >= 20 and flow_total_gb >= 1000.0 and bandwidth_mbps >= 1000)

    ltv_score = 10.0
    if monthly_renew <= 2.0:
        ltv_score = 20.0
        reasons.append(f"月供仅¥{monthly_renew:.2f}/月 (超级超值/绝版传家宝，长期持有近乎零成本 +20)")
    elif monthly_renew <= 10.0:
        ltv_score = 17.0
        reasons.append(f"月供¥{monthly_renew:.2f}/月 (好线路10元内，真正超值月供梯队 +17)")
    elif is_high_spec_powerhouse and monthly_renew <= 20.0:
        # 如日本 IIJ 这种高配机器（2G口径+2G内存+20G盘+1000G流量，月折合13.25元）
        ltv_score = 18.0
        reasons.append(f"大带宽({bandwidth_mbps}M)+大内存({int(ram_gb)}G)+大硬盘({disk_gb}G) (优质高配生产力机型，¥{monthly_renew:.2f}/月超高价值 +18)")
    elif monthly_renew <= 20.0:
        ltv_score = 12.0
        reasons.append(f"月供¥{monthly_renew:.2f}/月 (好线路20元档成交价一般/平价 +12)")
    elif monthly_renew <= 22.0:
        ltv_score = 7.0
        reasons.append(f"月供¥{monthly_renew:.2f}/月略微超出平价线 (+7)")
    elif monthly_renew <= 25.0:
        ltv_score = 2.0
        reasons.append(f"月供¥{monthly_renew:.2f}/月已偏高，长期养机持续失血负担大 (+2)")
    elif is_high_spec_powerhouse:
        ltv_score = 10.0
        reasons.append(f"高配生产力机型({int(ram_gb)}G内存/{disk_gb}G盘/{bandwidth_mbps}M口) (大容量多流量，月供尚可 +10)")
    else:
        ltv_score = 0.0
        reasons.append(f"月续费¥{monthly_renew:.2f}/月过高，缺乏长期持有性价比 (0)")

    # 针对山哥业务场景专属硬件加分（FRP反代/大流量/超大口径）：
    # 2000Mbps (2.5G口) 是市面稀缺硬实力！
    if bandwidth_mbps >= 2000:
        ltv_score = min(20.0, ltv_score + 3.0)
        reasons.append(f"{bandwidth_mbps}Mbps超高速口径(极度契合FRP反代与多路中转并发 +3)")
    elif bandwidth_mbps >= 1000:
        ltv_score = min(20.0, ltv_score + 2.0)
        reasons.append(f"{bandwidth_mbps}Mbps千兆口径(契合FRP反代 +2)")

    if ram_gb >= 2.0:
        ltv_score = min(20.0, ltv_score + 1.0)

    # 4. 入手价格与续费价格双轮驱动 (0 - 15 分)
    # 核心：全新机只看月续费；二手挂单必须同时看销售价格与续费价格！
    price_score = 8.0
    if item_type == "store":
        # 全新商品：买入即按续费价，严格按山哥价格标准
        if monthly_renew <= 10.0:
            price_score = 15.0
            reasons.append(f"一手真正超值月续费¥{monthly_renew:.2f}/月 (+15)")
        elif is_high_spec_powerhouse and monthly_renew <= 20.0:
            price_score = 14.0
            reasons.append(f"一手高配生产力机型月续费¥{monthly_renew:.2f}/月极具性价比 (+14)")
        elif monthly_renew <= 20.0:
            price_score = 10.0
            reasons.append(f"一手月续费¥{monthly_renew:.2f}/月 (20元档一般价位 +10)")
        elif monthly_renew <= 25.0:
            price_score = 4.0
            reasons.append(f"一手月续费¥{monthly_renew:.2f}/月偏贵 (-4)")
        elif is_high_spec_powerhouse:
            price_score = 10.0
            reasons.append("优质高配生产力机型续费价值良好 (+10)")
        else:
            price_score = 2.0
            reasons.append(f"一手月续费¥{monthly_renew:.2f}/月过高 (-6)")
    else:
        # 二手商品：双轮驱动（销售价看折价捡漏，续费价看养机压力）
        ask_score = 8.0
        if is_budget_asset and price_cny <= 10.0:
            ask_score = 15.0
        elif renew_price_cny > 0:
            discount = price_cny / renew_price_cny
            if discount <= 0.3:
                ask_score = 15.0
            elif discount <= 0.6:
                ask_score = 12.0
            elif discount > 1.3:
                ask_score = 4.0
            else:
                ask_score = 8.0

        # 续费价对二手价格分的调节权重
        if monthly_renew <= 2.0:
            price_score = 15.0
            reasons.append(f"双优：入手¥{price_cny:.1f} + 超级超值传家宝月供¥{monthly_renew:.1f} (+15)")
        elif monthly_renew <= 10.0:
            price_score = min(15.0, max(12.0, ask_score))
            reasons.append(f"双优：入手¥{price_cny:.1f} + 真正超值月续费¥{monthly_renew:.1f} (+{int(price_score)})")
        elif is_high_spec_powerhouse and monthly_renew <= 20.0:
            price_score = 14.0
            reasons.append(f"入手¥{price_cny:.1f}，高配生产力机型续费价值极高 (+{int(price_score)})")
        elif monthly_renew <= 20.0:
            price_score = min(11.0, ask_score)
            reasons.append(f"入手¥{price_cny:.1f}，月续费¥{monthly_renew:.1f} (好线路20元档成交价一般 +{int(price_score)})")
        elif monthly_renew <= 25.0:
            price_score = min(3.0, ask_score)
            reasons.append(f"入手¥{price_cny:.1f}，但月续费¥{monthly_renew:.1f}偏贵压低价格分 (+{int(price_score)})")
        elif is_high_spec_powerhouse:
            price_score = min(10.0, ask_score)
            reasons.append(f"入手¥{price_cny:.1f}，高配生产力机型长线价值良好 (+{int(price_score)})")
        else:
            price_score = 1.0
            reasons.append(f"注意：月续费¥{monthly_renew:.1f}过高，缺乏二手长期持有性价比 (+1)")

    # 5. 续费致命失血惩罚 (Renewal Drain Penalty)
    # 严格遵照山哥铁律：优化比较好、续费 25、流量 1000 的，只能打三四十分！
    # 超过 20 元平价线的机器，每多 1 元月供产生强烈失血惩罚
    renew_drain_penalty = 0.0
    if not is_high_spec_powerhouse and monthly_renew > 20.0:
        renew_drain_penalty = (monthly_renew - 20.0) * 4.2
        reasons.append(f"高月租失血惩罚：月供¥{monthly_renew:.1f}/月严重透支性价比 (-{int(renew_drain_penalty)})")

    raw_total = network_score + traffic_score + ltv_score + price_score - renew_drain_penalty
    final_score = max(5, min(100, int(round(raw_total))))

    USD_RATE = 7.25

    # 6. 山哥传家宝资产化估值模型与做市商策略 (Annualized Arbitrage Fair Valuation & Market-Making)
    # 严格遵照山哥三款核心主力做市买卖底牌：
    # IIJ: 成本¥350，出售底价¥500，买入目标≤¥200
    # ISP: 目标售价¥400，买入目标¥200-250
    # SB:  目标售价¥250-300，买入目标¥100-170
    t_ratio = max(0.1, flow_total_gb / 1000.0)
    ram_ratio = max(0.25, memory_mb / 1024.0)
    disk_ratio = max(0.5, disk_gb / 10.0)
    bw_ratio = max(0.5, bandwidth_mbps / 200.0)
    phi_capacity = 8.0 * (0.50 * t_ratio + 0.25 * ram_ratio + 0.10 * disk_ratio + 0.15 * min(10.0, bw_ratio))

    route_tier = str(route_info.get("tier") or "C")
    tier_k_map = {"S": 3.0, "A": 2.3, "B": 1.6, "C": 1.0}
    k_route = tier_k_map.get(route_tier, 1.0)

    combined = f"{name} {node_name} {detail} {intro}".lower()
    is_dual_isp = ("isp" in combined or "双isp" in combined) and bool(route_info.get("native_ip"))
    k_ip = 1.25 if is_dual_isp else (1.10 if route_info.get("native_ip") else 1.0)
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
                # 命中山哥扫货抄底买入区间！
                reasons.append(f"🚨 命中山哥扫货捡漏区间(标价¥{price_cny:.1f} ≤ 买入上限¥{target_buy_price:.0f})，极高优先级抄底！")
                final_score = max(final_score, 92)  # 提振至神机必抢！
                valuation_rationale = f"【极度超值扫货】当前标价¥{price_cny:.1f}落入山哥买入心仪价(≤¥{target_buy_price:.0f})，持有出售底价¥{target_sell_price:.0f}"
            else:
                valuation_rationale = f"【山哥关注型号】买入目标¥{target_buy_price:.0f}以内，心理出售底价¥{target_sell_price:.0f} (当前标价¥{price_cny:.1f})"
                if price_cny > target_sell_price:
                    reasons.append(f"标价¥{price_cny:.1f}高于山哥心仪卖出价(¥{target_sell_price:.0f})，溢价过高")
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
    comp_price = monthly_renew if item_type == "server" else price_cny
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

    points_per_yuan = round(eff_capability_points / eff_cost_ask, 2)
    points_per_usd = round(points_per_yuan * USD_RATE, 2)
    cost_per_point_cny = round(comp_price / max(1, final_score), 3)

    monthly_points_per_yuan = round(eff_capability_points / eff_cost_renew, 2)
    monthly_points_per_usd = round(monthly_points_per_yuan * USD_RATE, 2)

    if final_score >= 88:
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
    elif final_score >= 45:
        grade = "C"
        grade_label = "【性价比偏低】"
        badge_color = "orange"
    else:
        grade = "D"
        grade_label = "【拦截避坑】"
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
            "network_score": max(0, int(network_score)),
            "ltv_score": int(ltv_score),
            "traffic_score": int(traffic_score),
            "price_score": int(price_score),
        },
        "reasons": reasons,
    }
