"""
FastAPI application entry point.
"""
from __future__ import annotations

import asyncio
import logging
import os
from contextlib import asynccontextmanager
from datetime import datetime

import httpx
from apscheduler.schedulers.asyncio import AsyncIOScheduler
from fastapi import Depends, FastAPI, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
from sqlalchemy import desc, select
from sqlalchemy.ext.asyncio import AsyncSession

from .config import get_all_settings, update_settings
from .database import AlertEvent, MachineSnapshot, PriceHistory, get_db, init_db
from .poller import run_poll
from .token_manager import ensure_fresh, refresh_now, token_status
from .ws import broadcast, connect, disconnect

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

scheduler = AsyncIOScheduler()
_poll_interval: int = 60  # seconds, updated from DB on startup


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Init DB
    await init_db()

    # Load interval from settings
    from .database import SessionLocal
    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    interval = settings.get("refresh_interval", 60)

    # If token is available in env (first run), save it
    env_token = os.getenv("AKILE_TOKEN", "")
    if env_token and not settings.get("akile_token"):
        async with SessionLocal() as db:
            from .config import set_setting
            await set_setting(db, "akile_token", env_token)

    scheduler.add_job(run_poll, "interval", seconds=interval, id="poll",
                      next_run_time=datetime.now())
    # Keep the Akile token alive entirely inside the backend: check hourly,
    # refresh when within the expiry margin. Runs an immediate check on startup.
    scheduler.add_job(ensure_fresh, "interval", hours=1, id="token_refresh",
                      next_run_time=datetime.now())
    # Prune old events daily to keep the DB lean.
    scheduler.add_job(_prune_events, "interval", hours=24, id="prune_events",
                      next_run_time=datetime.now())
    scheduler.start()
    logger.info("Scheduler started with interval=%ds", interval)

    yield

    scheduler.shutdown()


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

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# ---------------------------------------------------------------------------
# WebSocket
# ---------------------------------------------------------------------------

@app.websocket("/ws")
async def websocket_endpoint(ws: WebSocket):
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
async def get_settings(db: AsyncSession = Depends(get_db)):
    return await get_all_settings(db)


@app.post("/api/settings")
async def save_settings(data: dict, db: AsyncSession = Depends(get_db)):
    result = await update_settings(db, data)

    # Reschedule poller if interval changed
    new_interval = result.get("refresh_interval", 60)
    if scheduler.get_job("poll"):
        scheduler.reschedule_job("poll", trigger="interval", seconds=new_interval)
        logger.info("Rescheduled poller to %ds", new_interval)

    return result


@app.post("/api/poll/trigger")
async def trigger_poll():
    """Manually trigger a poll cycle."""
    asyncio.create_task(run_poll())
    return {"status": "triggered"}


@app.post("/api/token/refresh")
async def manual_refresh_token():
    """Manually force an Akile token refresh (writes new token to DB)."""
    ok = await refresh_now(reason="manual")
    return {"status": "ok" if ok else "failed"}


@app.get("/api/token/status")
async def get_token_status():
    """Report the current token's expiry/health for the UI."""
    return await token_status()


# ---------------------------------------------------------------------------
# Machines
# ---------------------------------------------------------------------------

@app.get("/api/machines")
async def list_machines(
    category: str | None = None,
    available_only: bool = False,
    db: AsyncSession = Depends(get_db),
):
    q = select(MachineSnapshot)
    if category:
        q = q.where(MachineSnapshot.category == category)
    if available_only:
        q = q.where(MachineSnapshot.available == True)  # noqa: E712
    q = q.order_by(MachineSnapshot.category, MachineSnapshot.area, MachineSnapshot.name)
    rows = (await db.execute(q)).scalars().all()
    return [_machine_dict(r) for r in rows]


def _machine_dict(r: MachineSnapshot) -> dict:
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
async def get_price_history(plan_id: str | None = None, db: AsyncSession = Depends(get_db)):
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
async def buy_machine(plan_id: str, db: AsyncSession = Depends(get_db)):
    """Manually trigger a purchase for a secondhand listing."""
    settings = await get_all_settings(db)
    token = settings.get("akile_token", "")
    if not token:
        return {"error": "No token configured"}

    from . import akile_client as api
    try:
        result = await api.buy_secondhand(token, {"id": plan_id, "cycle": settings.get("auto_buy_cycle", 1)})
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
