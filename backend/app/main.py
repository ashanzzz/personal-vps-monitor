"""
FastAPI application entry point.
"""
from __future__ import annotations

import asyncio
import json
import logging
import os
from contextlib import asynccontextmanager
from datetime import datetime

import httpx
from apscheduler.schedulers.asyncio import AsyncIOScheduler
from fastapi import Depends, FastAPI, HTTPException, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from sqlalchemy import desc, select
from sqlalchemy.ext.asyncio import AsyncSession

from . import auth
from .auth import require_auth
from .config import PAGE_KEYS, get_all_settings, update_settings
from .database import AccountCache, AlertEvent, MachineSnapshot, PriceHistory, get_db, init_db
from .logging_setup import read_log_tail, set_level, setup_logging
from .match import find_official_equivalent
from .poller import poll_account, run_page_poll, run_poll
from .scoring import compute_score
from .token_manager import ensure_fresh, refresh_now, token_status
from .version import __version__, get_version_info
from .ws import broadcast, connect, disconnect
from . import akile_client

setup_logging()
logger = logging.getLogger(__name__)

scheduler = AsyncIOScheduler()
_poll_interval: int = 60  # seconds, updated from DB on startup


def _reschedule_page_jobs(settings: dict) -> None:
    """(Re)schedule one poll job per page using its independent interval."""
    for page in PAGE_KEYS:
        cfg = settings.get(f"{page}_buy") or {}
        interval = max(5, int(cfg.get("refresh_interval", 60) or 60))
        job_id = f"poll_{page}"
        if scheduler.get_job(job_id):
            scheduler.reschedule_job(job_id, trigger="interval", seconds=interval)
        else:
            scheduler.add_job(
                run_page_poll, "interval", seconds=interval, id=job_id,
                args=[page], next_run_time=datetime.now(),
            )
        logger.info("page poller %s scheduled interval=%ds", page, interval)


async def _run_account_poll() -> None:
    """Refresh the account cache (balance + profile + own servers) on its own
    independent interval. Keeps the token alive first, like the page pollers."""
    from .database import SessionLocal
    try:
        await ensure_fresh()
    except Exception:
        logger.exception("ensure_fresh failed during account poll (continuing)")

    async with SessionLocal() as db:
        settings = await get_all_settings(db)
    token = settings.get("akile_token") or ""
    if not token:
        return
    try:
        await poll_account(token, settings)
    except Exception:
        logger.exception("account poll failed")


def _reschedule_account_job(settings: dict) -> None:
    """(Re)schedule the account-cache poll using account_refresh_interval."""
    interval = max(30, int(settings.get("account_refresh_interval", 300) or 300))
    job_id = "poll_account"
    if scheduler.get_job(job_id):
        scheduler.reschedule_job(job_id, trigger="interval", seconds=interval)
    else:
        scheduler.add_job(
            _run_account_poll, "interval", seconds=interval, id=job_id,
            next_run_time=datetime.now(),
        )
    logger.info("account poller scheduled interval=%ds", interval)


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Init DB
    await init_db()

    # Provision the admin account from env if configured and none exists yet.
    await auth.provision_from_env()

    # Load interval from settings
    from .database import SessionLocal
    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    # If token is available in env (first run), save it
    env_token = os.getenv("AKILE_TOKEN", "")
    if env_token and not settings.get("akile_token"):
        async with SessionLocal() as db:
            from .config import set_setting
            await set_setting(db, "akile_token", env_token)

    # Apply persisted log level.
    set_level(settings.get("log_level", "INFO"))
    logger.info("app.start version=%s", __version__)

    # Per-page pollers (each with its own independent interval). These also
    # drive auto-buy.
    _reschedule_page_jobs(settings)

    # Account-page cache poller (balance + profile + own servers) on its own
    # interval so the account page reads cache instead of hitting akile on load.
    _reschedule_account_job(settings)

    # Keep the Akile token alive entirely inside the backend: check hourly,
    # refresh when within the expiry margin. Runs an immediate check on startup.
    scheduler.add_job(ensure_fresh, "interval", hours=1, id="token_refresh",
                      next_run_time=datetime.now())
    # Prune old events daily to keep the DB lean.
    scheduler.add_job(_prune_events, "interval", hours=24, id="prune_events",
                      next_run_time=datetime.now())
    # Daily checkin for AK coins (08:01 Asia/Shanghai).
    from apscheduler.triggers.cron import CronTrigger
    tz = settings.get("timezone", "Asia/Shanghai")
    scheduler.add_job(
        _daily_checkin, CronTrigger(hour=8, minute=1, timezone=tz),
        id="daily_checkin",
    )
    scheduler.start()
    logger.info("Scheduler started (per-page pollers active)")

    yield

    scheduler.shutdown()


async def _daily_checkin() -> None:
    """Automatic daily checkin for AK coins."""
    from .database import SessionLocal

    try:
        await ensure_fresh()
    except Exception:
        logger.exception("ensure_fresh failed during daily checkin (continuing)")

    async with SessionLocal() as db:
        settings = await get_all_settings(db)
    token = settings.get("akile_token") or ""
    if not token:
        logger.info("daily_checkin: no token configured, skipping")
        return

    try:
        result = await akile_client.checkin(token)
        status_code = result.get("status_code")
        status_msg = result.get("status_msg", "")
        if status_code == 0:
            logger.info("daily_checkin: success – %s", status_msg)
        elif status_code == 1:
            logger.info("daily_checkin: already signed – %s", status_msg)
        else:
            logger.warning("daily_checkin: unexpected response: %s", result)
    except Exception:
        logger.exception("daily_checkin failed")


async def _prune_events() -> None:
    """Delete AlertEvent rows older than event_retention_days."""
    from datetime import timedelta

    from .database import SessionLocal

    async with SessionLocal() as db:
        settings = await get_all_settings(db)
        days = settings.get("event_retention_days", 90)
        try:
            days = int(days)
        except (TypeError, ValueError):
            days = 90
        if days <= 0:
            return
        cutoff = datetime.utcnow() - timedelta(days=days)
        from sqlalchemy import delete
        await db.execute(delete(AlertEvent).where(AlertEvent.created_at < cutoff))
        await db.commit()
        logger.info("Pruned events older than %d days", days)


app = FastAPI(title="Akile Monitor", lifespan=lifespan)

# CORS: default to a permissive setup for LAN use, but allow locking down to a
# comma-separated allowlist via ALLOWED_ORIGINS (e.g. "https://akile.example.com").
_origins_env = os.getenv("ALLOWED_ORIGINS", "").strip()
_allow_origins = [o.strip() for o in _origins_env.split(",") if o.strip()] or ["*"]
app.add_middleware(
    CORSMiddleware,
    allow_origins=_allow_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# ---------------------------------------------------------------------------
# Auth
# ---------------------------------------------------------------------------

class LoginBody(BaseModel):
    username: str
    password: str


class SetupBody(BaseModel):
    username: str
    password: str


class ChangePwBody(BaseModel):
    old_password: str
    new_password: str


@app.get("/api/auth/status")
async def auth_status():
    """Whether an admin exists yet — drives login vs first-time setup screen."""
    return {"initialized": await auth.admin_exists()}


@app.post("/api/auth/setup")
async def auth_setup(body: SetupBody, db: AsyncSession = Depends(get_db)):
    """One-time admin creation, only allowed when no admin exists."""
    if await auth.get_admin(db) is not None:
        raise HTTPException(status_code=400, detail="管理员已存在")
    if not body.username or len(body.password) < 6:
        raise HTTPException(status_code=400, detail="用户名必填，密码至少 6 位")
    await auth.create_admin(db, body.username, body.password)
    token = await auth.issue_token(db, body.username)
    return {"token": token, "username": body.username}


@app.post("/api/auth/login")
async def auth_login(body: LoginBody, db: AsyncSession = Depends(get_db)):
    admin = await auth.get_admin(db)
    # Constant-ish behaviour: don't reveal whether the username exists.
    if admin is None or admin.username != body.username or not auth.verify_password(
        body.password, admin.password_hash
    ):
        raise HTTPException(status_code=401, detail="用户名或密码错误")
    token = await auth.issue_token(db, admin.username)
    return {"token": token, "username": admin.username}


@app.post("/api/auth/change-password")
async def auth_change_password(
    body: ChangePwBody,
    user: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
):
    admin = await auth.get_admin(db)
    if admin is None or not auth.verify_password(body.old_password, admin.password_hash):
        raise HTTPException(status_code=400, detail="原密码错误")
    if len(body.new_password) < 6:
        raise HTTPException(status_code=400, detail="新密码至少 6 位")
    await auth.change_password(db, admin.username, body.new_password)
    return {"status": "ok"}


# ---------------------------------------------------------------------------
# WebSocket
# ---------------------------------------------------------------------------

@app.websocket("/ws")
async def websocket_endpoint(ws: WebSocket, token: str = ""):
    # Authenticate via ?token=... query param before accepting the socket.
    if not await auth.verify_ws_token(token):
        await ws.close(code=4401)
        return
    await connect(ws)
    try:
        while True:
            await ws.receive_text()  # keep-alive ping support
    except WebSocketDisconnect:
        disconnect(ws)


# ---------------------------------------------------------------------------
# Settings
# ---------------------------------------------------------------------------

@app.get("/api/settings")
async def get_settings(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    return await get_all_settings(db)


@app.post("/api/settings")
async def save_settings(data: dict, _: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    result = await update_settings(db, data)

    # Reschedule per-page pollers (their intervals may have changed).
    _reschedule_page_jobs(result)

    # Reschedule the account-cache poller (account_refresh_interval may have changed).
    _reschedule_account_job(result)

    # Apply log level live.
    set_level(result.get("log_level", "INFO"))

    return result


@app.get("/api/version")
async def get_version():
    return get_version_info()


@app.get("/api/logs/app")
async def get_app_log(lines: int = 500, _: str = Depends(require_auth)):
    """Return the tail of the persistent app log for in-UI debugging."""
    lines = max(1, min(lines, 5000))
    return {"lines": lines, "content": read_log_tail(lines)}


@app.post("/api/poll/trigger")
async def trigger_poll(_: str = Depends(require_auth)):
    """Manually trigger a poll cycle."""
    asyncio.create_task(run_poll())
    return {"status": "triggered"}


@app.post("/api/token/refresh")
async def manual_refresh_token(_: str = Depends(require_auth)):
    """Manually force an Akile token refresh (writes new token to DB)."""
    ok = await refresh_now(reason="manual")
    return {"status": "ok" if ok else "failed"}


@app.get("/api/token/status")
async def get_token_status(_: str = Depends(require_auth)):
    """Report the current token's expiry/health for the UI."""
    return await token_status()


class AkileLoginBody(BaseModel):
    username: str
    password: str
    ga_code: str = ""  # optional 2FA code (step 2 only)


@app.post("/api/akile-login")
async def akile_login(body: AkileLoginBody, _: str = Depends(require_auth),
                      db: AsyncSession = Depends(get_db)):
    """Log into Akile with username/password (+ optional 2FA code).

    Two-step flow when 2FA is enabled:
      1. POST with username+password only → if Akile requires 2FA, returns
         {need_2fa: true, message: "..."} so the frontend can show a modal.
      2. POST again with username+password+ga_code → completes login.

    On success the JWT is persisted as the akile_token setting.
    """
    if not body.username or not body.password:
        raise HTTPException(status_code=400, detail="用户名和密码不能为空")

    login_payload: dict = {"email": body.username, "password": body.password}
    if body.ga_code.strip():
        login_payload["ga"] = body.ga_code.strip()

    headers = {
        "Origin": "https://akile.ai",
        "Referer": "https://akile.ai/",
        "User-Agent": (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
            "Chrome/126.0 Safari/537.36"
        ),
        "Accept": "application/json, text/plain, */*",
        "Content-Type": "application/json",
    }

    try:
        async with httpx.AsyncClient(timeout=25) as c:
            r = await c.post(
                "https://api.akile.ai/api/v1/user/login",
                json=login_payload,
                headers=headers,
            )
            r.raise_for_status()
            data = r.json()
    except httpx.HTTPStatusError as e:
        raise HTTPException(
            status_code=502,
            detail=f"Akile 登录接口返回错误 (HTTP {e.response.status_code})",
        )
    except httpx.RequestError:
        raise HTTPException(status_code=502, detail="无法连接 Akile 登录接口")

    status_code = data.get("status_code")
    status_msg = data.get("status_msg", "")

    # Detect 2FA requirement BEFORE checking status_code — Akile returns
    # status_code=0 even when 2FA is pending (only status_msg indicates it).
    if not body.ga_code.strip() and _is_2fa_required(status_msg):
        return {"need_2fa": True, "message": status_msg or "请输入二次验证码"}

    if status_code != 0:
        # Map known error messages to user-friendly hints
        detail = status_msg or "登录失败"
        if "密码" in detail:
            detail = "密码错误，请检查后重试"
        elif "用户" in detail or "不存在" in detail:
            detail = "用户名不存在"
        raise HTTPException(status_code=400, detail=detail)

    token = (data.get("data") or {}).get("token", "")
    if not token or token.count(".") != 2:
        raise HTTPException(
            status_code=502,
            detail=f"登录成功但未获取到有效 Token，响应：{status_msg}",
        )

    # Persist the new token
    from .config import set_setting
    await set_setting(db, "akile_token", token)

    # Return fresh token status
    return await token_status()


def _is_2fa_required(msg: str) -> bool:
    """Heuristic: does the Akile status_msg indicate 2FA is needed?"""
    if not msg:
        return False
    keywords = ["验证", "2FA", "GA", "ga", "两步", "动态码", "authenticator", "totp"]
    return any(kw in msg for kw in keywords)


@app.get("/api/account")
async def get_account(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Return the cached account snapshot (profile + balance + own servers).

    Reads from the single-row account_cache table that the account poller keeps
    fresh on its own interval (account_refresh_interval, default 300s), so
    loading the page never blocks on an upstream call. When no cache exists yet
    (e.g. fresh install before the first poll), populate it once on demand.
    """
    settings = await get_all_settings(db)
    token = settings.get("akile_token", "")
    if not token:
        return {"configured": False, "profile": None, "servers": [],
                "errors": ["no_token"], "updated_at": None}

    row = await db.scalar(select(AccountCache).where(AccountCache.id == 1))
    if row is None or not row.data:
        # Cold cache: fetch once so the page isn't empty on first visit.
        await poll_account(token, settings)
        row = await db.scalar(select(AccountCache).where(AccountCache.id == 1))
        if row is None or not row.data:
            return {"configured": True, "profile": None, "servers": [],
                    "errors": ["cache_empty"], "updated_at": None}

    payload = json.loads(row.data)
    payload["updated_at"] = row.updated_at.isoformat() if row.updated_at else None
    return payload


@app.post("/api/account/refresh")
async def refresh_account(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Force an immediate live fetch and update the account cache, then return it."""
    settings = await get_all_settings(db)
    token = settings.get("akile_token", "")
    if not token:
        return {"configured": False, "profile": None, "servers": [],
                "errors": ["no_token"], "updated_at": None}

    await poll_account(token, settings)
    row = await db.scalar(select(AccountCache).where(AccountCache.id == 1))
    if row is None or not row.data:
        return {"configured": True, "profile": None, "servers": [],
                "errors": ["cache_empty"], "updated_at": None}
    payload = json.loads(row.data)
    payload["updated_at"] = row.updated_at.isoformat() if row.updated_at else None
    return payload


@app.post("/api/checkin")
async def manual_checkin(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Manually trigger the daily AK-coin checkin, returning Akile's raw response."""
    settings = await get_all_settings(db)
    token = settings.get("akile_token", "")
    if not token:
        return {"error": "No token configured"}
    try:
        return await akile_client.checkin(token)
    except httpx.HTTPError as e:
        return {"error": str(e)}


# ---------------------------------------------------------------------------
# Machines
# ---------------------------------------------------------------------------

@app.get("/api/machines")
async def list_machines(
    category: str | None = None,
    available_only: bool = False,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
):
    q = select(MachineSnapshot)
    if category:
        q = q.where(MachineSnapshot.category == category)
    if available_only:
        q = q.where(MachineSnapshot.available == True)  # noqa: E712
    q = q.order_by(MachineSnapshot.category, MachineSnapshot.area, MachineSnapshot.name)
    rows = (await db.execute(q)).scalars().all()

    # Per-page scoring rules (independent weights per category). category -> key.
    settings = await get_all_settings(db)
    score_rules_by_cat = {
        "vps": settings.get("score_rules_cloud"),
        "ls": settings.get("score_rules_ls"),
        "secondhand": settings.get("score_rules_secondhand"),
    }

    # For secondhand rows, find an equivalent brand-new official plan so the UI
    # can show a "official new one exists" comparison. Fetch official plans once.
    has_secondhand = any(r.category == "secondhand" for r in rows)
    official: list[MachineSnapshot] = []
    if has_secondhand:
        official = (await db.execute(
            select(MachineSnapshot).where(MachineSnapshot.category.in_(["vps", "ls"]))
        )).scalars().all()

    result = []
    for r in rows:
        match = None
        if r.category == "secondhand" and official:
            match = find_official_equivalent(r, official)
        score = compute_score(r, score_rules_by_cat.get(r.category))
        result.append(_machine_dict(r, match, score))
    return result


def _machine_dict(r: MachineSnapshot, official_match: dict | None = None,
                  score: float = 0.0) -> dict:
    return {
        "id": r.id,
        "category": r.category,
        "plan_id": r.plan_id,
        "area": r.area,
        "node": r.node,
        "name": r.name,
        "stock": r.stock,
        "price_monthly": r.price_monthly,
        "price_yearly": r.price_yearly,
        "cpu": r.cpu,
        "memory_mb": r.memory_mb,
        "disk_gb": r.disk_gb,
        "bandwidth_mbps": r.bandwidth_mbps,
        "traffic_gb": r.traffic_gb,
        "ipv4": r.ipv4,
        "available": r.available,
        "ip_status": getattr(r, "ip_status", "") or "",
        "score": score,
        "official_match": official_match,
        "updated_at": r.updated_at.isoformat() if r.updated_at else None,
    }


# ---------------------------------------------------------------------------
# Alert Events
# ---------------------------------------------------------------------------

@app.get("/api/events")
async def list_events(
    limit: int = 100,
    category: str | None = None,
    event_type: str | None = None,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
):
    q = select(AlertEvent).order_by(desc(AlertEvent.id))
    if category:
        q = q.where(AlertEvent.category == category)
    if event_type:
        q = q.where(AlertEvent.event_type == event_type)
    q = q.limit(limit)
    rows = (await db.execute(q)).scalars().all()
    return [
        {
            "id": r.id,
            "category": r.category,
            "plan_id": r.plan_id,
            "name": r.name,
            "area": r.area,
            "event_type": r.event_type,
            "old_value": r.old_value,
            "new_value": r.new_value,
            "notified": r.notified,
            "created_at": r.created_at.isoformat() if r.created_at else None,
        }
        for r in rows
    ]


# ---------------------------------------------------------------------------
# Price History
# ---------------------------------------------------------------------------

@app.get("/api/price-history")
async def get_price_history(plan_id: str | None = None, _: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    q = select(PriceHistory).order_by(PriceHistory.plan_id, PriceHistory.cycle)
    if plan_id:
        q = q.where(PriceHistory.plan_id == plan_id)
    rows = (await db.execute(q)).scalars().all()
    return [
        {
            "plan_id": r.plan_id,
            "price": r.price,
            "cycle": r.cycle,
            "fetched_at": r.fetched_at.isoformat() if r.fetched_at else None,
        }
        for r in rows
    ]


# ---------------------------------------------------------------------------
# Auto-buy (secondhand)
# ---------------------------------------------------------------------------

@app.post("/api/buy/{plan_id}")
async def buy_machine(plan_id: str, _: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Manually trigger a purchase for a secondhand listing."""
    settings = await get_all_settings(db)
    token = settings.get("akile_token", "")
    if not token:
        return {"error": "No token configured"}

    from . import akile_client as api
    cycle = (settings.get("secondhand_buy") or {}).get("cycle", 1)
    try:
        result = await api.buy_secondhand(token, {"id": plan_id, "cycle": cycle})
    except httpx.HTTPError as e:
        return {"error": str(e)}

    # Emit purchase event
    from .database import SessionLocal
    from .notify import send_telegram
    event = {
        "category": "secondhand",
        "plan_id": plan_id,
        "name": f"ID:{plan_id}",
        "area": "",
        "event_type": "purchased",
        "old_value": None,
        "new_value": str(result),
        "notified": False,
    }
    async with SessionLocal() as s:
        from .database import AlertEvent
        s.add(AlertEvent(**event))
        await s.commit()

    if settings.get("tg_notify_purchased") and settings.get("tg_bot_token"):
        event["new_value"] = "购买成功"
        await send_telegram(settings["tg_bot_token"], settings["tg_chat_id"], event)

    await broadcast({"type": "purchased", "plan_id": plan_id, "result": result})
    return result
