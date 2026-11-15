"""
Third-party read-only API for 个人 vps 监控.

This namespace exposes a stable, versioned, read-only contract for external
consumers. It reuses the same machine snapshot data as the internal admin API,
but keeps authentication and route names separate.
"""
from __future__ import annotations

from typing import Literal

from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from .auth import require_read_api_key
from .config import get_all_settings
from .database import MachineSnapshot, get_db
from .match import find_official_equivalent
from .scoring import compute_score, compute_star_rating, price_recommendation
from .version import get_version_info

router = APIRouter(prefix="/api/read/v1", tags=["read-api"])
_ALLOWED_CATEGORIES = {"vps", "ls", "secondhand", "cloud"}


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


def _normalize_category(category: str | None) -> str | None:
    if category is None:
        return None
    category = category.strip().lower()
    if not category:
        return None
    if category == "cloud":
        return "vps"
    if category not in _ALLOWED_CATEGORIES:
        raise HTTPException(status_code=400, detail="不支持的分类")
    return category


def _machine_payload(
    r: MachineSnapshot,
    official_match: dict | None = None,
    score: float = 0.0,
) -> dict:
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


def _score_rules_by_category(settings: dict) -> dict[str, object]:
    return {
        "vps": settings.get("score_rules_cloud"),
        "ls": settings.get("score_rules_ls"),
        "secondhand": settings.get("score_rules_secondhand"),
    }


def _sort_items(
    items: list[dict],
    sort: Literal["score_desc", "price_asc", "star_desc"],
) -> list[dict]:
    if sort == "price_asc":
        return sorted(
            items,
            key=lambda item: (
                item.get("price_monthly") or 10**18,
                -float(item.get("score") or 0),
                -(item.get("star_rating") or 0),
                item.get("plan_id") or "",
            ),
        )
    if sort == "star_desc":
        return sorted(
            items,
            key=lambda item: (
                -(item.get("star_rating") or 0),
                -float(item.get("score") or 0),
                item.get("plan_id") or "",
            ),
        )
    return sorted(
        items,
        key=lambda item: (
            -float(item.get("score") or 0),
            -(item.get("star_rating") or 0),
            item.get("plan_id") or "",
        ),
    )


async def _build_meta(db: AsyncSession) -> dict:
    settings = await get_all_settings(db)
    rows = (await db.execute(select(MachineSnapshot))).scalars().all()
    updated_at = settings.get("store_last_refresh")
    if not updated_at:
        latest_row = max((r.updated_at for r in rows if r.updated_at), default=None)
        updated_at = latest_row.isoformat() if latest_row else None
    version = get_version_info()
    return {
        "version": version.get("version", ""),
        "git_sha": version.get("git_sha", ""),
        "build_time": version.get("build_time", ""),
        "updated_at": updated_at,
        "machine_count": len(rows),
        "categories": sorted({r.category for r in rows}),
    }


async def _build_machine_response(
    db: AsyncSession,
    category: str | None,
    available_only: bool,
    min_score: float,
    min_star: int,
    ip_status: str | None,
    sort: Literal["score_desc", "price_asc", "star_desc"],
) -> dict:
    settings = await get_all_settings(db)
    normalized_category = _normalize_category(category)

    min_score = max(min_score, 0.0)
    min_star = max(min_star, 0)

    q = select(MachineSnapshot)
    if normalized_category:
        q = q.where(MachineSnapshot.category == normalized_category)
    if available_only:
        q = q.where(MachineSnapshot.available == True)  # noqa: E712
    if ip_status:
        q = q.where(MachineSnapshot.ip_status == ip_status)
    q = q.order_by(MachineSnapshot.category, MachineSnapshot.area, MachineSnapshot.name)
    rows = (await db.execute(q)).scalars().all()

    official: list[MachineSnapshot] = []
    if any(r.category == "secondhand" for r in rows):
        official = (
            await db.execute(
                select(MachineSnapshot).where(MachineSnapshot.category.in_(["vps", "ls"]))
            )
        ).scalars().all()

    score_rules_by_cat = _score_rules_by_category(settings)
    items: list[dict] = []
    for r in rows:
        match = None
        if r.category == "secondhand" and official:
            match = find_official_equivalent(r, official)
        score = compute_score(r, score_rules_by_cat.get(r.category))
        payload = _machine_payload(r, match, score)
        if payload["score"] < min_score:
            continue
        if payload["star_rating"] < min_star:
            continue
        items.append(payload)

    items = _sort_items(items, sort)
    meta = await _build_meta(db)
    return {
        "version": meta["version"],
        "git_sha": meta["git_sha"],
        "build_time": meta["build_time"],
        "updated_at": meta["updated_at"],
        "count": len(items),
        "items": items,
    }


@router.get("/meta")
async def read_meta(
    _: str = Depends(require_read_api_key),
    db: AsyncSession = Depends(get_db),
):
    return await _build_meta(db)


@router.get("/health")
async def read_health(
    _: str = Depends(require_read_api_key),
    db: AsyncSession = Depends(get_db),
):
    meta = await _build_meta(db)
    return {
        "status": "ok",
        "version": meta["version"],
        "updated_at": meta["updated_at"],
        "machine_count": meta["machine_count"],
        "categories": meta["categories"],
    }


@router.get("/machines")
@router.get("/machines/{category}")
async def read_machines(
    category: str | None = None,
    available_only: bool = False,
    min_score: float = 0.0,
    min_star: int = 0,
    ip_status: str | None = None,
    sort: Literal["score_desc", "price_asc", "star_desc"] = "score_desc",
    _: str = Depends(require_read_api_key),
    db: AsyncSession = Depends(get_db),
):
    if ip_status is not None:
        ip_status = ip_status.strip() or None
    return await _build_machine_response(
        db=db,
        category=category,
        available_only=available_only,
        min_score=min_score,
        min_star=min_star,
        ip_status=ip_status,
        sort=sort,
    )
