"""
Sub-Store Subscription & Node Pipeline Integration Connector.

Connects to the self-hosted Sub-Store instance (default: http://192.168.8.11:3000)
to provide optional node publishing from VPS instances into Sub-Store collections.
"""
from __future__ import annotations

import logging
from typing import Any, Dict, List, Optional
import httpx

logger = logging.getLogger(__name__)

DEFAULT_SUBSTORE_URL = "http://192.168.8.11:3000"


class SubStoreConnector:
    """
    Client for interacting with Sub-Store backend service.
    """
    def __init__(
        self,
        base_url: str = DEFAULT_SUBSTORE_URL,
        backend_path: str = "",
        timeout: float = 5.0,
    ):
        self.base_url = base_url.rstrip("/")
        self.backend_path = backend_path.strip("/")
        self.timeout = timeout

    async def get_status(self) -> Dict[str, Any]:
        """Check Sub-Store status and runtime version via GET /api/version."""
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                r = await client.get(f"{self.base_url}/api/version")
                if r.status_code == 200:
                    data = r.json()
                    return {
                        "online": True,
                        "version": data.get("version", "unknown"),
                        "sha": data.get("sha", ""),
                        "base_url": self.base_url,
                        "error": None,
                    }
                return {
                    "online": False,
                    "status_code": r.status_code,
                    "error": f"HTTP {r.status_code}: {r.text}",
                }
        except Exception as e:
            return {
                "online": False,
                "base_url": self.base_url,
                "error": str(e),
            }

    def format_node_from_vps(
        self,
        server_info: Dict[str, Any],
        custom_configs: Optional[List[Dict[str, Any]]] = None,
    ) -> List[Dict[str, Any]]:
        """
        Produce normalized Sub-Store node JSON specifications for ANY specific VPS machine.
        Automatically resolves:
          - Machine platform (akile, acck, lazycat, etc.)
          - Public IP or NAT mapped entrance
          - Forwarded ports from NAT rules (e.g. Reality, SS)
        """
        platform = str(server_info.get("platform", "vps")).upper()
        sname = server_info.get("name") or f"{platform}-{server_info.get('id', 'srv')}"
        area = server_info.get("area_name") or server_info.get("area") or "HK"
        host = server_info.get("ipv4") or server_info.get("host") or ""
        
        nodes = []
        configs = custom_configs or server_info.get("proxy_configs") or []
        
        # If no specific configs passed, infer from NAT ports if available
        if not configs:
            ports = server_info.get("port_mappings") or []
            for p in ports:
                priv = p.get("privatePort") or p.get("private_port")
                pub = p.get("publicPort") or p.get("public_port")
                if not pub:
                    continue
                remark = str(p.get("remark", "")).lower()
                if priv in (443, 8443) or "reality" in remark:
                    configs.append({
                        "name": f"[{platform}|{area}] {sname}-Reality",
                        "type": "vless",
                        "port": pub,
                        "sni": "www.apple.com",
                    })
                elif priv in (8443, 55966) or "ss" in remark:
                    configs.append({
                        "name": f"[{platform}|{area}] {sname}-SS",
                        "type": "ss",
                        "port": pub,
                    })

        for cfg in configs:
            ptype = cfg.get("type", "vless")
            port = int(cfg.get("port") or 443)
            node_tag = cfg.get("name") or f"[{platform}|{area}] {sname}-{ptype.upper()}"
            
            if ptype == "vless":
                nodes.append({
                    "name": node_tag,
                    "type": "vless",
                    "server": host,
                    "port": port,
                    "uuid": cfg.get("uuid", "bbe1e489-a372-47f1-8f65-41aa93118bd5"),
                    "network": cfg.get("network", "tcp"),
                    "tls": True,
                    "flow": cfg.get("flow", "xtls-rprx-vision"),
                    "servername": cfg.get("sni", "www.apple.com"),
                    "reality-opts": {
                        "public-key": cfg.get("public_key", "fSA2tLImc06SvOdAubf2tI2q_w9J9F9cuatYzkMXhUg"),
                        "short-id": cfg.get("short_id", "c653e997"),
                    },
                    "client-fingerprint": "chrome",
                })
            elif ptype == "ss":
                nodes.append({
                    "name": node_tag,
                    "type": "ss",
                    "server": host,
                    "port": port,
                    "cipher": cfg.get("cipher", "2022-blake3-aes-128-gcm"),
                    "password": cfg.get("password", "4iHVwPPj/ENy6UvVulkOHA=="),
                    "udp": cfg.get("udp", True),
                })
            elif ptype == "hysteria2":
                nodes.append({
                    "name": node_tag,
                    "type": "hysteria2",
                    "server": host,
                    "port": port,
                    "password": cfg.get("password", ""),
                    "sni": cfg.get("sni", ""),
                })

        return nodes


substore_connector = SubStoreConnector()
