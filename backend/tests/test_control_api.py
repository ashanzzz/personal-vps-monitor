from __future__ import annotations

import asyncio
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest
from fastapi import HTTPException

from app import auth, control_api, main


class _RowsResult:
    def __init__(self, rows):
        self._rows = rows

    def scalars(self):
        return self

    def all(self):
        return self._rows


def test_require_control_api_key_rejects_when_disabled(monkeypatch):
    monkeypatch.delenv("PUBLIC_CONTROL_TOKEN", raising=False)

    with pytest.raises(HTTPException) as exc:
        asyncio.run(auth.require_control_api_key(x_api_key="anything"))

    assert exc.value.status_code == 503


def test_require_control_api_key_accepts_matching_token(monkeypatch):
    monkeypatch.setenv("PUBLIC_CONTROL_TOKEN", "control-secret")

    result = asyncio.run(auth.require_control_api_key(x_api_key="control-secret"))

    assert result == "ok"


def test_control_settings_view_hides_sensitive_fields():
    view = control_api.public_control_settings(
        {
            "akile_token": "secret-token",
            "tg_bot_token": "bot-secret",
            "tg_chat_id": "chat-secret",
            "webhook_url": "https://example.invalid/hook",
            "filter_min_score": 88,
            "filter_min_star": 4,
            "random_jitter_enabled": True,
            "score_rules_cloud": {"cpu": 1.0},
        }
    )

    assert "akile_token" not in view
    assert "tg_bot_token" not in view
    assert "tg_chat_id" not in view
    assert "webhook_url" not in view
    assert view["filter_min_score"] == 88
    assert view["filter_min_star"] == 4
    assert view["random_jitter_enabled"] is True


def test_apply_control_settings_updates_allowed_fields_and_reschedules(monkeypatch):
    captured: dict[str, object] = {}

    async def fake_update_settings(_db, data):
        captured["data"] = data
        return {
            "akile_token": "secret-token",
            "tg_bot_token": "bot-secret",
            "tg_chat_id": "chat-secret",
            "webhook_url": "https://example.invalid/hook",
            "filter_min_score": 90,
            "filter_min_star": 4,
            "score_rules_cloud": {"cpu": 2.0},
            "log_level": "DEBUG",
        }

    fake_main = SimpleNamespace(
        _reschedule_page_jobs=lambda settings: captured.setdefault("page_jobs", settings),
        _reschedule_account_job=lambda settings: captured.setdefault("account_job", settings),
        set_level=lambda level: captured.setdefault("level", level),
        _schedule_daily_checkin=AsyncMock(),
    )

    monkeypatch.setattr(control_api, "update_settings", fake_update_settings)
    monkeypatch.setattr(control_api, "_get_main_runtime", lambda: fake_main)

    result = asyncio.run(
        control_api.apply_control_settings(
            SimpleNamespace(),
            {
                "filter_min_score": 90,
                "filter_min_star": 4,
                "score_rules_cloud": {"cpu": 2.0},
                "akile_token": "ignored",
                "tg_bot_token": "ignored",
                "webhook_url": "ignored",
            },
        )
    )

    assert captured["data"] == {
        "filter_min_score": 90,
        "filter_min_star": 4,
        "score_rules_cloud": {"cpu": 2.0},
    }
    assert captured["page_jobs"]["filter_min_score"] == 90
    assert captured["account_job"]["filter_min_score"] == 90
    assert captured["level"] == "DEBUG"
    fake_main._schedule_daily_checkin.assert_awaited()
    assert "akile_token" not in result
    assert "tg_bot_token" not in result
    assert "tg_chat_id" not in result
    assert "webhook_url" not in result
    assert result["filter_min_score"] == 90
    assert result["filter_min_star"] == 4


def test_control_api_router_is_mounted_on_app():
    routes = {getattr(route, "path", None) for route in main.app.routes}
    assert "/api/control/v1/settings" in routes
    assert "/api/control/v1/poll/trigger" in routes
