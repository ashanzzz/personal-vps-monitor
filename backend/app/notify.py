"""
Telegram notification sender.
"""
from __future__ import annotations

import httpx

_ICONS = {
    "new_listing": "🆕",
    "restock": "✅",
    "out_of_stock": "❌",
    "delisted": "🗑",
    "price_change": "💰",
    "purchased": "🛒",
}

_PRICE_PER_CYCLE = {1: "月", 12: "年", 36: "三年"}


def _fmt_price(fen: str | int | None) -> str:
    if fen is None:
        return "—"
    try:
        return f"¥{int(fen) / 100:.2f}"
    except (TypeError, ValueError):
        return str(fen)


def _build_text(event: dict) -> str:
    icon = _ICONS.get(event["event_type"], "ℹ️")
    et_labels = {
        "new_listing": "新上架",
        "restock": "已补货",
        "out_of_stock": "已售罄",
        "delisted": "已下架",
        "price_change": "价格变动",
        "purchased": "购买成功",
    }
    label = et_labels.get(event["event_type"], event["event_type"])
    cat_labels = {"vps": "新机VPS", "ls": "轻量机", "secondhand": "二手机"}
    cat = cat_labels.get(event.get("category", ""), event.get("category", ""))

    lines = [
        f"{icon} <b>Akile {cat} {label}</b>",
        f"机器：{event.get('name', event.get('plan_id', ''))}",
    ]
    if event.get("area"):
        lines.append(f"区域：{event['area']}")

    et = event["event_type"]
    if et == "price_change":
        lines.append(f"价格：{_fmt_price(event.get('old_value'))} → {_fmt_price(event.get('new_value'))}")
    elif et in ("restock", "out_of_stock", "new_listing", "delisted"):
        if event.get("new_value") not in (None, ""):
            pass  # availability is self-describing
    elif et == "purchased":
        lines.append(f"金额：{_fmt_price(event.get('new_value'))}")

    return "\n".join(lines)


async def send_telegram(token: str, chat_id: str, event: dict) -> None:
    text = _build_text(event)
    url = f"https://api.telegram.org/bot{token}/sendMessage"
    try:
        async with httpx.AsyncClient(timeout=10) as c:
            await c.post(url, json={
                "chat_id": chat_id,
                "text": text,
                "parse_mode": "HTML",
                "disable_notification": False,
            })
    except Exception:
        pass  # Don't crash the poller on notification failure


async def send_custom(token: str, chat_id: str, text: str) -> None:
    url = f"https://api.telegram.org/bot{token}/sendMessage"
    try:
        async with httpx.AsyncClient(timeout=10) as c:
            await c.post(url, json={
                "chat_id": chat_id,
                "text": text,
                "parse_mode": "HTML",
            })
    except Exception:
        pass
