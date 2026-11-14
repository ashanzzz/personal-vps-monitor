from __future__ import annotations

from app.main import app
from app import lazycat_client


def test_lazycat_routes_registered():
    route_paths = {getattr(r, "path", "") for r in app.routes}
    assert "/api/lazycat/checkin" in route_paths
    assert "/api/lazycat/account" in route_paths
    assert "/api/lazycat/instances" in route_paths
    assert "/api/lazycat/market" in route_paths
    assert "/api/lazycat/store" in route_paths
    assert "/api/lazycat/buy/{listing_id}" in route_paths
    assert "/api/lazycat/lotteries" in route_paths


def test_lazycat_headers():
    h = lazycat_client._headers("test_token_xyz")
    assert h["Authorization"] == "Bearer test_token_xyz"
    assert "dash.lazycat.wiki" in h["Origin"]


def test_lazycat_headers_with_bearer():
    h = lazycat_client._headers("Bearer test_token_xyz")
    assert h["Authorization"] == "Bearer test_token_xyz"
