"""
AkileCloud Vendor Provider Implementation.
"""
from __future__ import annotations

import logging
from typing import Any, Dict, List, Optional

from .base import BaseProvider
from .. import akile_client

logger = logging.getLogger(__name__)


class AkileProvider(BaseProvider):
    @property
    def id(self) -> str:
        return "akile"

    @property
    def name(self) -> str:
        return "AkileCloud"

    async def get_account_info(self, token: str) -> Dict[str, Any]:
        return await akile_client.get_user_info(token)

    async def get_instances(self, token: str) -> List[Dict[str, Any]]:
        return await akile_client.get_my_servers(token)

    async def checkin(self, token: str) -> Dict[str, Any]:
        return await akile_client.checkin(token)

    async def get_secondhand_listings(
        self,
        token: Optional[str] = None,
        fetch_all: bool = False,
        page: int = 1,
        page_size: int = 50,
    ) -> List[Dict[str, Any]]:
        return await akile_client.get_secondhand_list(
            token or "",
            fetch_all=fetch_all,
            page=page,
            page_size=page_size,
        )

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
                "platform": "akile",
                "listing_id": listing_id,
                "message": "Dry-run purchase simulation completed successfully",
            }
        return await akile_client.buy_secondhand(token, int(listing_id))

    async def toggle_auto_renew(
        self,
        token: str,
        instance_id: int | str,
        enabled: bool,
    ) -> Dict[str, Any]:
        return await akile_client.update_auto_renew(token, int(instance_id), enabled)
