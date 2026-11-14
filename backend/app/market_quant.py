"""
Market Quant & Asset Fingerprinting Engine for Akile & ACCK Secondhand Assets.
Standardizes heterogeneous VPS listings into canonical financial asset SKUs,
computes daily robust median price distribution, tracks volume, and diagnoses market downturns.
"""

import math
import statistics
import time
from datetime import datetime
from typing import Any, Dict, List, Optional, Tuple

from sqlalchemy import select, and_
from sqlalchemy.ext.asyncio import AsyncSession

try:
    from app.database import MarketAssetDailyKline
    from app.route_analyzer import evaluate_cost_effectiveness
except ImportError:
    from backend.app.database import MarketAssetDailyKline
    from backend.app.route_analyzer import evaluate_cost_effectiveness


def _percentile(data: List[float], p: float) -> float:
    """Calculate percentile (0.0 to 1.0) on pre-sorted numeric list."""
    if not data:
        return 0.0
    if len(data) == 1:
        return float(data[0])
    k = (len(data) - 1) * p
    f = int(k)
    c = min(len(data) - 1, f + 1)
    d = k - f
    return float(data[f] + (data[c] - data[f]) * d)


def compute_asset_fingerprint(
    name: str = "",
    node_name: str = "",
    area: str = "",
    detail: str = "",
    intro: str = "",
    cpu: int = 1,
    memory_mb: float = 1024.0,
    disk_gb: float = 10.0,
    bandwidth_mbps: int = 1000,
    flow_total_gb: float = 1000.0,
    renew_price_cny: float = 0.0,
    price_cny: float = 0.0,
    due_time_unix: Optional[int] = None,
    platform: str = "all",
) -> Dict[str, Any]:
    """
    标的归类与资产指纹提取核心算法：
    将千奇百怪的商品标题，根据客观物理底座（线路家族、CPU/内存/硬盘规格、带宽口径、额定流量、月供阶梯）
    聚类映射为唯一的规范资产标的 (Base Asset SKU)，并解耦记录具体差异配置 (Delta Traits)。
    """
    combined = f"{name} {node_name} {area} {detail} {intro}".lower()

    # 1. 识别线路家族 (Route Family)
    is_dual_isp = ("isp" in combined or "双isp" in combined)
    if any(k in combined for k in ["9929", "as9929", "cu9929"]):
        route_family = "US_AS9929"
        route_family_cn = "美西 9929 顶级优化"
    elif any(k in combined for k in ["4837", "as4837", "cu169"]) and is_dual_isp:
        route_family = "US_AS4837_DUALISP"
        route_family_cn = "美西 4837 双ISP原生"
    elif any(k in combined for k in ["4837", "as4837", "cu169"]):
        route_family = "US_AS4837"
        route_family_cn = "美西 4837 骨干优化"
    elif any(k in combined for k in ["gia", "cn2", "cmin2"]):
        route_family = "US_GIA_CMIN2"
        route_family_cn = "美西 GIA/CMIN2 旗舰"
    elif any(k in combined for k in ["softbank", "sb", "软银", "jpsb"]):
        route_family = "JP_SOFTBANK"
        route_family_cn = "日本 软银特色直连"
    elif any(k in combined for k in ["iij", "jpiij"]):
        route_family = "JP_IIJ"
        route_family_cn = "日本 IIJ 特色直连"
    elif any(k in combined for k in ["cmi", "as58453"]) and "香港" in area:
        route_family = "HK_CMI"
        route_family_cn = "中国香港 CMI 骨干直连"
    elif "香港" in area or any(k in combined for k in ["hk", "hongkong"]):
        route_family = "HK_BGP"
        route_family_cn = "中国香港 BGP 互联"
    elif "台湾" in area or any(k in combined for k in ["tw", "taiwan"]):
        route_family = "TW_BGP"
        route_family_cn = "中国台湾 BGP 互联"
    elif "新加坡" in area or any(k in combined for k in ["sg", "singapore"]):
        route_family = "SG_BGP"
        route_family_cn = "新加坡 BGP 互联"
    elif "日本" in area or any(k in combined for k in ["jp", "japan"]):
        route_family = "JP_BGP"
        route_family_cn = "日本 BGP 互联"
    elif "美国" in area or any(k in combined for k in ["us", "lax", "sjc"]):
        route_family = "US_BGP"
        route_family_cn = "美国 BGP 国际互联"
    else:
        route_family = "GLOBAL_BGP"
        route_family_cn = "全球其他国际互联"

    # 2. 识别硬件规格档 (Spec Bucket: CPU/RAM/DISK)
    ram_gb = round(memory_mb / 1024.0, 1)
    if ram_gb <= 0.6:
        ram_tag = "512M"
    elif ram_gb <= 1.2:
        ram_tag = "1G"
    elif ram_gb <= 2.2:
        ram_tag = "2G"
    elif ram_gb <= 4.2:
        ram_tag = "4G"
    else:
        ram_tag = f"{int(ram_gb)}G"

    if disk_gb <= 6.0:
        disk_tag = "5G"
    elif disk_gb <= 12.0:
        disk_tag = "10G"
    elif disk_gb <= 25.0:
        disk_tag = "20G"
    elif disk_gb <= 50.0:
        disk_tag = "40G"
    else:
        disk_tag = f"{int(disk_gb)}G"

    spec_code = f"{cpu}C{ram_tag}{disk_tag}"

    # 3. 识别带宽口径档 (Bandwidth Bucket)
    if bandwidth_mbps <= 250:
        bw_tag = "200M"
    elif bandwidth_mbps <= 600:
        bw_tag = "500M"
    elif bandwidth_mbps <= 1500:
        bw_tag = "1000M"
    elif bandwidth_mbps <= 3000:
        bw_tag = "2500M"
    elif bandwidth_mbps <= 6000:
        bw_tag = "5000M"
    else:
        bw_tag = "10000M"

    # 4. 识别额定流量档 (Flow Bucket)
    if flow_total_gb <= 400:
        flow_tag = "300G"
    elif flow_total_gb <= 800:
        flow_tag = "600G"
    elif flow_total_gb <= 1100:
        flow_tag = "1000G"
    elif flow_total_gb <= 1500:
        flow_tag = "1200G"
    elif flow_total_gb <= 2200:
        flow_tag = "2000G"
    elif flow_total_gb <= 3500:
        flow_tag = "2500G"
    elif flow_total_gb <= 6000:
        flow_tag = "5000G"
    else:
        flow_tag = "10000G"

    # 5. 识别续费月供档位 (Renew Bucket - 核心金融生命线)
    r = round(float(renew_price_cny), 2)
    if r <= 0.05:
        renew_tag = "R0_FREE"
        renew_canonical = 0.0
    elif r <= 0.35:
        renew_tag = "R0.18"
        renew_canonical = 0.18
    elif r <= 1.50:
        renew_tag = "R1.11"
        renew_canonical = 1.11
    elif r <= 2.50:
        renew_tag = "R1.90"
        renew_canonical = 1.90
    elif r <= 6.00:
        renew_tag = "R5.00"
        renew_canonical = 5.00
    elif r <= 11.00:
        renew_tag = "R10.00"
        renew_canonical = 10.00
    elif r <= 16.00:
        renew_tag = "R15.00"
        renew_canonical = 15.00
    else:
        renew_tag = f"R{int(r)}"
        renew_canonical = r

    # 统一规范资产标的代号 (Canonical Asset ID)
    asset_id = f"{route_family}-{spec_code}-{bw_tag}-{flow_tag}-{renew_tag}"
    asset_name = f"{route_family_cn} ({spec_code}·{bw_tag}·{flow_tag}·供¥{renew_canonical:g})"

    # 6. 具体差异特征 (Delta Traits: 解释个案溢价/折价的来源)
    now_ts = int(time.time())
    days_remaining = max(0, int(((due_time_unix or now_ts) - now_ts) / 86400)) if due_time_unix else 15
    is_blocked = ("被墙" in combined or "gfw" in combined or "block" in combined)
    is_high_cpu = any(k in combined for k in ["7k62", "epyc", "7532", "5950x", "7950x"])

    is_acck_018_prerequisite = (renew_canonical <= 0.35 and platform == "acck")

    delta_traits = {
        "is_dual_isp": is_dual_isp,
        "is_ip_blocked": is_blocked,
        "days_remaining": days_remaining,
        "is_high_perf_cpu": is_high_cpu,
        "is_acck_018_prerequisite": is_acck_018_prerequisite,
        "raw_renew_cny": r,
    }

    return {
        "asset_id": asset_id,
        "asset_name": asset_name,
        "route_family": route_family,
        "route_family_cn": route_family_cn,
        "spec_code": spec_code,
        "bandwidth_tag": bw_tag,
        "flow_tag": flow_tag,
        "renew_tag": renew_tag,
        "renew_canonical": renew_canonical,
        "delta_traits": delta_traits,
    }


def aggregate_daily_kline_snapshot(
    parsed_items: List[Dict[str, Any]],
    target_date: Optional[str] = None,
) -> List[Dict[str, Any]]:
    """
    根据当前在售全量二手明细，执行日度聚合计算：
    相同配置聚为一类，严格以「价格中位数 (P50)」为基准，同时计算 P25 支撑位、P75 阻力位与存量深度。
    """
    if not target_date:
        target_date = datetime.utcnow().strftime("%Y-%m-%d")

    # 1. 聚类分组
    grouped: Dict[str, Dict[str, Any]] = {}
    for item in parsed_items:
        raw_price = float(item.get("price_cny") or 0.0)
        if raw_price <= 0:
            continue

        raw_renew = float(item.get("renew_price_cny") or 0.0)
        bw = int(item.get("bandwidth") or 1000)
        flow = float(item.get("flow") or 1000.0)
        cpu = int(item.get("cpu") or 1)
        mem = float(item.get("memory_mb") or 1024.0)
        disk = float(item.get("disk_gb") or 10.0)
        due_unix = item.get("due_time")

        fp = compute_asset_fingerprint(
            name=item.get("title", ""),
            node_name=item.get("node", ""),
            area=item.get("area", ""),
            detail=item.get("intro", ""),
            intro=item.get("intro", ""),
            cpu=cpu,
            memory_mb=mem,
            disk_gb=disk,
            bandwidth_mbps=bw,
            flow_total_gb=flow,
            renew_price_cny=raw_renew,
            price_cny=raw_price,
            due_time_unix=due_unix,
            platform=item.get("platform", "all"),
        )
        aid = fp["asset_id"]

        if aid not in grouped:
            grouped[aid] = {
                "fingerprint": fp,
                "prices": [],
                "items": [],
            }
        grouped[aid]["prices"].append(raw_price)
        grouped[aid]["items"].append(item)

    # 2. 统计各标的中位数与公允估值
    kline_rows: List[Dict[str, Any]] = []
    for aid, gdata in grouped.items():
        prices = sorted(gdata["prices"])
        count = len(prices)
        if count == 0:
            continue

        fp = gdata["fingerprint"]
        med_price = round(float(statistics.median(prices)), 2)
        p25_price = round(_percentile(prices, 0.25), 2)
        p75_price = round(_percentile(prices, 0.75), 2)
        min_p = round(prices[0], 2)
        max_p = round(prices[-1], 2)

        # 取样机型进行模型公允身价验证
        sample_item = gdata["items"][0]
        eval_sample = evaluate_cost_effectiveness(
            item_type="secondhand",
            price_cny=med_price,
            renew_price_cny=fp["renew_canonical"],
            cpu=int(sample_item.get("cpu") or 1),
            memory_mb=float(sample_item.get("memory_mb") or 1024),
            disk_gb=float(sample_item.get("disk_gb") or 10),
            bandwidth_mbps=int(sample_item.get("bandwidth") or 1000),
            flow_total_gb=float(sample_item.get("flow") or 1000),
            name=sample_item.get("title", ""),
            node_name=sample_item.get("node", ""),
            area=sample_item.get("area", ""),
        )
        fair_val = float(eval_sample.get("fair_market_value_cny") or 0.0)

        # 溢价偏离度 (中位数 vs 公允身价)
        if fair_val > 0:
            spread_pct = round((med_price - fair_val) / fair_val * 100.0, 1)
        else:
            spread_pct = 0.0

        # 标的市场状态诊断
        if count >= 8 and spread_pct > 25.0:
            m_state = "DOWNTURN_ACCUMULATED"  # 存量积压滞销
        elif spread_pct < -15.0:
            m_state = "UNDERVALUED_OVERSOLD"  # 严重超卖/捡漏
        elif spread_pct > 40.0:
            m_state = "OVERVALUED_BUBBLE"     # 高位泡沫
        else:
            m_state = "NORMAL_BALANCED"       # 正常平衡

        kline_rows.append({
            "asset_id": aid,
            "date": target_date,
            "platform": "all",
            "asset_name": fp["asset_name"],
            "route_family": fp["route_family"],
            "specs_code": fp["spec_code"],
            "monthly_renew_cny": fp["renew_canonical"],
            "price_median": med_price,
            "price_p25": p25_price,
            "price_p75": p75_price,
            "price_min": min_p,
            "price_max": max_p,
            "active_listings": count,
            "inferred_sales": 0,
            "inferred_turnover_cny": 0.0,
            "turnover_rate": 0.0,
            "fair_model_value": fair_val,
            "premium_spread_pct": spread_pct,
            "market_state": m_state,
        })

    # 按盘口在售数量与重要度排序
    kline_rows.sort(key=lambda x: (x["active_listings"], x["price_median"]), reverse=True)
    return kline_rows


def diagnose_market_sentiment(
    kline_rows: List[Dict[str, Any]],
    total_active_listings: int = 262,
    tg_buy_count: int = 12,
    tg_sell_count: int = 48,
) -> Dict[str, Any]:
    """
    AI 股市化多空情绪与低迷诊断算法：
    综合存销比、标的中位数溢价偏离率、TG群买卖压力比，输出量化景气指数与低迷警报。
    """
    # 1. 计算大盘总积压度 (正常存量 100~160 台，超过 220 视为高位积压)
    inventory_stress = min(100.0, round(max(0.0, (total_active_listings - 120) / 180.0 * 100.0), 1))

    # 2. TG 买卖多空意向比 (Buy/Sell Ratio)
    tg_ratio = round(tg_buy_count / max(1.0, tg_sell_count), 2)

    # 3. 统计全盘标的溢价状态分布
    accumulated_count = sum(1 for r in kline_rows if r["market_state"] == "DOWNTURN_ACCUMULATED")
    oversold_count = sum(1 for r in kline_rows if r["market_state"] == "UNDERVALUED_OVERSOLD")
    bubble_count = sum(1 for r in kline_rows if r["market_state"] == "OVERVALUED_BUBBLE")

    # 4. 判定大盘宏观低迷程度
    # 低迷得分 (0-100): 分数越高越低迷冷清
    downturn_score = 0.0
    downturn_score += inventory_stress * 0.45  # 存量积压占比 45%
    if tg_ratio < 0.35:
        downturn_score += 35.0  # 买盘枯竭 +35分
    elif tg_ratio < 0.60:
        downturn_score += 20.0
    if accumulated_count >= 5:
        downturn_score += 20.0  # 多标的滞销 +20分

    downturn_score = min(100.0, round(downturn_score, 1))

    # 状态标签
    if downturn_score >= 65.0:
        market_phase = "BEARISH_DOWNTURN"
        phase_label = "🔴 深度低迷·买盘观望"
        summary = "市场处于明显的低迷停滞期：挂牌量高位积压，买家多空比偏低，多款标的面临流动性不足风险。"
    elif downturn_score >= 45.0:
        market_phase = "CONSOLIDATION"
        phase_label = "🟡 震荡整理·结构分化"
        summary = "市场整体处于震荡分化阶段：普通国际线路供过于求微跌，优质双ISP与特色优化标的相对抗跌。"
    else:
        market_phase = "ACTIVE_BULLISH"
        phase_label = "🟢 活跃放量·买盘旺盛"
        summary = "市场交投活跃，流转速度快，低价二手小鸡上线即被吸收。"

    # 提取量化低迷信号列表
    signals = []
    if total_active_listings >= 220:
        signals.append(f"⚠️ 盘口高位积压：当前总在售 {total_active_listings} 台 (超过220台警戒线)")
    if tg_ratio <= 0.35:
        signals.append(f"📉 买盘动能疲软：TG群收/出比率仅为 {tg_ratio} (出货帖远多于求购帖)")
    if accumulated_count > 0:
        signals.append(f"📦 滞销标的占比过半：全网有 {accumulated_count} 种主流标的中位挂牌价严重偏离公允价")

    # 挑选出最值得买入的超卖捡漏标的
    oversold_picks = [
        {
            "asset_name": r["asset_name"],
            "price_median": r["price_median"],
            "fair_value": r["fair_model_value"],
            "discount_pct": abs(r["premium_spread_pct"]),
            "count": r["active_listings"],
        }
        for r in kline_rows if r["premium_spread_pct"] < 0
    ][:5]

    return {
        "market_phase": market_phase,
        "phase_label": phase_label,
        "downturn_score": downturn_score,
        "inventory_stress_pct": inventory_stress,
        "tg_sentiment_ratio": tg_ratio,
        "total_active_listings": total_active_listings,
        "benchmark_types_count": len(kline_rows),
        "accumulated_types_count": accumulated_count,
        "oversold_types_count": oversold_count,
        "summary": summary,
        "downturn_signals": signals,
        "oversold_picks": oversold_picks,
    }


async def save_daily_kline_records(
    db: AsyncSession,
    kline_rows: List[Dict[str, Any]],
) -> int:
    """
    保存或更新日度 K 线中位数记录到 SQLite，确保 (asset_id, date) 唯一幂等更新。
    """
    count = 0
    for r in kline_rows:
        aid = r["asset_id"]
        dt = r["date"]
        stmt = select(MarketAssetDailyKline).where(
            and_(MarketAssetDailyKline.asset_id == aid, MarketAssetDailyKline.date == dt)
        )
        res = await db.execute(stmt)
        exist = res.scalar_one_or_none()

        if exist:
            exist.price_median = r["price_median"]
            exist.price_p25 = r["price_p25"]
            exist.price_p75 = r["price_p75"]
            exist.price_min = r["price_min"]
            exist.price_max = r["price_max"]
            exist.active_listings = r["active_listings"]
            exist.fair_model_value = r["fair_model_value"]
            exist.premium_spread_pct = r["premium_spread_pct"]
            exist.market_state = r["market_state"]
        else:
            new_row = MarketAssetDailyKline(
                asset_id=aid,
                date=dt,
                platform=r.get("platform", "all"),
                asset_name=r["asset_name"],
                route_family=r["route_family"],
                specs_code=r["specs_code"],
                monthly_renew_cny=r["monthly_renew_cny"],
                price_median=r["price_median"],
                price_p25=r["price_p25"],
                price_p75=r["price_p75"],
                price_min=r["price_min"],
                price_max=r["price_max"],
                active_listings=r["active_listings"],
                inferred_sales=r.get("inferred_sales", 0),
                inferred_turnover_cny=r.get("inferred_turnover_cny", 0.0),
                turnover_rate=r.get("turnover_rate", 0.0),
                fair_model_value=r["fair_model_value"],
                premium_spread_pct=r["premium_spread_pct"],
                market_state=r["market_state"],
            )
            db.add(new_row)
        count += 1

    await db.commit()
    return count
