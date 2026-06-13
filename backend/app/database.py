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

engine = create_async_engine(DATABASE_URL, echo=False)
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


async def init_db():
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)


async def get_db():
    async with SessionLocal() as session:
        yield session
