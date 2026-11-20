"""
LazyCat Cloud Vendor Provider Implementation.

=== 【用户铁律 · 永久生效】懒猫云实例绝不可销毁 ===
严禁添加任何销毁、删除实例的方法或入口。
"""
from __future__ import annotations

import logging
from typing import Any, Dict, List, Optional

from .base import BaseProvider
from .. import lazycat_client

logger = logging.getLogger(__name__)


class LazyCatProvider(BaseProvider):
    @property
    def id(self) -> str:
        return "lazycat"

    @property
    def name(self) -> str:
        return "LazyCat Cloud"

    async def get_account_info(self, token: str) -> Dict[str, Any]:
        info = await lazycat_client.get_user_info(token)
        bal = await lazycat_client.get_user_balance(token)
        return {
            "user": info.get("user") or info,
            "balance": bal,
        }

    async def get_instances(self, token: str) -> List[Dict[str, Any]]:
        return await lazycat_client.get_my_instances(token)

    async def checkin(self, token: str) -> Dict[str, Any]:
        return {
            "success": True,
            "platform": "lazycat",
            "message": "LazyCat has no manual check-in requirement; VIP rewards are automatically applied",
        }

    async def get_secondhand_listings(
        self,
        token: Optional[str] = None,
        fetch_all: bool = False,
        page: int = 1,
        page_size: int = 50,
    ) -> List[Dict[str, Any]]:
        if not token:
            return []
        data = await lazycat_client.get_market_listings(
            token,
            page=page,
            page_size=page_size,
        )
        return data.get("listings") or []

    async def buy_secondhand_listing(
        self,
        token: str,
        listing_id: int | str,
        price_cents: int = 0,
        safe_mode: bool = True,
    ) -> Dict[str, Any]:
        if safe_mode:
            return {
                "safe_mode": True,
                "platform": "lazycat",
                "listing_id": listing_id,
                "message": "Dry-run purchase simulation completed successfully",
            }
        return await lazycat_client.purchase_instance(token, int(listing_id))

    async def toggle_auto_renew(
        self,
        token: str,
        instance_id: int | str,
        enabled: bool,
    ) -> Dict[str, Any]:
        return {
            "success": False,
            "platform": "lazycat",
            "message": "LazyCat instances are billing per container cycle, auto-renew toggle is managed via balance",
        }
