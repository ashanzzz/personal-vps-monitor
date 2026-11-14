"""
Database models and async engine setup.
"""
import os
from datetime import datetime
from typing import Optional

from sqlalchemy import Boolean, DateTime, Integer, String, Text, Float, Index
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
    # Secondhand listing/sale price in fen. Distinct from renewal prices,
    # which are derived from price_datas into price_monthly/price_yearly.
    sale_price: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
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


class AccountCache(Base):
    """Cached snapshot of the logged-in Akile account.

    Single row (id=1). Holds the aggregated account payload (profile + balance
    + own servers) as a JSON blob so the account page can render instantly from
    cache instead of hitting the upstream API on every load. Refreshed on its
    own independent interval by the account poller, or on demand.
    """
    __tablename__ = "account_cache"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    data: Mapped[str] = mapped_column(Text, nullable=False, default="")  # JSON blob
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class CheckinLog(Base):
    """Daily checkin status and history for Akile & ACCK."""
    __tablename__ = "checkin_log"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    platform: Mapped[str] = mapped_column(String(32), nullable=False)  # "akile" | "acck"
    checkin_date: Mapped[str] = mapped_column(String(16), nullable=False)  # "YYYY-MM-DD"
    status: Mapped[str] = mapped_column(String(32), nullable=False)  # "success" | "already_signed" | "failed"
    message: Mapped[str] = mapped_column(Text, nullable=False, default="")
    details: Mapped[Optional[str]] = mapped_column(Text, nullable=True)  # JSON blob
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)


class MarketTrade(Base):
    """Dual-platform secondhand market trade transactions & arbitrage tracker."""
    __tablename__ = "market_trades"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    platform: Mapped[str] = mapped_column(String(32), nullable=False)  # "akile" | "acck"
    server_id: Mapped[int] = mapped_column(Integer, nullable=False)
    server_name: Mapped[str] = mapped_column(String(128), nullable=False, default="")
    plan_name: Mapped[str] = mapped_column(String(64), nullable=False, default="")
    node_name: Mapped[str] = mapped_column(String(64), nullable=False, default="")
    listing_id: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    buy_order_id: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    buy_price_cents: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    ask_price_cents: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    fee_rate: Mapped[float] = mapped_column(Float, nullable=False, default=0.10)
    fee_cents: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    seller_net_cents: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    profit_cents: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    status: Mapped[str] = mapped_column(String(32), nullable=False, default="on_sale")  # "on_sale", "sold", "cancelled"
    remark: Mapped[str] = mapped_column(Text, nullable=False, default="")
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)
    sold_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)


class MarketAssetDailyKline(Base):
    """Daily aggregated market stats (median price P50, P25, P75, min, max, volume, depth) for canonical asset SKUs.
    Designed for 1-3 years efficient retention (approx 50 canonical assets * 365 days = ~18,000 rows, <3MB).
    """
    __tablename__ = "market_asset_daily_kline"
    __table_args__ = (
        Index("ix_asset_kline_date", "date"),
        Index("ix_asset_kline_asset_date", "asset_id", "date", unique=True),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    asset_id: Mapped[str] = mapped_column(String(128), nullable=False)
    date: Mapped[str] = mapped_column(String(16), nullable=False)  # "YYYY-MM-DD"
    platform: Mapped[str] = mapped_column(String(32), nullable=False, default="all")
    asset_name: Mapped[str] = mapped_column(String(128), nullable=False, default="")
    route_family: Mapped[str] = mapped_column(String(64), nullable=False, default="")
    specs_code: Mapped[str] = mapped_column(String(64), nullable=False, default="")
    monthly_renew_cny: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)

    # 核心价格分布 (元) - 以中位数 P50 为核心公允基准
    price_median: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)
    price_p25: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)
    price_p75: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)
    price_min: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)
    price_max: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)

    # 盘口存量深度与推断成交
    active_listings: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    inferred_sales: Mapped[int] = mapped_column(Integer, nullable=False, default=0)
    inferred_turnover_cny: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)
    turnover_rate: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)

    # 模型公允身价与溢价/折价偏离度
    fair_model_value: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)
    premium_spread_pct: Mapped[float] = mapped_column(Float, nullable=False, default=0.0)
    market_state: Mapped[str] = mapped_column(String(32), nullable=False, default="NORMAL")  # "UNDERVALUED", "NORMAL", "OVERVALUED", "DOWNTURN"

    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)


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
        if not await _has_column(conn, "machine_snapshot", "sale_price"):
            await conn.exec_driver_sql(
                "ALTER TABLE machine_snapshot ADD COLUMN sale_price INTEGER"
            )


async def _has_column(conn, table: str, column: str) -> bool:
    rows = await conn.exec_driver_sql(f"PRAGMA table_info({table})")
    return any(r[1] == column for r in rows.fetchall())


async def get_db():
    async with SessionLocal() as session:
        yield session
