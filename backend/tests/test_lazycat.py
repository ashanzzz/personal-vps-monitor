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


# --- 【用户铁律】懒猫云实例绝不可销毁 ---

import pytest


@pytest.mark.parametrize("method,url", [
    ("POST", "/api/instances/123/destroy"),
    ("GET", "/api/instances/123/destroy-info"),
    ("POST", "/api/instances/123/delete-container"),
    ("DELETE", "/api/instances/123"),
    ("POST", "/api/instances/123/delete-and-refund"),
    ("POST", "/api/admin/instances/123/refund"),
])
def test_forbidden_destroy_paths_blocked(method, url):
    """销毁/删除实例本体的接口必须被硬拦截。"""
    with pytest.raises(lazycat_client.ForbiddenLazyCatOperation):
        lazycat_client._assert_not_forbidden(url, method)


@pytest.mark.parametrize("method,url", [
    ("GET", "/api/instances"),
    ("GET", "/api/instances/123"),
    ("POST", "/api/instances/123/restart"),
    ("POST", "/api/instances/123/stop"),
    ("POST", "/api/instances/123/suspend"),
    ("GET", "/api/instance-market/listings"),
    ("DELETE", "/api/instances/123/ports/80"),
    ("DELETE", "/api/instances/123/snapshots/7"),
    ("DELETE", "/api/instances/123/backups/3"),
])
def test_normal_paths_not_blocked(method, url):
    """正常运维接口（含删端口/快照/备份等子资源）不得被误拦。"""
    lazycat_client._assert_not_forbidden(url, method)


def test_no_destroy_method_on_client():
    """客户端不得存在任何销毁/删除实例的方法。"""
    forbidden_names = {
        "destroy_instance", "delete_instance", "delete_container",
        "delete_and_refund", "refund_instance", "get_destroy_info",
    }
    present = forbidden_names & set(dir(lazycat_client))
    assert not present, f"客户端出现了被禁止的销毁方法: {present}"

