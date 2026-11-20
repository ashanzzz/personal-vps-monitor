"""
Provider Registry & Factory for Personal VPS Monitor.
"""
from __future__ import annotations

import logging
from typing import Dict, List, Optional
from .base import BaseProvider

logger = logging.getLogger(__name__)

_PROVIDERS: Dict[str, BaseProvider] = {}


def register_provider(provider: BaseProvider) -> None:
    pid = provider.id.lower()
    _PROVIDERS[pid] = provider
    logger.debug("Registered provider: %s (%s)", pid, provider.name)


def get_provider(platform_id: str) -> Optional[BaseProvider]:
    return _PROVIDERS.get(platform_id.lower())


def list_providers() -> List[BaseProvider]:
    return list(_PROVIDERS.values())


def list_provider_ids() -> List[str]:
    return list(_PROVIDERS.keys())
