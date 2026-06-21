from __future__ import annotations

import asyncio
from datetime import datetime
from types import SimpleNamespace
from unittest.mock import AsyncMock

from app import buyer, main, poller, token_manager


class _AsyncSessionCtx:
    def __init__(self, db):
        self._db = db

    async def __aenter__(self):
        return self._db

    async def __aexit__(self, exc_type, exc, tb):
        return False


class _FixedDatetime(datetime):
    @classmethod
    def now(cls, tz=None):
        return datetime(2026, 6, 21, 12, 0, 0, tzinfo=tz)


def test_fastapi_docs_are_under_api_namespace():
    assert main.app.docs_url == "/api/docs"
    assert main.app.redoc_url == "/api/redoc"
    assert main.app.openapi_url == "/api/openapi.json"


def test_run_poll_does_not_synchronize_price_history(monkeypatch):
    monkeypatch.setattr(token_manager, "ensure_fresh", AsyncMock())
    monkeypatch.setattr(
        poller,
        "SessionLocal",
        lambda: _AsyncSessionCtx(SimpleNamespace()),
    )
    monkeypatch.setattr(
        poller,
        "get_all_settings",
        AsyncMock(
            return_value={
                "akile_token": "token",
                "vps_enabled": False,
                "ls_enabled": False,
                "secondhand_enabled": False,
            }
        ),
    )
    monkeypatch.setattr(poller, "poll_price_history", AsyncMock())
    monkeypatch.setattr(poller, "poll_vps", AsyncMock())
    monkeypatch.setattr(poller, "poll_ls", AsyncMock())
    monkeypatch.setattr(poller, "poll_secondhand", AsyncMock())

    asyncio.run(poller.run_poll())

    poller.poll_price_history.assert_not_called()


def test_run_page_poll_does_not_synchronize_price_history(monkeypatch):
    monkeypatch.setattr(token_manager, "ensure_fresh", AsyncMock())
    monkeypatch.setattr(
        poller,
        "SessionLocal",
        lambda: _AsyncSessionCtx(SimpleNamespace()),
    )
    monkeypatch.setattr(
        poller,
        "get_all_settings",
        AsyncMock(
            return_value={
                "akile_token": "token",
                "vps_enabled": True,
                "ls_enabled": False,
                "secondhand_enabled": False,
            }
        ),
    )
    fake_poll_vps = AsyncMock()
    monkeypatch.setattr(poller, "poll_price_history", AsyncMock())
    monkeypatch.setitem(poller._PAGE_POLLERS, "cloud", ("vps_enabled", fake_poll_vps))
    monkeypatch.setattr(buyer, "run_autobuy", AsyncMock())

    asyncio.run(poller.run_page_poll("cloud"))

    poller.poll_price_history.assert_not_called()
    fake_poll_vps.assert_awaited_once()
    buyer.run_autobuy.assert_awaited_once_with("cloud")


def test_page_poller_jobs_are_staggered_and_guarded(monkeypatch):
    calls: list[tuple[str, dict]] = []

    monkeypatch.setattr(main.scheduler, "get_job", lambda job_id: None)

    def fake_add_job(func, trigger, **kwargs):
        calls.append((kwargs["id"], {"trigger": trigger, **kwargs}))

    monkeypatch.setattr(main.scheduler, "add_job", fake_add_job)
    monkeypatch.setattr(main, "datetime", _FixedDatetime)

    main._reschedule_page_jobs(
        {
            "random_jitter_enabled": False,
            "cloud_buy": {"refresh_interval": 60},
            "ls_buy": {"refresh_interval": 60},
            "secondhand_buy": {"refresh_interval": 60},
        }
    )

    assert [job_id for job_id, _ in calls] == ["poll_cloud", "poll_ls", "poll_secondhand"]
    next_runs = [payload["next_run_time"] for _, payload in calls]
    assert next_runs[0] < next_runs[1] < next_runs[2]
    for _, payload in calls:
        assert payload["coalesce"] is True
        assert payload["max_instances"] == 1
        assert payload["misfire_grace_time"] >= 300


def test_support_jobs_are_separate_and_guarded(monkeypatch):
    calls: list[tuple[str, dict]] = []

    monkeypatch.setattr(main.scheduler, "get_job", lambda job_id: None)

    def fake_add_job(func, trigger, **kwargs):
        calls.append((kwargs["id"], {"trigger": trigger, **kwargs}))

    monkeypatch.setattr(main.scheduler, "add_job", fake_add_job)
    monkeypatch.setattr(main, "datetime", _FixedDatetime)

    main._schedule_support_jobs()

    ids = {job_id for job_id, _ in calls}
    assert {"token_refresh", "prune_events", "price_history_refresh"}.issubset(ids)
    for _, payload in calls:
        assert payload["coalesce"] is True
        assert payload["max_instances"] == 1
        assert payload["misfire_grace_time"] >= 300
        assert payload["next_run_time"] > _FixedDatetime.now()
