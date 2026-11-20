"""
Unified Providers Package.
Exposes registered cloud providers (Akile, ACCK, LazyCat).
"""
from .base import BaseProvider
from .registry import register_provider, get_provider, list_providers, list_provider_ids
from .akile import AkileProvider
from .acck import ACCKProvider
from .lazycat import LazyCatProvider

# Automatically register default providers
register_provider(AkileProvider())
register_provider(ACCKProvider())
register_provider(LazyCatProvider())

__all__ = [
    "BaseProvider",
    "register_provider",
    "get_provider",
    "list_providers",
    "list_provider_ids",
    "AkileProvider",
    "ACCKProvider",
    "LazyCatProvider",
]
