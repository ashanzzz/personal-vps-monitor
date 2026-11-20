"""
Network Latency & Connectivity Probe Engine for VPS instances.
Supports:
  - TCP Port Ping (Socket connect RTT, works for non-ICMP hosts)
  - HTTP / HTTPS RTT Ping
  - Concurrent multi-target batch probing
"""
from __future__ import annotations

import asyncio
import logging
import time
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)


async def tcp_ping(host: str, port: int = 22, timeout: float = 3.0) -> Dict[str, Any]:
    """
    Measure TCP connection round-trip time (RTT) to target host:port.
    Returns:
        {
            "host": host,
            "port": port,
            "alive": True/False,
            "rtt_ms": round(rtt, 1),
            "error": None or str
        }
    """
    if not host:
        return {"host": host, "port": port, "alive": False, "rtt_ms": -1, "error": "Empty host"}

    start = time.perf_counter()
    try:
        reader, writer = await asyncio.wait_for(
            asyncio.open_connection(host, port),
            timeout=timeout
        )
        rtt = (time.perf_counter() - start) * 1000.0
        writer.close()
        try:
            await writer.wait_closed()
        except Exception:
            pass
        return {
            "host": host,
            "port": port,
            "alive": True,
            "rtt_ms": round(rtt, 1),
            "error": None
        }
    except asyncio.TimeoutError:
        return {
            "host": host,
            "port": port,
            "alive": False,
            "rtt_ms": -1,
            "error": "Connection timed out"
        }
    except Exception as e:
        return {
            "host": host,
            "port": port,
            "alive": False,
            "rtt_ms": -1,
            "error": str(e)
        }


async def http_ping(url: str, timeout: float = 5.0, proxy: Optional[str] = None) -> Dict[str, Any]:
    """
    Measure HTTP/HTTPS latency to a target URL.
    """
    import httpx
    start = time.perf_counter()
    try:
        async with httpx.AsyncClient(timeout=timeout, proxy=proxy, verify=False) as client:
            resp = await client.get(url)
            rtt = (time.perf_counter() - start) * 1000.0
            return {
                "url": url,
                "alive": resp.status_code < 500,
                "status_code": resp.status_code,
                "rtt_ms": round(rtt, 1),
                "error": None
            }
    except Exception as e:
        return {
            "url": url,
            "alive": False,
            "status_code": 0,
            "rtt_ms": -1,
            "error": str(e)
        }


async def batch_tcp_ping(targets: List[Dict[str, Any]], timeout: float = 3.0) -> List[Dict[str, Any]]:
    """
    Batch concurrent probe across multiple hosts.
    Each target should be dict with "id", "host", "port" (default 22).
    """
    tasks = []
    for t in targets:
        h = t.get("host") or t.get("ip") or ""
        p = int(t.get("port") or 22)
        tasks.append(tcp_ping(h, p, timeout=timeout))

    results = await asyncio.gather(*tasks, return_exceptions=True)
    out = []
    for idx, res in enumerate(results):
        t = targets[idx]
        if isinstance(res, Exception):
            out.append({
                "target_id": t.get("id"),
                "name": t.get("name"),
                "host": t.get("host") or t.get("ip"),
                "port": t.get("port", 22),
                "alive": False,
                "rtt_ms": -1,
                "error": str(res)
            })
        else:
            merged = dict(res)
            merged["target_id"] = t.get("id")
            merged["name"] = t.get("name")
            out.append(merged)
    return out
