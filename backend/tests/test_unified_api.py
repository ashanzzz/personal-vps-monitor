from __future__ import annotations

import pytest
from app.refresh_manager import RefreshManager
from app.main import app


def test_refresh_manager_logic():
    mgr = RefreshManager()
    state = mgr.get_state()
    assert state["mode"] == "slow"
    assert state["current_interval"] == 30

    burst_state = mgr.start_burst(duration=60, interval=3)
    assert burst_state["mode"] == "burst"
    assert burst_state["current_interval"] == 3
    assert burst_state["remaining_seconds"] <= 60
    assert burst_state["total_bursts"] == 1

    slow_state = mgr.stop_burst()
    assert slow_state["mode"] == "slow"
    assert slow_state["current_interval"] == 30


def test_unified_routes_registered():
    route_paths = {getattr(r, "path", "") for r in app.routes}
    assert "/api/unified/refresh/status" in route_paths
    assert "/api/unified/refresh/burst" in route_paths
    assert "/api/unified/refresh/slow" in route_paths
    assert "/api/unified/servers" in route_paths
    assert "/api/unified/servers/renew" in route_paths
    assert "/api/unified/servers/auto-renew" in route_paths
    assert "/api/unified/billing" in route_paths
    assert "/api/unified/billing/pay" in route_paths
    assert "/api/unified/store" in route_paths
    assert "/api/unified/store/buy" in route_paths
    assert "/api/unified/secondhand" in route_paths
    assert "/api/unified/secondhand/buy" in route_paths
