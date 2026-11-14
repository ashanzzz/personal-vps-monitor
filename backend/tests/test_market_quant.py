"""
Tests for Market Quant & Asset Fingerprinting Engine.
"""
try:
    from app.market_quant import (
        compute_asset_fingerprint,
        aggregate_daily_kline_snapshot,
        diagnose_market_sentiment,
    )
except ImportError:
    from backend.app.market_quant import (
        compute_asset_fingerprint,
        aggregate_daily_kline_snapshot,
        diagnose_market_sentiment,
    )


def test_asset_fingerprint_normalization():
    # 1. 相同配置不同标题归一化测试
    fp1 = compute_asset_fingerprint(
        name="HKLite-One(番茄特供)",
        node_name="HKLite",
        area="中国香港",
        detail="BGP网络互联",
        cpu=1,
        memory_mb=1024,
        disk_gb=10,
        bandwidth_mbps=1000,
        flow_total_gb=1000,
        renew_price_cny=15.0,
        price_cny=45.0,
    )

    fp2 = compute_asset_fingerprint(
        name="HKLite-One",
        node_name="HKLite",
        area="中国香港",
        detail="香港直连",
        cpu=1,
        memory_mb=1024,
        disk_gb=10,
        bandwidth_mbps=1000,
        flow_total_gb=1000,
        renew_price_cny=15.0,
        price_cny=55.0,
    )

    assert fp1["asset_id"] == fp2["asset_id"]
    assert fp1["asset_id"] == "HK_BGP-1C1G10G-1000M-1000G-R15.00"

    # 2. 差异配置区分测试 (如双ISP vs 普通4837)
    fp_isp = compute_asset_fingerprint(
        name="LAX4837-ISPMini",
        node_name="LAX 4837",
        area="美国",
        detail="双ISP 原生IP",
        cpu=1,
        memory_mb=1024,
        disk_gb=10,
        bandwidth_mbps=1000,
        flow_total_gb=1200,
        renew_price_cny=1.90,
    )
    assert fp_isp["route_family"] == "US_AS4837_DUALISP"
    assert fp_isp["delta_traits"]["is_dual_isp"] is True
    assert fp_isp["renew_tag"] == "R1.90"


def test_daily_kline_aggregation_median():
    # 测试中位数而非平均数
    mock_items = [
        {"title": "JPSB-Mini", "node": "JPSB", "area": "日本", "intro": "软银", "cpu": 1, "memory_mb": 1024, "disk_gb": 5, "bandwidth": 1000, "flow": 1000, "renew_price_cny": 1.11, "price_cny": 120.0},
        {"title": "JPSB-Mini", "node": "JPSB", "area": "日本", "intro": "软银", "cpu": 1, "memory_mb": 1024, "disk_gb": 5, "bandwidth": 1000, "flow": 1000, "renew_price_cny": 1.11, "price_cny": 130.0},
        {"title": "JPSB-Mini", "node": "JPSB", "area": "日本", "intro": "软银", "cpu": 1, "memory_mb": 1024, "disk_gb": 5, "bandwidth": 1000, "flow": 1000, "renew_price_cny": 1.11, "price_cny": 130.0},
        {"title": "JPSB-Mini", "node": "JPSB", "area": "日本", "intro": "软银", "cpu": 1, "memory_mb": 1024, "disk_gb": 5, "bandwidth": 1000, "flow": 1000, "renew_price_cny": 1.11, "price_cny": 140.0},
        # 异常离谱高价挂单 (挂9999元)
        {"title": "JPSB-Mini", "node": "JPSB", "area": "日本", "intro": "软银", "cpu": 1, "memory_mb": 1024, "disk_gb": 5, "bandwidth": 1000, "flow": 1000, "renew_price_cny": 1.11, "price_cny": 9999.0},
    ]

    kline = aggregate_daily_kline_snapshot(mock_items, target_date="2026-11-03")
    assert len(kline) == 1
    row = kline[0]
    # 中位数应当坚决排斥 9999 离谱挂单，命中 130.0
    assert row["price_median"] == 130.0
    assert row["price_min"] == 120.0
    assert row["price_max"] == 9999.0
    assert row["active_listings"] == 5


def test_diagnose_market_sentiment():
    mock_kline = [
        {"asset_name": "JPSB 软银", "market_state": "DOWNTURN_ACCUMULATED", "premium_spread_pct": 35.0, "price_median": 200.0, "fair_model_value": 150.0, "active_listings": 12},
        {"asset_name": "HK BGP", "market_state": "DOWNTURN_ACCUMULATED", "premium_spread_pct": 40.0, "price_median": 50.0, "fair_model_value": 30.0, "active_listings": 15},
    ]
    diag = diagnose_market_sentiment(
        mock_kline,
        total_active_listings=280,
        tg_buy_count=5,
        tg_sell_count=35,
    )
    assert diag["market_phase"] == "BEARISH_DOWNTURN"
    assert diag["downturn_score"] > 60.0
    assert len(diag["downturn_signals"]) >= 2
