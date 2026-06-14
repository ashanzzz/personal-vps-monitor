"""
Database models and async engine setup.
"""
import os
from datetime import datetime
from typing import Optional

from sqlalchemy import JSON, Boolean, DateTime, Float, Integer, String, Text
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

DATABASE_URL = os.getenv("DATABASE_URL", "sqlite+aiosqlite:////app/data/monitor.db")

# busy_timeout=30s: when SQLite hits a write lock, wait up to 30s for it to
# clear instead of failing immediately with "database is locked". Set per
# connection via connect_args; PRAGMA busy_timeout in init_db is a backstop.
engine = create_async_engine(DATABASE_URL, echo=False, connect_args={"timeout": 30})
SessionLocal = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)


class Base(DeclarativeBase):
    pass


class Settings(Base):
    """Single-row app settings, keyed by name."""
    __tablename__ = "settings"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    key: Mapped[str] = mapped_column(String(64), unique=True, nullable=False)
    value: Mapped[str] = mapped_column(Text, nullable=False, default="")


class MachineSnapshot(Base):
    """Latest known state of each plan/product."""
    __tablename__ = "machine_snapshot"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    # "vps" | "ls" | "secondhand"
    category: Mapped[str] = mapped_column(String(16), nullable=False)
    plan_id: Mapped[str] = mapped_column(String(64), nullable=False)  # unique within category
    area: Mapped[str] = mapped_column(String(64), nullable=False, default="")
    node: Mapped[str] = mapped_column(String(128), nullable=False, default="")
    name: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    # stock=-1 means "not applicable" (LS machines)
    stock: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    # Price in fen (1/100 of yuan)
    price_monthly: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    price_yearly: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    cpu: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    memory_mb: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    disk_gb: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    bandwidth_mbps: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    traffic_gb: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    ipv4: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    # true = currently on sale / in stock / available
    available: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    # Seller-declared IP reachability for secondhand listings: "normal" | "gfw" | "".
    # Sourced from the Akile listing's ipStatus field; empty for vps/ls.
    ip_status: Mapped[str] = mapped_column(String(16), nullable=False, default="")
    extra: Mapped[Optional[str]] = mapped_column(Text, nullable=True)  # JSON blob
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class AlertEvent(Base):
    """Record of every state-change event."""
    __tablename__ = "alert_event"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    category: Mapped[str] = mapped_column(String(16), nullable=False)
    plan_id: Mapped[str] = mapped_column(String(64), nullable=False)
    name: Mapped[str] = mapped_column(String(256), nullable=False, default="")
    area: Mapped[str] = mapped_column(String(64), nullable=False, default="")
    # "new_listing" | "delisted" | "restock" | "out_of_stock" | "price_change"
    event_type: Mapped[str] = mapped_column(String(32), nullable=False)
    old_value: Mapped[Optional[str]] = mapped_column(String(256), nullable=True)
    new_value: Mapped[Optional[str]] = mapped_column(String(256), nullable=True)
    notified: Mapped[bool] = mapped_column(Boolean, nullable=False, default=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)


class PriceHistory(Base):
    """Historical pricing from GetAllRecyclePackage."""
    __tablename__ = "price_history"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    plan_id: Mapped[str] = mapped_column(String(64), nullable=False)
    price: Mapped[int] = mapped_column(Integer, nullable=False)  # fen
    cycle: Mapped[int] = mapped_column(Integer, nullable=False)  # months
    fetched_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)


class AdminUser(Base):
    """Single admin account for dashboard access control.

    Only one row is expected. Password is stored as a bcrypt hash; the plaintext
    is never persisted. Provisioned from env on first boot, or via the setup
    endpoint when no env credentials are supplied.
    """
    __tablename__ = "admin_user"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    username: Mapped[str] = mapped_column(String(64), unique=True, nullable=False)
    password_hash: Mapped[str] = mapped_column(String(256), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


async def init_db():
    async with engine.begin() as conn:
        # SQLite concurrency hardening: WAL lets readers and a writer coexist
        # (rollback-journal mode takes a global lock on every write), and
        # busy_timeout makes concurrent writers wait instead of erroring out.
        # WAL is a persistent DB property; busy_timeout/synchronous are
        # per-connection, so they're also set here on the init connection.
        if DATABASE_URL.startswith("sqlite"):
            await conn.exec_driver_sql("PRAGMA journal_mode=WAL")
            await conn.exec_driver_sql("PRAGMA synchronous=NORMAL")
            await conn.exec_driver_sql("PRAGMA busy_timeout=30000")
        await conn.run_sync(Base.metadata.create_all)
        # Lightweight migration: add ip_status to pre-existing machine_snapshot
        # tables (create_all won't ALTER an existing table).
        if not await _has_column(conn, "machine_snapshot", "ip_status"):
            await conn.exec_driver_sql(
                "ALTER TABLE machine_snapshot ADD COLUMN ip_status VARCHAR(16) NOT NULL DEFAULT ''"
            )


async def _has_column(conn, table: str, column: str) -> bool:
    rows = await conn.exec_driver_sql(f"PRAGMA table_info({table})")
    return any(r[1] == column for r in rows.fetchall())


async def get_db():
    async with SessionLocal() as session:
        yield session
