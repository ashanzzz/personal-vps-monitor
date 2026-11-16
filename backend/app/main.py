"""
FastAPI application entry point.
"""
from __future__ import annotations

import asyncio
import json
import logging
import os
from contextlib import asynccontextmanager
from datetime import datetime, timedelta

import httpx
from apscheduler.schedulers.asyncio import AsyncIOScheduler
from fastapi import Depends, FastAPI, HTTPException, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from sqlalchemy import desc, select
from sqlalchemy.ext.asyncio import AsyncSession

from . import auth
from .auth import require_auth
from .config import PAGE_KEYS, get_all_settings, set_setting, update_settings
from .database import AccountCache, AlertEvent, MachineSnapshot, PriceHistory, get_db, init_db
from .logging_setup import read_log_tail, set_level, setup_logging
from .match import find_official_equivalent
from .poller import poll_account, poll_price_history, run_page_poll, run_poll, run_store_refresh
from .scoring import compute_score, compute_star_rating, price_recommendation
from .token_manager import ensure_fresh, refresh_now, token_status
from .version import __version__, get_version_info
from .ws import broadcast, connect, disconnect
from . import akile_client
from .public_api import router as public_api_router
from .control_api import router as control_api_router
from .unified_api import router as unified_api_router

setup_logging()
logger = logging.getLogger(__name__)

scheduler = AsyncIOScheduler()
_poll_interval: int = 60  # seconds, updated from DB on startup

_JOB_DEFAULTS = {
    "coalesce": True,
    "max_instances": 1,
    "misfire_grace_time": 300,
}

_PAGE_JOB_START_OFFSETS = {
    "cloud": 0,
    "ls": 15,
    "secondhand": 30,
}

_SUPPORT_JOB_START_OFFSETS = {
    "token_refresh": 60,
    "prune_events": 120,
    "price_history_refresh": 180,
}


def _job_add_kwargs(first_run_delay_seconds: int) -> dict:
    """Shared APScheduler kwargs for long-running interval jobs."""
    return {
        **_JOB_DEFAULTS,
        "next_run_time": datetime.now() + timedelta(seconds=max(0, first_run_delay_seconds)),
        "replace_existing": True,
    }


def _schedule_interval_job(
    job_id: str,
    func,
    *,
    trigger_kwargs: dict,
    first_run_delay_seconds: int,
    args: list | tuple = (),
) -> None:
    """Add or reschedule a guarded interval job with a staggered first run."""
    if scheduler.get_job(job_id):
        scheduler.reschedule_job(job_id, trigger="interval", **trigger_kwargs)
        return

    scheduler.add_job(
        func,
        "interval",
        id=job_id,
        args=list(args),
        **trigger_kwargs,
        **_job_add_kwargs(first_run_delay_seconds),
    )


def _reschedule_page_jobs(settings: dict) -> None:
    """(Re)schedule one poll job per page using its independent interval.
    When random_jitter_enabled, the interval is multiplied by random(0.8, 1.2)."""
    import random
    jitter = settings.get("random_jitter_enabled", True)
    for index, page in enumerate(PAGE_KEYS):
        cfg = settings.get(f"{page}_buy") or {}
        base = max(5, int(cfg.get("refresh_interval", 60) or 60))
        interval = int(base * random.uniform(0.8, 1.2)) if jitter else base
        interval = max(5, interval)
        job_id = f"poll_{page}"
        start_offset = _PAGE_JOB_START_OFFSETS.get(page, index * 15)
        _schedule_interval_job(
            job_id,
            run_page_poll,
            trigger_kwargs={"seconds": interval},
            first_run_delay_seconds=start_offset,
            args=[page],
        )
        logger.info("page poller %s scheduled interval=%ds (base=%ds, jitter=%s)",
                    page, interval, base, jitter)


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
    """(Re)schedule the account-cache poll using account_refresh_interval.
    When random_jitter_enabled, the interval is multiplied by random(0.8, 1.2)."""
    import random
    base = max(30, int(settings.get("account_refresh_interval", 300) or 300))
    jitter = settings.get("random_jitter_enabled", True)
    interval = int(base * random.uniform(0.8, 1.2)) if jitter else base
    interval = max(30, interval)
    job_id = "poll_account"
    _schedule_interval_job(
        job_id,
        _run_account_poll,
        trigger_kwargs={"seconds": interval},
        first_run_delay_seconds=45,
    )
    logger.info("account poller scheduled interval=%ds (base=%ds, jitter=%s)",
                interval, base, jitter)


async def _run_price_history_refresh() -> None:
    """Refresh price history on its own low-frequency cadence."""
    from .database import SessionLocal

    try:
        await ensure_fresh()
    except Exception:
        logger.exception("ensure_fresh failed during price history refresh (continuing)")

    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    token = settings.get("akile_token") or ""
    if not token:
        logger.info("price_history_refresh: no token configured, skipping")
        return

    try:
        await poll_price_history(token)
    except Exception:
        logger.exception("price history refresh failed")


def _schedule_support_jobs() -> None:
    """Schedule low-frequency maintenance jobs with delayed first runs."""
    _schedule_interval_job(
        "token_refresh",
        ensure_fresh,
        trigger_kwargs={"hours": 1},
        first_run_delay_seconds=_SUPPORT_JOB_START_OFFSETS["token_refresh"],
    )
    _schedule_interval_job(
        "prune_events",
        _prune_events,
        trigger_kwargs={"hours": 24},
        first_run_delay_seconds=_SUPPORT_JOB_START_OFFSETS["prune_events"],
    )
    _schedule_interval_job(
        "price_history_refresh",
        _run_price_history_refresh,
        trigger_kwargs={"minutes": 30},
        first_run_delay_seconds=_SUPPORT_JOB_START_OFFSETS["price_history_refresh"],
    )
    logger.info("support jobs scheduled (token_refresh, prune_events, price_history_refresh)")


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

    # Low-frequency support jobs are staggered to avoid startup storms.
    _schedule_support_jobs()
    # Daily checkin for AK coins at a random time within the configured window.
    await _schedule_daily_checkin(settings)
    # Start targeted fast secondhand sniper (every 5 seconds)
    from .sniper import start_sniper_worker, stop_sniper_worker
    start_sniper_worker(interval_seconds=5)

    scheduler.start()
    logger.info("Scheduler started (per-page pollers and fast sniper active)")

    yield

    stop_sniper_worker()
    scheduler.shutdown()


def _pick_daily_checkin_minute(start: str, end: str) -> tuple[int, int]:
    """Pick a random HH:MM within an inclusive window.

    Supports cross-midnight windows such as 23:30–00:30 by sampling from the
    union of [start, 23:59] and [00:00, end]. Invalid inputs fall back to 08:01.
    """
    import random

    def _to_minutes(hhmm: str) -> int:
        try:
            h, m = hhmm.split(":")
            h_i = int(h)
            m_i = int(m)
            if 0 <= h_i <= 23 and 0 <= m_i <= 59:
                return h_i * 60 + m_i
        except Exception:
            pass
        return 8 * 60 + 1

    start_min = _to_minutes(start)
    end_min = _to_minutes(end)
    if start_min == end_min:
        pick_min = start_min
    elif end_min > start_min:
        pick_min = random.randint(start_min, end_min)
    else:
        day = 24 * 60
        tail_len = day - start_min
        head_len = end_min + 1
        pick = random.randrange(tail_len + head_len)
        pick_min = start_min + pick if pick < tail_len else pick - tail_len
    return pick_min // 60, pick_min % 60


async def _schedule_daily_checkin(settings: dict) -> None:
    """Schedule (or reschedule) the daily checkin job.

    Supports either a fixed daily time (e.g. 08:00) or a randomized time
    within a window [checkin_time_start, checkin_time_end].
    """
    tz = settings.get("timezone", "Asia/Shanghai")
    mode = settings.get("checkin_mode", "fixed")

    if mode == "fixed":
        fixed = settings.get("checkin_fixed_time", "08:00")
        try:
            pick_h, pick_m = [int(x) for x in fixed.split(":")]
        except Exception:
            pick_h, pick_m = 8, 0
        desc = f"fixed at {pick_h:02d}:{pick_m:02d}"
    else:
        start = settings.get("checkin_time_start", "08:00")
        end = settings.get("checkin_time_end", "09:00")
        pick_h, pick_m = _pick_daily_checkin_minute(start, end)
        desc = f"randomized at {pick_h:02d}:{pick_m:02d} (window {start}–{end})"

    job_id = "daily_checkin"
    if scheduler.get_job(job_id):
        scheduler.remove_job(job_id)

    from apscheduler.triggers.cron import CronTrigger
    scheduler.add_job(
        _daily_checkin,
        CronTrigger(hour=pick_h, minute=pick_m, timezone=tz),
        id=job_id,
    )
    logger.info("daily_checkin scheduled %s (tz=%s)", desc, tz)


async def _daily_checkin() -> None:
    """Automatic daily checkin for Akile & ACCK platforms with status recording."""
    from .database import SessionLocal, CheckinLog
    from datetime import date

    today_str = date.today().isoformat()

    try:
        await ensure_fresh()
    except Exception:
        logger.exception("ensure_fresh failed during daily checkin (continuing)")

    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    # 1. Akile 自动打卡
    if not settings.get("auto_checkin_enabled", True):
        logger.info("daily_checkin: Akile auto-checkin is disabled, skipping")
    else:
        token = settings.get("akile_token") or ""
        if token:
            try:
                result = await akile_client.checkin(token)
                status_code = result.get("status_code")
                status_msg = result.get("status_msg", "")
                if status_code == 0:
                    status = "success"
                    logger.info("Akile daily_checkin: success – %s", status_msg)
                elif status_code == 1:
                    status = "already_signed"
                    logger.info("Akile daily_checkin: already signed – %s", status_msg)
                else:
                    status = "failed"
                    logger.warning("Akile daily_checkin: unexpected response: %s", result)

                async with SessionLocal() as db:
                    db.add(CheckinLog(
                        platform="akile",
                        checkin_date=today_str,
                        status=status,
                        message=status_msg or str(result),
                        details=json.dumps(result, ensure_ascii=False),
                    ))
                    await db.commit()
            except Exception as e:
                logger.exception("Akile daily_checkin failed")
                async with SessionLocal() as db:
                    db.add(CheckinLog(
                        platform="akile",
                        checkin_date=today_str,
                        status="failed",
                        message=str(e),
                        details=None,
                    ))
                    await db.commit()

    # 2. ACCK 自动打卡
    if not settings.get("acck_auto_checkin_enabled", True):
        logger.info("daily_checkin: ACCK auto-checkin is disabled, skipping")
    else:
        acck_token = settings.get("acck_token") or ""
        if acck_token:
            from . import acck_client
            try:
                acck_res = await acck_client.checkin(acck_token)
                logger.info("ACCK daily_checkin result: %s", acck_res)
                # Parse ACCK result status
                text_repr = str(acck_res)
                if acck_res.get("code") == 0 or "成功" in text_repr or "已签到" in text_repr or acck_res.get("status") == 200:
                    if "已签到" in text_repr or "already" in text_repr.lower():
                        acck_status = "already_signed"
                    else:
                        acck_status = "success"
                elif "失败" in text_repr or "error" in text_repr.lower():
                    acck_status = "failed"
                else:
                    acck_status = "success"

                async with SessionLocal() as db:
                    db.add(CheckinLog(
                        platform="acck",
                        checkin_date=today_str,
                        status=acck_status,
                        message=text_repr[:500],
                        details=json.dumps(acck_res, ensure_ascii=False) if isinstance(acck_res, (dict, list)) else str(acck_res),
                    ))
                    await db.commit()
            except Exception as e:
                logger.exception("ACCK daily_checkin failed")
                async with SessionLocal() as db:
                    db.add(CheckinLog(
                        platform="acck",
                        checkin_date=today_str,
                        status="failed",
                        message=str(e),
                        details=None,
                    ))
                    await db.commit()

    # 3. LazyCat (懒猫云) 自动打卡
    if not settings.get("lazycat_auto_checkin_enabled", True):
        logger.info("daily_checkin: LazyCat auto-checkin is disabled, skipping")
    else:
        lazycat_token = settings.get("lazycat_token") or ""
        if lazycat_token:
            from . import lazycat_client
            try:
                lz_res = await lazycat_client.checkin(lazycat_token)
                logger.info("LazyCat daily_checkin result: %s", lz_res)
                text_repr = str(lz_res)
                if "已签到" in text_repr or "duplicate" in text_repr.lower():
                    lz_status = "already_signed"
                elif "成功" in text_repr or "rewarded" in text_repr.lower() or lz_res.get("points") is not None:
                    lz_status = "success"
                elif "失败" in text_repr or "error" in text_repr.lower():
                    lz_status = "failed"
                else:
                    lz_status = "success"

                async with SessionLocal() as db:
                    db.add(CheckinLog(
                        platform="lazycat",
                        checkin_date=today_str,
                        status=lz_status,
                        message=text_repr[:500],
                        details=json.dumps(lz_res, ensure_ascii=False) if isinstance(lz_res, (dict, list)) else str(lz_res),
                    ))
                    await db.commit()
            except Exception as e:
                logger.exception("LazyCat daily_checkin failed")
                async with SessionLocal() as db:
                    db.add(CheckinLog(
                        platform="lazycat",
                        checkin_date=today_str,
                        status="failed",
                        message=str(e),
                        details=None,
                    ))
                    await db.commit()


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


OPENAPI_TAGS = [
    {
        "name": "admin",
        "description": "管理员接口：登录、设置、日志、账户、机器、签到、手动刷新。",
    },
    {
        "name": "read-api",
        "description": "第三方只读 API：外部系统读取机器快照、评分和版本信息。",
    },
    {
        "name": "control-api",
        "description": "第三方控制 API：自动化写入设置并触发刷新/同步。",
    },
]

app = FastAPI(
    title="个人 vps 监控",
    description=(
        "个人 VPS / 云服务监控后端 API：统一管理多平台账户（懒猫云 / Akile / ACCK）"
        "、每日自动打卡、资产管理与选购工具。"
    ),
    version=__version__,
    openapi_tags=OPENAPI_TAGS,
    docs_url="/api/docs",
    redoc_url="/api/redoc",
    openapi_url="/api/openapi.json",
    swagger_ui_parameters={
        "persistAuthorization": True,
        "displayRequestDuration": True,
    },
    lifespan=lifespan,
)

# 补全根路径静默分流与前端通信兜底探针，杜绝 SPA 路由死锁
@app.get("/", include_in_schema=False)
async def root_fallback():
    from fastapi.responses import RedirectResponse
    return RedirectResponse(url="/api/docs")

@app.get("/api/health", tags=["admin"])
async def health_check():
    return {"status": "green", "timestamp": datetime.now().isoformat()}

@app.get("/api/sniper/status", tags=["admin"])
async def get_sniper_status():
    from . import sniper
    from .database import SessionLocal
    from .config import get_all_settings
    from .buy_engine import buy_state

    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    cfg = settings.get("secondhand_buy") or {}
    task = sniper._sniper_task
    exc_str = None
    if task and task.done():
        try:
            exc = task.exception()
            exc_str = str(exc) if exc else "None (exited normally)"
        except Exception as e:
            exc_str = f"failed to get exc: {e}"

    return {
        "status": "running" if (task and not task.done()) else "stopped",
        "task_exists": task is not None,
        "task_done": task.done() if task else None,
        "task_exception": exc_str,
        "interval_seconds": 5,
        "secondhand_buy_enabled": cfg.get("enabled", False),
        "real_purchase_enabled": settings.get("real_purchase_enabled", False),
        "rules_count": len(cfg.get("rules") or []),
        "rules": cfg.get("rules") or [],
        "daily_bought": buy_state.daily("secondhand"),
    }


app.include_router(public_api_router)
app.include_router(control_api_router)
app.include_router(unified_api_router)

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


class StoreRefreshBody(BaseModel):
    page: str | None = None
    limit: int | None = None


@app.get("/api/auth/status", tags=["admin"])
async def auth_status():
    """Whether an admin exists yet — drives login vs first-time setup screen."""
    return {"initialized": await auth.admin_exists()}


@app.post("/api/auth/setup", tags=["admin"])
async def auth_setup(body: SetupBody, db: AsyncSession = Depends(get_db)):
    """One-time admin creation, only allowed when no admin exists."""
    if await auth.get_admin(db) is not None:
        raise HTTPException(status_code=400, detail="管理员已存在")
    if not body.username or len(body.password) < 6:
        raise HTTPException(status_code=400, detail="用户名必填，密码至少 6 位")
    await auth.create_admin(db, body.username, body.password)
    token = await auth.issue_token(db, body.username)
    return {"token": token, "username": body.username}


@app.post("/api/auth/login", tags=["admin"])
async def auth_login(body: LoginBody, db: AsyncSession = Depends(get_db)):
    admin = await auth.get_admin(db)
    # Constant-ish behaviour: don't reveal whether the username exists.
    if admin is None or admin.username != body.username or not auth.verify_password(
        body.password, admin.password_hash
    ):
        raise HTTPException(status_code=401, detail="用户名或密码错误")
    token = await auth.issue_token(db, admin.username)
    return {"token": token, "username": admin.username}


@app.post("/api/auth/change-password", tags=["admin"])
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

@app.get("/api/settings", tags=["admin"])
async def get_settings(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    return await get_all_settings(db)


@app.post("/api/settings", tags=["admin"])
async def save_settings(data: dict, _: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    result = await update_settings(db, data)

    # Reschedule per-page pollers (their intervals may have changed).
    _reschedule_page_jobs(result)

    # Reschedule the account-cache poller (account_refresh_interval may have changed).
    _reschedule_account_job(result)

    # Apply log level live.
    set_level(result.get("log_level", "INFO"))

    # Reschedule daily checkin (time range may have changed).
    await _schedule_daily_checkin(result)

    return result


@app.get("/api/version", tags=["admin"])
async def get_version():
    return get_version_info()


@app.get("/api/logs/app", tags=["admin"])
async def get_app_log(lines: int = 500, _: str = Depends(require_auth)):
    """Return the tail of the persistent app log for in-UI debugging."""
    lines = max(1, min(lines, 5000))
    return {"lines": lines, "content": read_log_tail(lines)}


@app.post("/api/poll/trigger", tags=["admin"])
async def trigger_poll(_: str = Depends(require_auth)):
    """Manually trigger a full poll cycle (all pages)."""
    asyncio.create_task(run_poll())
    return {"status": "triggered"}


@app.post("/api/store/refresh", tags=["admin"])
async def refresh_store(body: StoreRefreshBody, _: str = Depends(require_auth)):
    """Manually refresh store listings (batch-limited by default)."""
    page = body.page.strip() if body.page else None
    if page and page not in PAGE_KEYS:
        raise HTTPException(400, f"Unknown page: {page}")
    limit = body.limit if body.limit and body.limit > 0 else 100
    asyncio.create_task(run_store_refresh(page, limit=limit))
    return {"status": "triggered", "page": page or "all", "limit": limit}


@app.post("/api/token/refresh", tags=["admin"])
async def manual_refresh_token(_: str = Depends(require_auth)):
    """Manually force an Akile token refresh (writes new token to DB)."""
    ok = await refresh_now(reason="manual")
    return {"status": "ok" if ok else "failed"}


@app.get("/api/token/status", tags=["admin"])
async def get_token_status(_: str = Depends(require_auth)):
    """Report the current token's expiry/health for the UI."""
    return await token_status()


class AkileLoginBody(BaseModel):
    username: str
    password: str
    ga_code: str = ""  # optional 2FA code (step 2 only)


@app.post("/api/akile-login", tags=["admin"])
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

    login_payload: dict = {"email": body.username, "password": body.password, "token": "", "verifyCode": ""}
    if body.ga_code.strip():
        login_payload["token"] = body.ga_code.strip()

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


@app.get("/api/account", tags=["admin"])
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


@app.post("/api/account/refresh", tags=["admin"])
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


@app.post("/api/checkin", tags=["admin"])
async def manual_checkin(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Manually trigger the daily AK-coin checkin, returning Akile's raw response
    plus parsed ak_coin_earned from the status_msg."""
    settings = await get_all_settings(db)
    token = settings.get("akile_token", "")
    if not token:
        return {"error": "No token configured"}
    try:
        result = await akile_client.checkin(token)
        status_msg = result.get("status_msg", "")
        # Parse coin amount from messages like "签到成功，获得 5 AK 币" or "签到成功，获得5AK"
        import re
        m = re.search(r'(\d+\.?\d*)', status_msg)
        result["ak_coin_earned"] = float(m.group(1)) if m else None

        from .database import CheckinLog
        from datetime import date
        status_code = result.get("status_code")
        status = "success" if status_code == 0 else ("already_signed" if status_code == 1 else "failed")
        db.add(CheckinLog(
            platform="akile",
            checkin_date=date.today().isoformat(),
            status=status,
            message=status_msg or str(result),
            details=json.dumps(result, ensure_ascii=False),
        ))
        await db.commit()

        return result
    except httpx.HTTPError as e:
        return {"error": str(e)}


# ---------------------------------------------------------------------------
# ACCK Platform Endpoints
# ---------------------------------------------------------------------------

@app.get("/api/acck/account", tags=["admin"])
async def get_acck_account(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Return the cached ACCK account snapshot (profile + points + servers)."""
    settings = await get_all_settings(db)
    token = settings.get("acck_token", "")
    if not token:
        return {"configured": False, "profile": None, "servers": [],
                "errors": ["no_token"], "updated_at": None}

    from .poller import poll_acck_account
    row = await db.scalar(select(AccountCache).where(AccountCache.id == 2))
    if row is None or not row.data:
        await poll_acck_account(token, settings)
        row = await db.scalar(select(AccountCache).where(AccountCache.id == 2))
        if row is None or not row.data:
            return {"configured": True, "profile": None, "servers": [],
                    "errors": ["cache_empty"], "updated_at": None}

    payload = json.loads(row.data)
    payload["updated_at"] = row.updated_at.isoformat() if row.updated_at else None
    return payload


@app.post("/api/acck/account/refresh", tags=["admin"])
async def refresh_acck_account(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Force an immediate live fetch for ACCK account."""
    settings = await get_all_settings(db)
    token = settings.get("acck_token", "")
    if not token:
        return {"configured": False, "profile": None, "servers": [],
                "errors": ["no_token"], "updated_at": None}

    from .poller import poll_acck_account
    await poll_acck_account(token, settings)
    row = await db.scalar(select(AccountCache).where(AccountCache.id == 2))
    if row is None or not row.data:
        return {"configured": True, "profile": None, "servers": [],
                "errors": ["cache_empty"], "updated_at": None}
    payload = json.loads(row.data)
    payload["updated_at"] = row.updated_at.isoformat() if row.updated_at else None
    return payload


@app.post("/api/acck/checkin", tags=["admin"])
async def manual_acck_checkin(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Manually trigger daily checkin for ACCK."""
    settings = await get_all_settings(db)
    token = settings.get("acck_token", "")
    if not token:
        return {"error": "未配置 ACCK Token，请先在系统设置中填入"}
    from . import acck_client
    from .database import CheckinLog
    from datetime import date
    try:
        res = await acck_client.checkin(token)
        text_repr = str(res)
        if res.get("code") == 0 or "成功" in text_repr or "已签到" in text_repr or res.get("status") == 200:
            acck_status = "already_signed" if ("已签到" in text_repr or "already" in text_repr.lower()) else "success"
        else:
            acck_status = "failed"
        db.add(CheckinLog(
            platform="acck",
            checkin_date=date.today().isoformat(),
            status=acck_status,
            message=text_repr[:500],
            details=json.dumps(res, ensure_ascii=False) if isinstance(res, (dict, list)) else str(res),
        ))
        await db.commit()
        return res
    except httpx.HTTPError as e:
        return {"error": str(e)}


@app.get("/api/checkin/status")
async def get_checkin_status(db: AsyncSession = Depends(get_db)):
    """Return the checkin status for today (Akile & ACCK) and recent history (public/admin readable)."""
    from datetime import date
    from .database import CheckinLog
    today_str = date.today().isoformat()

    settings = await get_all_settings(db)
    q_today = select(CheckinLog).where(CheckinLog.checkin_date == today_str).order_by(CheckinLog.id.desc())
    today_rows = (await db.execute(q_today)).scalars().all()

    akile_today = next((r for r in today_rows if r.platform == "akile"), None)
    acck_today = next((r for r in today_rows if r.platform == "acck"), None)
    lazycat_today = next((r for r in today_rows if r.platform == "lazycat"), None)

    q_hist = select(CheckinLog).order_by(CheckinLog.id.desc()).limit(30)
    hist_rows = (await db.execute(q_hist)).scalars().all()

    def serialize_log(log_row):
        if not log_row:
            return None
        return {
            "id": log_row.id,
            "platform": log_row.platform,
            "checkin_date": log_row.checkin_date,
            "status": log_row.status,
            "message": log_row.message,
            "created_at": log_row.created_at.isoformat() if log_row.created_at else None,
        }

    return {
        "today": today_str,
        "config": {
            "mode": settings.get("checkin_mode", "fixed"),
            "fixed_time": settings.get("checkin_fixed_time", "08:00"),
            "time_start": settings.get("checkin_time_start", "08:00"),
            "time_end": settings.get("checkin_time_end", "09:00"),
            "akile_enabled": settings.get("auto_checkin_enabled", True),
            "acck_enabled": settings.get("acck_auto_checkin_enabled", True),
            "lazycat_enabled": settings.get("lazycat_auto_checkin_enabled", True),
        },
        "status": {
            "akile": {
                "checked_in": akile_today.status in ("success", "already_signed") if akile_today else False,
                "status": akile_today.status if akile_today else "pending",
                "last_log": serialize_log(akile_today),
            },
            "acck": {
                "checked_in": acck_today.status in ("success", "already_signed") if acck_today else False,
                "status": acck_today.status if acck_today else "pending",
                "last_log": serialize_log(acck_today),
            },
            "lazycat": {
                "checked_in": lazycat_today.status in ("success", "already_signed") if lazycat_today else False,
                "status": lazycat_today.status if lazycat_today else "pending",
                "last_log": serialize_log(lazycat_today),
            }
        },
        "history": [serialize_log(r) for r in hist_rows]
    }


@app.get("/api/acck/servers", tags=["admin"])
async def get_acck_servers(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Get list of user's own servers from ACCK."""
    settings = await get_all_settings(db)
    token = settings.get("acck_token", "")
    if not token:
        return []
    from . import acck_client
    try:
        return await acck_client.get_my_servers(token)
    except Exception as e:
        return {"error": str(e)}


@app.get("/api/acck/shop", tags=["admin"])
async def get_acck_shop_data(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Get ACCK shop balance, items, and checkin logs."""
    settings = await get_all_settings(db)
    token = settings.get("acck_token", "")
    if not token:
        return {"error": "No token configured"}
    from . import acck_client
    try:
        shop = await acck_client.get_ac_shop(token)
        logs = await acck_client.get_shop_logs(token)
        return {"shop": shop, "logs": logs}
    except Exception as e:
        return {"error": str(e)}


@app.post("/api/acck/buy-preview", tags=["admin"])
async def acck_buy_preview_endpoint(payload: dict, _: str = Depends(require_auth)):
    """【防误购安全模式】安全试算 ACCK 订单信息，严禁发起扣费支付。"""
    from . import acck_client
    return await acck_client.buy_preview("", payload)


# ---------------------------------------------------------------------------
# LazyCat (dash.lazycat.wiki) endpoints
# ---------------------------------------------------------------------------

@app.post("/api/lazycat/checkin", tags=["admin"])
async def manual_lazycat_checkin(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """手动触发懒猫云每日签到。"""
    settings = await get_all_settings(db)
    token = settings.get("lazycat_token", "")
    if not token:
        return {"error": "未配置 LazyCat Token，请先在系统设置中填入"}
    from . import lazycat_client
    from .database import CheckinLog
    from datetime import date
    try:
        res = await lazycat_client.checkin(token)
        text_repr = str(res)
        if "已签到" in text_repr or "duplicate" in text_repr.lower():
            lz_status = "already_signed"
        elif "成功" in text_repr or "rewarded" in text_repr.lower() or res.get("points") is not None:
            lz_status = "success"
        elif "失败" in text_repr or "error" in text_repr.lower():
            lz_status = "failed"
        else:
            lz_status = "success"

        db.add(CheckinLog(
            platform="lazycat",
            checkin_date=date.today().isoformat(),
            status=lz_status,
            message=text_repr[:500],
            details=json.dumps(res, ensure_ascii=False) if isinstance(res, (dict, list)) else str(res),
        ))
        await db.commit()
        return res
    except Exception as e:
        return {"error": str(e)}


@app.get("/api/lazycat/account", tags=["admin"])
async def get_lazycat_account_data(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """获取懒猫云账户资料、余额、VIP 等级与特权卡状态。"""
    settings = await get_all_settings(db)
    token = settings.get("lazycat_token", "")
    if not token:
        return {"error": "未配置 LazyCat Token"}
    from . import lazycat_client
    try:
        user_info = await lazycat_client.get_user_info(token)
        balance = await lazycat_client.get_user_balance(token)
        vip_card = await lazycat_client.get_vip_card(token)
        return {
            "user": user_info,
            "balance": balance,
            "vip_card": vip_card,
        }
    except Exception as e:
        return {"error": str(e)}


@app.get("/api/lazycat/instances", tags=["admin"])
async def get_lazycat_user_instances(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """获取懒猫云名下所有实例。"""
    settings = await get_all_settings(db)
    token = settings.get("lazycat_token", "")
    if not token:
        return []
    from . import lazycat_client
    try:
        return await lazycat_client.get_my_instances(token)
    except Exception as e:
        return {"error": str(e)}


@app.get("/api/lazycat/market", tags=["admin"])
async def get_lazycat_market_listings(
    region: str | None = None,
    page: int = 1,
    pageSize: int = 20,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
):
    """获取懒猫云交易市场二手在售实例。"""
    settings = await get_all_settings(db)
    token = settings.get("lazycat_token", "")
    if not token:
        return {"error": "未配置 LazyCat Token"}
    from . import lazycat_client
    try:
        return await lazycat_client.get_market_listings(token, region=region, page=page, page_size=pageSize)
    except Exception as e:
        return {"error": str(e)}


@app.post("/api/lazycat/login", tags=["admin"])
async def lazycat_login(
    body: dict,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
):
    """用账号密码（可选 2FA 动态码）登录懒猫云，并把 Token 保存到系统设置。

    请求体：{ username, password, ga_code? }
    """
    from . import lazycat_client

    username = body.get("username", "")
    password = body.get("password", "")
    ga_code = body.get("ga_code", "") or None

    if not username or not password:
        return {"error": "请填写懒猫云账号和密码"}

    def _looks_like_2fa(text: str) -> bool:
        low = text.lower()
        return any(k in low for k in (
            "totp", "2fa", "two-factor", "two_factor", "verification code",
            "verify code", "otp", "二次验证", "两步验证", "动态码", "验证码",
        ))

    try:
        res = await lazycat_client.login(
            username=username, password=password, totp_code=ga_code
        )
    except httpx.HTTPStatusError as e:
        detail = e.response.text[:500]
        if _looks_like_2fa(detail):
            return {
                "need_2fa": True,
                "message": "该账号开启了二次验证，请输入验证器 App 中的 6 位动态码后重试",
                "detail": detail,
            }
        return {"error": f"登录失败（HTTP {e.response.status_code}）: {detail}"}
    except Exception as e:
        return {"error": f"登录请求失败: {e}"}

    # 提取 Token：兼容多种响应字段名
    token = (
        res.get("token")
        or res.get("access_token")
        or res.get("jwt")
        or (isinstance(res.get("data"), dict) and res["data"].get("token"))
        or (isinstance(res.get("data"), dict) and res["data"].get("accessToken"))
    )

    if not token:
        raw = str(res)[:500]
        if _looks_like_2fa(raw):
            return {
                "need_2fa": True,
                "message": "该账号开启了二次验证，请输入验证器 App 中的 6 位动态码后重试",
                "detail": raw,
            }
        return {
            "error": f"登录成功但未在响应中找到 Token 字段，完整响应: {raw}"
        }

    # 把 Token 写入系统设置
    await set_setting(db, "lazycat_token", token)
    await db.commit()

    return {
        "status": "ok",
        "message": "懒猫云登录成功，Token 已自动保存",
        "token_length": len(token),
    }


@app.get("/api/lazycat/store", tags=["admin"])
async def get_lazycat_store_catalog():
    """获取懒猫云公共套餐商店（无须 Token）。"""
    from . import lazycat_client
    try:
        packages = await lazycat_client.get_public_packages()
        regions = await lazycat_client.get_public_regions()
        return {"packages": packages, "regions": regions}
    except Exception as e:
        return {"error": str(e)}


@app.post("/api/lazycat/buy/{listing_id}", tags=["admin"])
async def buy_lazycat_instance(
    listing_id: str,
    _: str = Depends(require_auth),
    db: AsyncSession = Depends(get_db),
):
    """【带安全锁】购买懒猫云二手在售实例。"""
    settings = await get_all_settings(db)
    if not settings.get("real_purchase_enabled", False):
        return {
            "status_code": 1,
            "status_msg": "【防误购安全保护锁已开启】未发起真实支付扣费。已拦截懒猫云下单动作！",
            "dry_run": True,
            "listing_id": listing_id,
        }
    token = settings.get("lazycat_token", "")
    if not token:
        return {"error": "未配置 LazyCat Token"}
    from . import lazycat_client
    try:
        return await lazycat_client.purchase_instance(token, listing_id)
    except Exception as e:
        return {"error": str(e)}


@app.get("/api/lazycat/lotteries", tags=["admin"])
async def get_lazycat_lotteries_info(_: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """获取懒猫云活动中心大转盘抽奖活动。"""
    settings = await get_all_settings(db)
    token = settings.get("lazycat_token", "")
    if not token:
        return {"error": "未配置 LazyCat Token"}
    from . import lazycat_client
    try:
        return await lazycat_client.get_lotteries(token)
    except Exception as e:
        return {"error": str(e)}


# ---------------------------------------------------------------------------
# Machines
# ---------------------------------------------------------------------------

@app.get("/api/machines", tags=["admin"])
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


def _coerce_fen_int(raw: object) -> int:
    if raw in (None, "") or isinstance(raw, bool):
        return 0
    try:
        return int(raw)
    except (TypeError, ValueError):
        return 0


def _coerce_optional_fen(raw: object) -> int | None:
    value = _coerce_fen_int(raw)
    if value == 0 and raw not in (0, "0", 0.0):
        return None
    return value


def _machine_dict(r: MachineSnapshot, official_match: dict | None = None,
                  score: float = 0.0) -> dict:
    star = compute_star_rating(score)
    price_monthly = _coerce_fen_int(getattr(r, "price_monthly", 0))
    price_yearly = _coerce_optional_fen(getattr(r, "price_yearly", None))
    sale_price = _coerce_optional_fen(getattr(r, "sale_price", None))
    recommendation = price_recommendation(score, price_monthly, price_yearly)
    return {
        "id": r.id,
        "category": r.category,
        "plan_id": r.plan_id,
        "area": r.area,
        "node": r.node,
        "name": r.name,
        "stock": r.stock,
        "price_monthly": price_monthly,
        "price_yearly": price_yearly,
        "sale_price": sale_price,
        "cpu": r.cpu,
        "memory_mb": r.memory_mb,
        "disk_gb": r.disk_gb,
        "bandwidth_mbps": r.bandwidth_mbps,
        "traffic_gb": r.traffic_gb,
        "ipv4": r.ipv4,
        "available": r.available,
        "ip_status": getattr(r, "ip_status", "") or "",
        "score": score,
        "star_rating": star,
        "price_recommendation": recommendation,
        "official_match": official_match,
        "updated_at": r.updated_at.isoformat() if r.updated_at else None,
    }


# ---------------------------------------------------------------------------
# Alert Events
# ---------------------------------------------------------------------------

@app.get("/api/events", tags=["admin"])
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

@app.get("/api/price-history", tags=["admin"])
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

@app.post("/api/buy/{plan_id}", tags=["admin"])
async def buy_machine(plan_id: str, _: str = Depends(require_auth), db: AsyncSession = Depends(get_db)):
    """Manually trigger a purchase for a secondhand listing."""
    settings = await get_all_settings(db)

    # 严格防误购保护锁：除非显式开启 real_purchase_enabled，否则一律仅作为安全模拟，严禁发起扣费下单
    if not settings.get("real_purchase_enabled", False):
        return {
            "status_code": 1,
            "status_msg": "【防误购安全保护锁已开启】未发起真实支付扣费。已拦截下单动作！",
            "dry_run": True,
            "plan_id": plan_id,
        }

    token = settings.get("akile_token", "")
    if not token:
        return {"error": "No token configured"}

    from . import akile_client as api
    try:
        result = await api.buy_secondhand(token, plan_id, auto_pay=True)
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
