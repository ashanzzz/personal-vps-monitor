from __future__ import annotations

import asyncio
import json
from datetime import datetime
from types import SimpleNamespace

from app import main, poller, scoring


def test_compute_score_skips_invalid_price_monthly_string() -> None:
    machine = {
        "cpu": 4,
        "memory_mb": 4096,
        "price_monthly": "售¥199.00",
    }

    score = scoring.compute_score(
        machine,
        {
            "cpu": 50,
            "memory": 20,
            "price_monthly": -1,
        },
    )

    # cpu=4 -> 200, memory=4GB -> 80. Invalid monthly-price strings should be
    # ignored instead of crashing the whole /api/machines response.
    assert score == 280.0


def test_machine_dict_exposes_sale_price_and_tolerates_dirty_monthly_price() -> None:
    row = SimpleNamespace(
        id=1,
        category="secondhand",
        plan_id="sh-1",
        area="JP",
        node="Tokyo",
        name="Used VPS",
        stock=1,
        price_monthly="售¥199.00",
        price_yearly=None,
        sale_price=19900,
        cpu=2,
        memory_mb=2048,
        disk_gb=40,
        bandwidth_mbps=100,
        traffic_gb=1000,
        ipv4=1,
        available=True,
        ip_status="normal",
        updated_at=datetime(2026, 1, 1, 0, 0, 0),
    )

    payload = main._machine_dict(row, score=80.0)

    assert payload["sale_price"] == 19900
    assert payload["price_monthly"] == 0
    assert payload["price_recommendation"] is None


def test_poll_secondhand_uses_price_datas_for_monthly_and_separates_sale_price(monkeypatch) -> None:
    captured: dict = {}

    async def fake_get_secondhand_list(token: str, page: int = 1, page_size: int = 100) -> dict:
        assert token == "token-123"
        assert page_size == 100
        if page != 1:
            return {"status_code": 0, "total": 1, "list": []}
        return {
            "status_code": 0,
            "total": 1,
            "list": [
                {
                    "id": 9001,
                    "area_name": "JP",
                    "node_name": "Tokyo",
                    "plan_name": "Used VPS",
                    "price": "售¥199.00",
                    "price_datas": [
                        {"cycle": 1, "price": 9900},
                        {"cycle": 12, "price": 99000},
                    ],
                    "cpu": 2,
                    "memory": 2048,
                    "disk": 40,
                    "bandwidth": 100,
                    "flow": 1000,
                    "ipv4_num": 1,
                    "ipStatus": "normal",
                }
            ],
        }

    async def fake_upsert(db, category: str, plan_id: str, fields: dict):
        captured["category"] = category
        captured["plan_id"] = plan_id
        captured["fields"] = fields
        return []

    async def noop(*args, **kwargs):
        return None

    class DummySession:
        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, tb):
            return False

    monkeypatch.setattr(poller.api, "get_secondhand_list", fake_get_secondhand_list)
    monkeypatch.setattr(poller, "_upsert", fake_upsert)
    monkeypatch.setattr(poller, "_save_events", noop)
    monkeypatch.setattr(poller, "_touch_store_last_refresh", noop)
    monkeypatch.setattr(poller, "broadcast", noop)
    monkeypatch.setattr(poller, "SessionLocal", lambda: DummySession())

    asyncio.run(poller.poll_secondhand("token-123", {}, limit=1))

    assert captured["category"] == "secondhand"
    assert captured["plan_id"] == "9001"
    fields = captured["fields"]
    assert fields["price_monthly"] == 9900
    assert fields["price_yearly"] == 99000
    assert fields["sale_price"] == 19900
    assert json.loads(fields["extra"])["price"] == "售¥199.00"
