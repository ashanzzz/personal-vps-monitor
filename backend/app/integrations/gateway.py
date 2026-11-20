"""
Unraid Sing-box / Clash.Meta Gateway Integration Connector.

Connects to the Unraid local network gateway core (default: http://192.168.8.11:9090)
to provide optional read-only telemetry, latency verification, and outbound policy group observation.
"""
from __future__ import annotations

import logging
from typing import Any, Dict, List, Optional
from urllib.parse import quote
import httpx

logger = logging.getLogger(__name__)

DEFAULT_UNRAID_GATEWAY_URL = "http://192.168.8.11:9090"
DEFAULT_UNRAID_CONTROL_TOKEN = "Woo@@@204317"


class UnraidGatewayConnector:
    """
    Client for Unraid Sing-box / Clash.Meta REST API.
    Operates in decoupled mode: if the gateway is unreachable, it fails gracefully without affecting VPS core.
    """
    def __init__(
        self,
        api_base: str = DEFAULT_UNRAID_GATEWAY_URL,
        secret: str = DEFAULT_UNRAID_CONTROL_TOKEN,
        timeout: float = 5.0,
    ):
        self.api_base = api_base.rstrip("/")
        self.secret = secret
        self.timeout = timeout

    def _headers(self) -> Dict[str, str]:
        headers = {
            "Content-Type": "application/json",
            "User-Agent": "Personal-VPS-Monitor/GatewayConnector",
        }
        if self.secret:
            headers["Authorization"] = f"Bearer {self.secret}"
        return headers

    async def get_version(self) -> Dict[str, Any]:
        """GET /version - Check if gateway is running and fetch version."""
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                r = await client.get(f"{self.api_base}/version", headers=self._headers())
                if r.status_code == 200:
                    data = r.json()
                    return {"running": True, "api_base": self.api_base, **data}
                return {"running": False, "status_code": r.status_code, "error": r.text}
        except Exception as e:
            return {"running": False, "api_base": self.api_base, "error": str(e)}

    async def get_proxies(self) -> Dict[str, Any]:
        """GET /proxies - Fetch all groups and active nodes."""
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                r = await client.get(f"{self.api_base}/proxies", headers=self._headers())
                r.raise_for_status()
                data = r.json()
                raw_proxies = data.get("proxies", {})
                
                groups = {}
                nodes = {}
                for name, item in raw_proxies.items():
                    itype = item.get("type", "")
                    if itype in ("Selector", "URLTest", "Fallback"):
                        groups[name] = item
                    elif itype not in ("Reject", "Direct", "Compatible"):
                        nodes[name] = item

                return {
                    "running": True,
                    "total_count": len(raw_proxies),
                    "groups_count": len(groups),
                    "nodes_count": len(nodes),
                    "groups": groups,
                    "nodes": nodes,
                }
        except Exception as e:
            return {"running": False, "error": str(e), "groups": {}, "nodes": {}}

    async def get_group_detail(self, group_name: str) -> Dict[str, Any]:
        """GET /proxies/{group_name} - Query specific policy group."""
        try:
            url = f"{self.api_base}/proxies/{quote(group_name)}"
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                r = await client.get(url, headers=self._headers())
                r.raise_for_status()
                return {"group": group_name, "data": r.json()}
        except Exception as e:
            return {"group": group_name, "error": str(e)}

    async def test_node_delay(
        self,
        node_name: str,
        test_url: str = "http://www.gstatic.com/generate_204",
        timeout_ms: int = 5000,
    ) -> Dict[str, Any]:
        """GET /proxies/{node_name}/delay - Test node latency through gateway."""
        try:
            url = f"{self.api_base}/proxies/{quote(node_name)}/delay?timeout={timeout_ms}&url={quote(test_url)}"
            async with httpx.AsyncClient(timeout=(timeout_ms / 1000.0) + 2.0) as client:
                r = await client.get(url, headers=self._headers())
                if r.status_code == 200:
                    data = r.json()
                    return {"node": node_name, "alive": True, "delay_ms": data.get("delay", -1)}
                return {"node": node_name, "alive": False, "status_code": r.status_code, "error": r.text}
        except Exception as e:
            return {"node": node_name, "alive": False, "delay_ms": -1, "error": str(e)}

    async def select_node(self, group_name: str, target_node: str) -> Dict[str, Any]:
        """PUT /proxies/{group_name} - Switch outbound node in a selector group."""
        try:
            url = f"{self.api_base}/proxies/{quote(group_name)}"
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                r = await client.put(url, json={"name": target_node}, headers=self._headers())
                if r.status_code in (200, 204):
                    return {"status": "success", "group": group_name, "active_node": target_node}
                return {"status": "error", "status_code": r.status_code, "error": r.text}
        except Exception as e:
            return {"status": "error", "group": group_name, "error": str(e)}

    async def reload_config(self, force: bool = False) -> Dict[str, Any]:
        """PUT /configs - Hot reload configuration."""
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                r = await client.put(f"{self.api_base}/configs?force={str(force).lower()}", headers=self._headers(), json={})
                if r.status_code in (200, 204):
                    return {"status": "success", "message": "Unraid gateway configuration reloaded"}
                return {"status": "error", "error": r.text}
        except Exception as e:
            return {"status": "error", "error": str(e)}


gateway_connector = UnraidGatewayConnector()
