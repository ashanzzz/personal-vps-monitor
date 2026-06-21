from __future__ import annotations

from types import SimpleNamespace

from app import poller


def test_passes_filters_rejects_low_score_notifications():
    machine = SimpleNamespace(
        category="vps",
        cpu=0,
        memory_mb=0,
        disk_gb=10,
        bandwidth_mbps=100,
        traffic_gb=10,
        price_monthly=10000,
        sale_price=None,
    )
    event = {"category": "vps", "plan_id": "vps-1", "name": "Alpha", "area": "JP", "event_type": "new_listing"}
    settings = {
        "notify_scope": "all",
        "watched_plans": [],
        "filter_areas": [],
        "filter_name_include": "",
        "filter_name_exclude": "",
        "filter_max_price": 0,
        "filter_min_cpu": 0,
        "filter_min_memory_mb": 0,
        "filter_min_traffic_gb": 0,
        "filter_min_score": 200,
        "filter_min_star": 0,
        "score_rules_cloud": {"cpu": 100.0, "memory": 100.0, "price_monthly": 0.0},
        "score_rules_ls": {},
        "score_rules_secondhand": {},
    }

    assert poller._passes_filters(event, machine, settings) is False


def test_passes_filters_rejects_low_star_notifications():
    machine = SimpleNamespace(
        category="vps",
        cpu=0,
        memory_mb=0,
        disk_gb=10,
        bandwidth_mbps=100,
        traffic_gb=10,
        price_monthly=10000,
        sale_price=None,
    )
    event = {"category": "vps", "plan_id": "vps-1", "name": "Alpha", "area": "JP", "event_type": "new_listing"}
    settings = {
        "notify_scope": "all",
        "watched_plans": [],
        "filter_areas": [],
        "filter_name_include": "",
        "filter_name_exclude": "",
        "filter_max_price": 0,
        "filter_min_cpu": 0,
        "filter_min_memory_mb": 0,
        "filter_min_traffic_gb": 0,
        "filter_min_score": 0,
        "filter_min_star": 5,
        "score_rules_cloud": {"cpu": 1.0, "memory": 1.0, "price_monthly": 0.0},
        "score_rules_ls": {},
        "score_rules_secondhand": {},
    }

    assert poller._passes_filters(event, machine, settings) is False
