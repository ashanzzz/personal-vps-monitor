"""
Admin authentication: bcrypt password hashing + JWT session tokens.

Provisioning order on startup:
1. If ADMIN_USER + ADMIN_PASSWORD are set in the env and no admin exists yet,
   create the admin from env (idempotent — won't overwrite an existing one).
2. Otherwise the dashboard shows a one-time setup screen so the user can pick a
   username/password, stored as a bcrypt hash.

The JWT signing secret comes from AUTH_SECRET (env) when provided; otherwise a
random secret is generated once and persisted in the settings table so tokens
survive restarts without a hard-coded default.
"""
from __future__ import annotations

import os
import secrets
import time

import bcrypt
import jwt
from fastapi import Depends, Header, HTTPException, status
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from .config import get_setting, set_setting
from .database import AdminUser, SessionLocal, get_db

# Session token lifetime (seconds). Default 7 days.
TOKEN_TTL_SECONDS = int(os.getenv("AUTH_TOKEN_TTL", str(7 * 24 * 3600)))
_ALGO = "HS256"


# ---------------------------------------------------------------------------
# Password hashing
# ---------------------------------------------------------------------------

def hash_password(plain: str) -> str:
    return bcrypt.hashpw(plain.encode("utf-8"), bcrypt.gensalt()).decode("utf-8")


def verify_password(plain: str, hashed: str) -> bool:
    try:
        return bcrypt.checkpw(plain.encode("utf-8"), hashed.encode("utf-8"))
    except (ValueError, TypeError):
        return False


# ---------------------------------------------------------------------------
# JWT signing secret (env > persisted random)
# ---------------------------------------------------------------------------

async def _get_secret(db: AsyncSession) -> str:
    env_secret = os.getenv("AUTH_SECRET", "")
    if env_secret:
        return env_secret
    stored = await get_setting(db, "_auth_secret")
    if stored:
        return stored
    generated = secrets.token_urlsafe(48)
    await set_setting(db, "_auth_secret", generated)
    return generated


# ---------------------------------------------------------------------------
# Token issue / verify
# ---------------------------------------------------------------------------

async def issue_token(db: AsyncSession, username: str) -> str:
    secret = await _get_secret(db)
    now = int(time.time())
    payload = {"sub": username, "iat": now, "exp": now + TOKEN_TTL_SECONDS}
    return jwt.encode(payload, secret, algorithm=_ALGO)


async def _decode_token(db: AsyncSession, token: str) -> dict:
    secret = await _get_secret(db)
    return jwt.decode(token, secret, algorithms=[_ALGO])


# ---------------------------------------------------------------------------
# Admin provisioning
# ---------------------------------------------------------------------------

async def get_admin(db: AsyncSession) -> AdminUser | None:
    return await db.scalar(select(AdminUser).limit(1))


async def admin_exists() -> bool:
    async with SessionLocal() as db:
        return (await get_admin(db)) is not None


async def provision_from_env() -> None:
    """Create the admin from ADMIN_USER/ADMIN_PASSWORD if none exists yet."""
    user = os.getenv("ADMIN_USER", "").strip()
    pw = os.getenv("ADMIN_PASSWORD", "")
    if not user or not pw:
        return
    async with SessionLocal() as db:
        if await get_admin(db) is not None:
            return
        db.add(AdminUser(username=user, password_hash=hash_password(pw)))
        await db.commit()


async def create_admin(db: AsyncSession, username: str, password: str) -> None:
    db.add(AdminUser(username=username, password_hash=hash_password(password)))
    await db.commit()


async def change_password(db: AsyncSession, username: str, new_password: str) -> bool:
    admin = await get_admin(db)
    if admin is None or admin.username != username:
        return False
    admin.password_hash = hash_password(new_password)
    await db.commit()
    return True


# ---------------------------------------------------------------------------
# FastAPI dependency: require a valid bearer token
# ---------------------------------------------------------------------------

async def require_auth(
    authorization: str = Header(default=""),
    db: AsyncSession = Depends(get_db),
) -> str:
    """Validate the Authorization: Bearer *** header. Returns the username."""
    if not authorization.lower().startswith("bearer "):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="未登录")
    token = authorization[7:].strip()
    try:
        payload = await _decode_token(db, token)
    except jwt.ExpiredSignatureError:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="登录已过期")
    except jwt.PyJWTError:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="无效的登录凭证")
    return payload.get("sub", "")


async def require_read_api_key(
    x_api_key: str = Header(default="", alias="X-Api-Key"),
) -> str:
    """Validate the public read-only API key from PUBLIC_READ_TOKEN.

    The dependency is intentionally separate from admin JWT auth so third-party
    consumers can read machine snapshots without receiving an admin session.
    """
    read_token = os.getenv("PUBLIC_READ_TOKEN", "").strip()
    if not read_token:
        raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="只读 API 未启用")
    if not x_api_key or not secrets.compare_digest(x_api_key, read_token):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="无效的只读访问密钥")
    return "ok"


async def verify_ws_token(token: str) -> bool:
    """Validate a token passed as a WebSocket query param. Never raises."""
    if not token:
        return False
    try:
        async with SessionLocal() as db:
            await _decode_token(db, token)
        return True
    except Exception:
        return False
