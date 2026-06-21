"""
Backend control API.

This namespace exposes a writable, authenticated control surface for trusted
automation. It keeps the control token separate from both the admin JWT and the
read-only API key.
"""
from __future__ import annotations

import asyncio
import importlib
import json
from typing import Any

from fastapi import APIRouter, Depends
from pydantic import BaseModel
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from .auth import require_control_api_key
from .config import DEFAULTS, get_all_settings, update_settings
from .database import AccountCache, get_db
from .poller import poll_account, run_poll, run_store_refresh
from .token_manager import refresh_now, token_status
from .version import get_version_info

router = APIRouter(prefix="/api/control/v1", tags=["control-api"])

_SENSITIVE_SETTINGS = {"akile_token", "tg_bot_token", "tg_chat_id", "webhook_url", "_auth_secret"}
_CONTROL_WRITABLE_KEYS = {key for key in DEFAULTS if key not in _SENSITIVE_SETTINGS}
_CAPABILITIES = [
    "settings",
    "poll",
    "store_refresh",
    "token_refresh",
    "account",
    "account_refresh",
]


class ControlStoreRefreshBody(BaseModel):
    page: str | None = None
    limit: int | None = None


def _get_main_runtime():
    return importlib.import_module("app.main")


def public_control_settings(settings: dict[str, Any]) -> dict[str, Any]:
    return {k: v for k, v in settings.items() if k not in _SENSITIVE_SETTINGS}


def _filter_control_payload(data: dict[str, Any]) -> dict[str, Any]:
    return {k: v for k, v in data.items() if k in _CONTROL_WRITABLE_KEYS}


async def _apply_runtime_side_effects(settings: dict[str, Any]) -> None:
    runtime = _get_main_runtime()
    runtime._reschedule_page_jobs(settings)
    runtime._reschedule_account_job(settings)
    runtime.set_level(settings.get("log_level", "INFO"))
    await runtime._schedule_daily_checkin(settings)


async def apply_control_settings(db: AsyncSession, data: dict[str, Any]) -> dict[str, Any]:
    allowed = _filter_control_payload(data)
    result = await update_settings(db, allowed)
    await _apply_runtime_side_effects(result)
    return public_control_settings(result)


async def _build_meta(db: AsyncSession) -> dict:
    settings = await get_all_settings(db)
    version = get_version_info()
    updated_at = settings.get("store_last_refresh")
    return {
        "version": version.get("version", ""),
        "git_sha": version.get("git_sha", ""),
        "build_time": version.get("build_time", ""),
        "updated_at": updated_at,
        "capabilities": list(_CAPABILITIES),
    }


async def _load_account_payload(db: AsyncSession) -> dict:
    row = await db.scalar(select(AccountCache).where(AccountCache.id == 1))
    if row is None or not row.data:
        return {
            "configured": False,
            "profile": None,
            "servers": [],
            "errors": ["cache_empty"],
            "updated_at": None,
        }
    payload = json.loads(row.data)
    payload["updated_at"] = row.updated_at.isoformat() if row.updated_at else None
    return payload


async def _refresh_account_cache(db: AsyncSession) -> dict:
    settings = await get_all_settings(db)
    token = settings.get("akile_token", "")
    if not token:
        return {
            "configured": False,
            "profile": None,
            "servers": [],
            "errors": ["no_token"],
            "updated_at": None,
        }
    await poll_account(token, settings)
    return await _load_account_payload(db)


@router.get("/meta")
async def control_meta(
    _: str = Depends(require_control_api_key),
    db: AsyncSession = Depends(get_db),
):
    return await _build_meta(db)


@router.get("/health")
async def control_health(
    _: str = Depends(require_control_api_key),
    db: AsyncSession = Depends(get_db),
):
    meta = await _build_meta(db)
    return {
        "status": "ok",
        "version": meta["version"],
        "updated_at": meta["updated_at"],
        "capabilities": meta["capabilities"],
    }


@router.get("/settings")
async def read_control_settings(
    _: str = Depends(require_control_api_key),
    db: AsyncSession = Depends(get_db),
):
    return public_control_settings(await get_all_settings(db))


@router.post("/settings")
async def write_control_settings(
    data: dict[str, Any],
    _: str = Depends(require_control_api_key),
    db: AsyncSession = Depends(get_db),
):
    return await apply_control_settings(db, data)


@router.post("/poll/trigger")
async def trigger_poll(
    _: str = Depends(require_control_api_key),
):
    asyncio.create_task(run_poll())
    return {"status": "triggered"}


@router.post("/store/refresh")
async def refresh_store(
    body: ControlStoreRefreshBody,
    _: str = Depends(require_control_api_key),
):
    page = body.page.strip() if body.page else None
    limit = body.limit if body.limit and body.limit > 0 else 100
    asyncio.create_task(run_store_refresh(page, limit=limit))
    return {"status": "triggered", "page": page or "all", "limit": limit}


@router.post("/token/refresh")
async def control_refresh_token(
    _: str = Depends(require_control_api_key),
):
    ok = await refresh_now(reason="manual-control")
    return {"status": "ok" if ok else "failed"}


@router.get("/token/status")
async def control_token_status(
    _: str = Depends(require_control_api_key),
):
    return await token_status()


@router.get("/account")
async def control_account(
    _: str = Depends(require_control_api_key),
    db: AsyncSession = Depends(get_db),
):
    return await _load_account_payload(db)


@router.post("/account/refresh")
async def control_refresh_account(
    _: str = Depends(require_control_api_key),
    db: AsyncSession = Depends(get_db),
):
    return await _refresh_account_cache(db)
