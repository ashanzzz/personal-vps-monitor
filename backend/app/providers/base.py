"""
Abstract Base Provider Specification for Cloud VPS & Instance Vendors.

Every platform provider (Akile, ACCK, LazyCat, or future vendors) implements
this contract to achieve clean separation of concerns and symmetrical lifecycle capabilities.
"""
from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Any, Dict, List, Optional


class BaseProvider(ABC):
    """
    Unified contract that all vendor adapters must satisfy.
    """

    @property
    @abstractmethod
    def id(self) -> str:
        """Unique provider identifier (e.g. 'akile', 'acck', 'lazycat')."""
        pass

    @property
    @abstractmethod
    def name(self) -> str:
        """Human-readable provider name."""
        pass

    @abstractmethod
    async def get_account_info(self, token: str) -> Dict[str, Any]:
        """Fetch profile, balance, credits/points and VIP level."""
        pass

    @abstractmethod
    async def get_instances(self, token: str) -> List[Dict[str, Any]]:
        """Fetch active server/container instances belonging to the user."""
        pass

    @abstractmethod
    async def checkin(self, token: str) -> Dict[str, Any]:
        """Perform daily check-in / reward claim."""
        pass

    @abstractmethod
    async def get_secondhand_listings(
        self,
        token: Optional[str] = None,
        fetch_all: bool = False,
        page: int = 1,
        page_size: int = 50,
    ) -> List[Dict[str, Any]]:
        """Fetch in-market secondhand or push-shop transfer listings."""
        pass

    @abstractmethod
    async def buy_secondhand_listing(
        self,
        token: str,
        listing_id: int | str,
        price_cents: int = 0,
        safe_mode: bool = True,
    ) -> Dict[str, Any]:
        """Purchase or simulate purchase of a secondhand listing."""
        pass

    @abstractmethod
    async def toggle_auto_renew(
        self,
        token: str,
        instance_id: int | str,
        enabled: bool,
    ) -> Dict[str, Any]:
        """Toggle instance auto-renewal status."""
        pass
