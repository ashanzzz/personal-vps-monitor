import pytest
from app.sniper import resolve_search_prefix, item_matches_sniper_rule

def test_resolve_search_prefix():
    rule1 = {"tags_include": ["元旦快乐"]}
    assert resolve_search_prefix(rule1) == "HKLite-One"

    rule2 = {"tags_include": ["hklite-one(元旦快乐)"]}
    assert resolve_search_prefix(rule2) == "HKLite-One"

    rule3 = {"tags_include": ["jpiij-lite"]}
    assert resolve_search_prefix(rule3) == "JPIIJ"

    rule_explicit = {"search_prefix": "CustomPrefix"}
    assert resolve_search_prefix(rule_explicit) == "CustomPrefix"

def test_item_matches_sniper_rule():
    rule = {
        "tags_include": ["元旦快乐"],
        "sale_price_max": 3000,
    }
    
    # Matching item
    item_ok = {
        "name": "HKLite-One(元旦快乐)",
        "price": 3000,
        "status": 0
    }
    assert item_matches_sniper_rule(rule, item_ok) is True

    # Cheaper item
    item_cheap = {
        "name": "HKLite-One(元旦快乐)",
        "price": 2000,
        "status": 0
    }
    assert item_matches_sniper_rule(rule, item_cheap) is True

    # Too expensive
    item_expensive = {
        "name": "HKLite-One(元旦快乐)",
        "price": 3500,
        "status": 0
    }
    assert item_matches_sniper_rule(rule, item_expensive) is False

    # Status not 0 (e.g. sold)
    item_sold = {
        "name": "HKLite-One(元旦快乐)",
        "price": 2500,
        "status": 1
    }
    assert item_matches_sniper_rule(rule, item_sold) is False

    # Name doesn't match tag
    item_wrong_name = {
        "name": "HKLite-One(番茄特供)",
        "price": 2500,
        "status": 0
    }
    assert item_matches_sniper_rule(rule, item_wrong_name) is False

def test_item_matches_sniper_rule_with_specs():
    rule_spec = {
        "tags_include": ["元旦快乐"],
        "sale_price_max": 3000,
        "spec": {
            "cpu": {"min": 2, "max": 0},
            "memory": {"min": 2048, "max": 0},
            "disk": {"min": 10, "max": 0},
        }
    }

    # Matches all specs
    item_good = {
        "name": "HKLite-One(元旦快乐)",
        "price": 2800,
        "status": 0,
        "cpu": 2,
        "memory": 2048,
        "disk": 10,
    }
    assert item_matches_sniper_rule(rule_spec, item_good) is True

    # Failed cpu (1C instead of 2C)
    item_bad_cpu = {
        "name": "HKLite-One(元旦快乐)",
        "price": 2800,
        "status": 0,
        "cpu": 1,
        "memory": 2048,
        "disk": 10,
    }
    assert item_matches_sniper_rule(rule_spec, item_bad_cpu) is False

    # Failed memory (1024MB instead of 2048MB)
    item_bad_mem = {
        "name": "HKLite-One(元旦快乐)",
        "price": 2800,
        "status": 0,
        "cpu": 2,
        "memory": 1024,
        "disk": 10,
    }
    assert item_matches_sniper_rule(rule_spec, item_bad_mem) is False

    # ACCK style field names (memory_mb, disk_gb)
    item_acck_format = {
        "name": "HKLite-One(元旦快乐)",
        "price": 2800,
        "status": 0,
        "cpu": 2,
        "memory_mb": 2048,
        "disk_gb": 10,
    }
    assert item_matches_sniper_rule(rule_spec, item_acck_format) is True
