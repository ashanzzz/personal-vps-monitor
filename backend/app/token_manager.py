"""
Akile token lifecycle management.

The token is a 24h JWT. Akile exposes a refresh endpoint that swaps a still-valid
token for a fresh 24h one (no login / 2FA). This module keeps the token alive
entirely inside the backend so the app runs standalone — no external cron needed.

Strategy:
- A scheduled job runs periodically and refreshes when the token is within
  REFRESH_MARGIN_SECONDS of expiry.
- On a 401 during polling, callers can trigger an immediate refresh + retry.
- The new token is written back to the DB settings, so the running poller picks
  it up on its next cycle without a restart.
- Refresh failures emit a Telegram alert (if configured) so the user can re-login.
"""
from __future__ import annotations

import base64
import json
import logging
import time

from . import akile_client as api
from .config import get_all_settings, set_setting
from .database import SessionLocal
from .notify import send_custom

logger = logging.getLogger(__name__)

# Refresh when the token has this much (or less) validity remaining.
REFRESH_MARGIN_SECONDS = 6 * 3600  # 6 hours

RELOGIN_HINT = (
    "请打开 https://akile.ai/console/pushshop 登录后，F12 → Console 执行 "
    "console.log(localStorage.getItem('akile-token'))，把新 token 发我更新。"
)


def jwt_exp(tok: str) -> int:
    """Return the JWT 'exp' (unix seconds), or 0 if it can't be parsed."""
    try:
        payload = tok.split(".")[1]
        payload += "=" * (-len(payload) % 4)
        decoded = json.loads(base64.urlsafe_b64decode(payload))
        return int(decoded.get("exp", 0))
    except Exception:
        return 0


def _seconds_left(tok: str) -> int:
    exp = jwt_exp(tok)
    if not exp:
        return -1  # unknown — treat as "needs attention"
    return exp - int(time.time())


async def _alert(settings: dict, text: str) -> None:
    """Send a Telegram alert if a bot is configured. Never raises."""
    bot = settings.get("tg_bot_token")
    chat = settings.get("tg_chat_id")
    if bot and chat:
        try:
            await send_custom(bot, chat, text)
        except Exception:
            logger.exception("Failed to send token alert")


async def refresh_now(reason: str = "scheduled") -> bool:
    """Force a refresh of the current token and persist the result.

    Returns True on success, False otherwise. Safe to call concurrently-ish;
    last write wins, which is fine for an idempotent token swap.
    """
    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    token = settings.get("akile_token") or ""
    if not token:
        logger.warning("Token refresh (%s) skipped: no token configured", reason)
        await _alert(settings, f"⚠️ Akile token 未配置，无法续期。{RELOGIN_HINT}")
        return False

    try:
        new_token = await api.refresh_token(token)
    except Exception as e:  # httpx errors, RuntimeError, etc.
        left = _seconds_left(token)
        hours = left / 3600 if left > 0 else 0
        logger.error("Token refresh (%s) failed: %s", reason, e)
        await _alert(
            settings,
            f"⚠️ Akile token 续期失败（{reason}）：{e}\n"
            f"当前 token 剩余约 {hours:.1f} 小时。{RELOGIN_HINT}",
        )
        return False

    async with SessionLocal() as db:
        await set_setting(db, "akile_token", new_token)

    logger.info("Token refreshed (%s); new token valid ~24h", reason)
    return True


async def ensure_fresh() -> None:
    """Scheduled entry point: refresh only if the token is near expiry.

    Called periodically by APScheduler. Cheap no-op when the token is healthy.
    """
    async with SessionLocal() as db:
        settings = await get_all_settings(db)

    token = settings.get("akile_token") or ""
    if not token:
        logger.info("ensure_fresh: no token configured, skipping")
        return

    left = _seconds_left(token)
    if left == -1:
        logger.warning("ensure_fresh: token exp unparseable, attempting refresh")
        await refresh_now(reason="unparseable-exp")
        return

    if left <= REFRESH_MARGIN_SECONDS:
        logger.info("ensure_fresh: token has %ds left, refreshing", left)
        await refresh_now(reason="near-expiry")
    else:
        logger.debug("ensure_fresh: token healthy (%ds left), no action", left)
