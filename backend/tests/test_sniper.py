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
