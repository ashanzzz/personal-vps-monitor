from __future__ import annotations

import asyncio
from datetime import datetime
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest
from fastapi import HTTPException

from app import auth, main, public_api


class _RowsResult:
    def __init__(self, rows):
        self._rows = rows

    def scalars(self):
        return self

    def all(self):
        return self._rows


def test_require_read_api_key_rejects_when_disabled(monkeypatch):
    monkeypatch.delenv("PUBLIC_READ_TOKEN", raising=False)

    with pytest.raises(HTTPException) as exc:
        asyncio.run(auth.require_read_api_key(x_api_key="anything"))

    assert exc.value.status_code == 503


def test_require_read_api_key_accepts_matching_token(monkeypatch):
    monkeypatch.setenv("PUBLIC_READ_TOKEN", "read-secret")

    result = asyncio.run(auth.require_read_api_key(x_api_key="read-secret"))

    assert result == "ok"


def test_read_machines_returns_score_and_public_fields(monkeypatch):
    monkeypatch.setenv("PUBLIC_READ_TOKEN", "read-secret")

    row = SimpleNamespace(
        id=1,
        category="vps",
        plan_id="vps-1",
        area="JP",
        node="Tokyo",
        name="Alpha",
        stock=3,
        price_monthly=9900,
        price_yearly=99000,
        sale_price=None,
        cpu=2,
        memory_mb=4096,
        disk_gb=40,
        bandwidth_mbps=100,
        traffic_gb=1000,
        ipv4=1,
        available=True,
        ip_status="",
        updated_at=datetime(2026, 1, 1, 0, 0, 0),
    )

    db = SimpleNamespace(execute=AsyncMock(return_value=_RowsResult([row])))

    async def fake_get_all_settings(_db):
        return {
            "score_rules_cloud": {"cpu": 1, "memory": 1, "price_monthly": 0},
            "score_rules_ls": {},
            "score_rules_secondhand": {},
        }

    monkeypatch.setattr(public_api, "get_all_settings", fake_get_all_settings)
    monkeypatch.setattr(public_api, "find_official_equivalent", lambda *args, **kwargs: None)
    monkeypatch.setattr(public_api, "get_version_info", lambda: {"version": "0.11.0", "git_sha": "abc", "build_time": "now"})

    result = asyncio.run(
        public_api.read_machines(
            category=None,
            available_only=False,
            min_score=0,
            min_star=0,
            ip_status=None,
            sort="score_desc",
            _="ok",
            db=db,
        )
    )

    assert result["count"] == 1
    item = result["items"][0]
    assert item["plan_id"] == "vps-1"
    assert item["price_monthly"] == 9900
    assert item["star_rating"] >= 0
    assert "price_recommendation" in item
    assert item["official_match"] is None


def test_public_api_router_is_mounted_on_app():
    routes = {getattr(route, "path", None) for route in main.app.routes}
    assert "/api/read/v1/machines" in routes
    assert "/api/read/v1/meta" in routes
