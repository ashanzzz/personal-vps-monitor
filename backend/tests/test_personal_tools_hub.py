from __future__ import annotations

import pytest
from app.config import DEFAULTS
from app import acck_client
from app.main import app

def test_config_safe_buy_defaults():
    assert "acck_token" in DEFAULTS
    assert DEFAULTS["safe_buy_mode"] is True
    assert DEFAULTS["real_purchase_enabled"] is False


@pytest.mark.anyio
async def test_acck_buy_preview_is_simulation():
    res = await acck_client.buy_preview("mock_token", {"id": "test_service_1", "period": 1})
    assert res["status"] == "simulated"
    assert res["protected"] is True
    assert "防误购保护锁已生效" in res["msg"]


def test_routes_expose_personal_tools_hub_endpoints():
    route_paths = {getattr(r, "path", "") for r in app.routes}
    assert "/api/acck/account" in route_paths
    assert "/api/acck/account/refresh" in route_paths
    assert "/api/acck/checkin" in route_paths
    assert "/api/acck/servers" in route_paths
    assert "/api/acck/shop" in route_paths
    assert "/api/acck/buy-preview" in route_paths
    assert "/api/buy/{plan_id}" in route_paths
